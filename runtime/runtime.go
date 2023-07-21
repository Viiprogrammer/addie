package runtime

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/MindHunter86/anilibria-hlp-service/blocklist"
	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

type RuntimePatchType uint8

const (
	RuntimePatchLottery RuntimePatchType = iota
	RuntimePatchQuality
	RuntimePatchBlocklist
	RuntimePatchBlocklistIps
	RuntimePatchLimiter
)

var (
	ErrRuntimeUndefinedPatch = errors.New("given patch payload is undefined")

	RuntimeUtilsBindings = map[string]RuntimePatchType{
		utils.CfgLotteryChance:     RuntimePatchLottery,
		utils.CfgQualityLevel:      RuntimePatchQuality,
		utils.CfgBlockList:         RuntimePatchBlocklistIps,
		utils.CfgBlockListSwitcher: RuntimePatchBlocklist,
		utils.CfgLimiterSwitcher:   RuntimePatchLimiter,
	}

	// intenal
	log *zerolog.Logger
	ccx *cli.Context
	ctx context.Context

	runtimeChangesHumanize = map[RuntimePatchType]string{
		RuntimePatchLottery:      "lottery chance",
		RuntimePatchQuality:      "quality level",
		RuntimePatchBlocklist:    "blocklist switch",
		RuntimePatchBlocklistIps: "blocklist ips",
		RuntimePatchLimiter:      "limiter switch",
	}
)

type (
	Runtime struct {
		Config ConfigStorage

		// todo - refactor
		blocklist *blocklist.Blocklist // temporary;

		gQualityLock  sync.RWMutex
		gQualityLevel utils.TitleQuality

		gLotteryLock   sync.RWMutex
		gLotteryChance int

		gLimiterLock    sync.RWMutex
		gLimiterEnabled int

		boot sync.WaitGroup
	}
	RuntimePatch struct {
		Type  RuntimePatchType
		Patch []byte
	}
)

func (m *Runtime) GetQualityLevel() (q utils.TitleQuality, ok bool) {

	// if !m.gQualityLock.TryRLock() || !m.Softer.GetSwitchResult(payload) {
	if !m.gQualityLock.TryRLock() {
		return 0, false
	}
	defer m.gQualityLock.RUnlock()

	q, ok = m.gQualityLevel, true
	return
}

func (m *Runtime) updateQualityLevel(q utils.TitleQuality) {
	m.gQualityLock.Lock()
	defer m.gQualityLock.Unlock()

	m.gQualityLevel = q
}

func (m *Runtime) GetLotteryChance() (c int, ok bool) {
	if !m.gLotteryLock.TryRLock() {
		return 0, false
	}
	defer m.gLotteryLock.RUnlock()

	c, ok = m.gLotteryChance, true
	return
}

func (m *Runtime) updateLotteryChance(c int) {
	m.gLotteryLock.Lock()
	defer m.gLotteryLock.Unlock()

	m.gLotteryChance = c
}

// func (m *Runtime) GetLimiterStatus() (s int, ok bool) {
// 	if !m.gLimiterLock.TryRLock() {
// 		return 0, false
// 	}
// 	defer m.gLimiterLock.RUnlock()

// 	s, ok = m.gLimiterEnabled, true
// 	return
// }

// func (m *Runtime) updateLimiterStatus(s int) {
// 	m.gLimiterLock.Lock()
// 	defer m.gLimiterLock.Unlock()

// 	m.gLimiterEnabled = s
// }

func NewRuntime(c context.Context) *Runtime {
	ctx = c

	blist := ctx.Value(utils.ContextKeyBlocklist).(*blocklist.Blocklist)
	log = ctx.Value(utils.ContextKeyLogger).(*zerolog.Logger)
	ccx = ctx.Value(utils.ContextKeyCliContext).(*cli.Context)

	return &Runtime{
		Config: NewConfigStorage(),

		blocklist: blist,

		gQualityLevel:   utils.TitleQualityFHD,
		gLotteryChance:  0,
		gLimiterEnabled: 0,
	}
}

