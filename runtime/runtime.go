package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/MindHunter86/addie/blocklist"
	"github.com/MindHunter86/addie/utils"
	"github.com/rs/zerolog"
)

type RuntimePatchType uint8

const (
	RuntimePatchLottery RuntimePatchType = iota
	RuntimePatchQuality
	RuntimePatchBlocklist
	RuntimePatchBlocklistIps
	RuntimePatchLimiter
	RuntimePatchA5bility
	RuntimePatchStdoutAccess
)

var (
	ErrRuntimeUndefinedPatch = errors.New("given patch payload is undefined")

	RuntimeUtilsBindings = map[string]RuntimePatchType{
		utils.CfgLotteryChance:     RuntimePatchLottery,
		utils.CfgQualityLevel:      RuntimePatchQuality,
		utils.CfgBlockList:         RuntimePatchBlocklistIps,
		utils.CfgBlockListSwitcher: RuntimePatchBlocklist,
		utils.CfgLimiterSwitcher:   RuntimePatchLimiter,
		utils.CfgClusterA5bility:   RuntimePatchA5bility,
		utils.CfgStdoutAccessLog:   RuntimePatchStdoutAccess,
	}

	// intenal
	log *zerolog.Logger

	runtimeChangesHumanize = map[RuntimePatchType]string{
		RuntimePatchLottery:      "lottery chance",
		RuntimePatchQuality:      "quality level",
		RuntimePatchBlocklist:    "blocklist switch",
		RuntimePatchBlocklistIps: "blocklist ips",
		RuntimePatchLimiter:      "limiter switch",
		RuntimePatchA5bility:     "balancer's cluster availability",
		RuntimePatchStdoutAccess: "stdout access log switcher",
	}
)

type (
	Runtime struct {
		Config ConfigStorage

		// todo - refactor
		blocklist *blocklist.Blocklist // temporary;
	}
	RuntimePatch struct {
		Type  RuntimePatchType
		Patch []byte
	}
)

func NewRuntime(c context.Context) (r *Runtime, e error) {
	blist := c.Value(utils.ContextKeyBlocklist).(*blocklist.Blocklist)
	log = c.Value(utils.ContextKeyLogger).(*zerolog.Logger)

	r = &Runtime{
		blocklist: blist,
	}

	if r.Config, e = NewConfigStorage(c); e != nil {
		return
	}

	return
}

func (m *Runtime) ApplyPatch(patch *RuntimePatch) (e error) {

	if len(patch.Patch) == 0 {
		return ErrRuntimeUndefinedPatch
	}

	switch patch.Type {
	case RuntimePatchLottery:
		e = patch.ApplyLotteryChance(&m.Config)
	case RuntimePatchA5bility:
		e = patch.ApplyA5bility(&m.Config)

	case RuntimePatchQuality:
		e = patch.ApplyQualityLevel(&m.Config)
	case RuntimePatchBlocklistIps:
		e = patch.ApplyBlocklistIps(&m.Config, m.blocklist)

	case RuntimePatchBlocklist:
		e = patch.ApplySwitch(&m.Config, ConfigParamBlocklist)
	case RuntimePatchLimiter:
		e = patch.ApplySwitch(&m.Config, ConfigParamLimiter)
	case RuntimePatchStdoutAccess:
		e = patch.ApplySwitch(&m.Config, ConfigParamStdoutAccess)

	default:
		panic("internal error - undefined runtime patch type")
	}

	if e != nil {
		log.Error().Err(e).
			Msgf("could not apply runtime configuration (%s)", runtimeChangesHumanize[patch.Type])
	}

	return
}

func (m *RuntimePatch) ApplyA5bility(st *ConfigStorage) (e error) {
	var chance int
	if chance, e = strconv.Atoi(string(m.Patch)); e != nil {
		return
	}

	if chance < 0 || chance > 100 {
		e = fmt.Errorf("chance could not be less than 0 and more than 100, current %d", chance)
		return
	}

	log.Info().Msgf("runtime patch has been applied for A5Bility with %d", chance)
	return st.SetValue(ConfigParamA5bility, chance)
}

func (m *RuntimePatch) ApplyBlocklistIps(_ *ConfigStorage, bl *blocklist.Blocklist) (e error) {
	buf := string(m.Patch)

	if buf == "_" {
		bl.Reset()
		log.Info().Msg("runtime patch has been for Blocklist.Reset")
		return
	}

	lastsize := bl.Size()
	ips := strings.Split(buf, ",")
	bl.Push(ips...)

	// dummy code
	// ???
	// st.SetValue(ConfigParamBlocklistIps, ips)

	log.Info().Msgf("runtime patch has been for Blocklist, applied %d ips", len(ips))
	log.Debug().Msgf("apply blocklist: last size - %d, new - %d", lastsize, bl.Size())
	return
}

func (m *RuntimePatch) ApplySwitch(st *ConfigStorage, param ConfigParam) (e error) {
	buf := string(m.Patch)

	switch buf {
	case "0":
		e = st.SetValue(param, 0)
	case "1":
		e = st.SetValue(param, 1)
	default:
		e = fmt.Errorf("invalid value in runtime bool patch for %s : %s", GetNameByConfigParam[param], buf)
		return
	}

	log.Debug().Msgf("runtime patch has been applied for %s with %s", GetNameByConfigParam[param], buf)
	return
}

func (m *RuntimePatch) ApplyQualityLevel(st *ConfigStorage) (e error) {
	buf := string(m.Patch)

	quality, ok := utils.GetTitleQualityByString[buf]
	if !ok {
		e = fmt.Errorf("quality is invalid; 480, 720, 1080 values are permited only, current - %s", buf)
		return
	}

	log.Info().Msgf("runtime patch has been applied for QualityLevel with %s", buf)
	return st.SetValueSmoothly(ConfigParamQuality, quality)
}

func (m *RuntimePatch) ApplyLotteryChance(st *ConfigStorage) (e error) {
	var chance int
	if chance, e = strconv.Atoi(string(m.Patch)); e != nil {
		return
	}

	if chance < 0 || chance > 100 {
		e = fmt.Errorf("chance could not be less than 0 and more than 100, current %d", chance)
		return
	}

	log.Info().Msgf("runtime patch has been applied for LotteryChance with %d", chance)
	return st.SetValueSmoothly(ConfigParamLottery, chance)
}