func (m *Runtime) ApplyPatch(patch *RuntimePatch) (e error) {

	if len(patch.Patch) == 0 {
		return ErrRuntimeUndefinedPatch
	}

	switch patch.Type {
	case RuntimePatchLottery:
		e = m.applyLotteryChance(patch.Patch)
	case RuntimePatchQuality:
		e = m.applyQualityLevel(patch.Patch)
	case RuntimePatchBlocklist:
		e = m.applyBlocklistSwitch(patch.Patch)
	case RuntimePatchBlocklistIps:
		e = m.applyBlocklistChanges(patch.Patch)
	case RuntimePatchLimiter:
		e = m.applyLimitterSwitch(patch.Patch)
	default:
		panic("internal error - undefined runtime patch type")
	}

	if e != nil {
		log.Error().Err(e).
			Msgf("could not apply runtime configuration (%s)", runtimeChangesHumanize[patch.Type])
	}

	return
}

// func (m *Runtime) Bootstrap() {
// 	defer m.boot.Done()

// 	log.Trace().Msg("runtime - bootstrap has been started")
// 	defer log.Trace().Msg("runtime - bootstrap has been stopped")

// loop:
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Trace().Msg("internal abort() has been caught")
// 			break loop
// 		}
// 	}
// }

func (m *Runtime) applyBlocklistChanges(input []byte) (e error) {
	log.Debug().Msgf("runtime config - blocklist update requested")
	log.Debug().Msgf("apply blocklist - old size - %d", m.blocklist.Size())

	if bytes.Equal(input, []byte("_")) {
		m.blocklist.Reset()
		log.Info().Msg("runtime config - blocklist has been reseted")
		return
	}

	ips := strings.Split(string(input), ",")
	m.blocklist.Push(ips...)

	log.Info().Msg("runtime config - blocklist update completed")
	log.Debug().Msgf("apply blocklist - updated size - %d", m.blocklist.Size())
	return
}

func (m *Runtime) applyBlocklistSwitch(input []byte) (e error) {

	var enabled int
	if enabled, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	log.Trace().Msgf("runtime config - blocklist apply value %d", enabled)

	switch enabled {
	case 0:
		m.blocklist.Disable(true)
	case 1:
		m.blocklist.Disable(false)
	default:
		log.Warn().Int("enabled", enabled).
			Msg("runtime config - blocklist switcher could not be non 0 or non 1")
		return
	}

	log.Info().Msg("runtime config - blocklist status updated")
	log.Debug().Msgf("apply blocklist - updated value - %d", enabled)
	return
}

func (m *Runtime) applyLimitterSwitch(input []byte) (e error) {
	var enabled int
	if enabled, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	log.Trace().Msgf("runtime config - limiter apply value %d", enabled)

	switch enabled {
	case 0:
		fallthrough
	case 1:
		m.Config.SetValueSmoothly(ConfigParamLimiter, enabled)

		log.Info().Msg("runtime config - limiter status updated")
		log.Debug().Msgf("apply limiter - updated value - %d", enabled)
	default:
		log.Warn().Int("enabled", enabled).
			Msg("runtime config - limiter switcher could not be non 0 or non 1")
		return
	}
	return
}

func (m *Runtime) applyLotteryChance(input []byte, sdeploy ...bool) (e error) {
	sdeploy = append(sdeploy, false)

	var chance int
	if chance, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	if chance < 0 || chance > 100 {
		log.Warn().Int("chance", chance).Msg("chance could not be less than 0 and more than 100")
		return
	}

	log.Info().Msgf("runtime config - applied lottery chance %s", string(input))

	if !sdeploy[0] {
		m.updateLotteryChance(chance)
		return
	}

	log.Trace().Msg("runtime config - using smooth deployment")
	return
}

func (m *Runtime) applyQualityLevel(input []byte) (e error) {
	log.Debug().Msg("quality settings change requested")

	var quality utils.TitleQuality

	switch string(input) {
	case "480":
		quality = utils.TitleQualitySD
	case "720":
		quality = utils.TitleQualityHD
	case "1080":
		quality = utils.TitleQualityFHD
	default:
		log.Warn().Str("input", string(input)).Msg("qulity level can be 480 720 or 1080 only")
		return
	}

	m.updateQualityLevel(quality)
	log.Info().Msgf("runtime config - applied quality %s", string(input))
	return
}
