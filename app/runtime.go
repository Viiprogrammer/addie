package app

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

type RuntimePatchType uint8

const (
	RuntimePatchLottery RuntimePatchType = iota
	RuntimePatchQuality
	RuntimePatchBlocklist
	RuntimePatchBlocklistIps
	RuntimePatchLimiter
	RuntimePatchConsulNCluster
	RuntimePatchConsulCCluster
)

var (
	ErrRuntimeUndefinedPatch = errors.New("given patch payload is undefined")

	runtimeChangesHumanize = map[RuntimePatchType]string{
		RuntimePatchLottery:        "lottery chance",
		RuntimePatchQuality:        "quality level",
		RuntimePatchBlocklist:      "blocklist switch",
		RuntimePatchBlocklistIps:   "blocklist ips",
		RuntimePatchLimiter:        "limiter switch",
		RuntimePatchConsulNCluster: "consul cache-node cluster",
		RuntimePatchConsulCCluster: "consul cache-cloud cluster",
	}
)

type (
	Runtime struct {
		// todo - refactor
		blocklist *blocklist // temporary;
	}
	RuntimePatch struct {
		Type  RuntimePatchType
		Patch []byte
	}
	RuntimeConfig struct {
		lotteryChance     []byte
		qualityLevel      []byte
		blocklistIps      []byte
		blocklistSwitcher []byte
		limiterSwitch     []byte
	}
)

func NewRuntime(b *blocklist) *Runtime {
	return &Runtime{
		blocklist: b,
	}
}

func (m *Runtime) ApplyPath(patch *RuntimePatch) (e error) {
	if len(patch.Patch) == 0 {
		return ErrRuntimeUndefinedPatch
	}

	switch patch.Type {
	case RuntimePatchLottery:
		e = m.applyLotteryChance(patch.Patch)
	case RuntimePatchQuality:
		e = m.applyQualityLevel(patch.Patch)
	case RuntimePatchBlocklist:
		e = m.applyLimitterSwitch(patch.Patch)
	case RuntimePatchBlocklistIps:
		e = m.applyBlocklistChanges(patch.Patch)
	case RuntimePatchLimiter:
		e = m.applyLimitterSwitch(patch.Patch)
	case RuntimePatchConsulNCluster:
	case RuntimePatchConsulCCluster:
	default:
		panic("internal error - undefined runtime patch type")
	}

	if e != nil {
		gLog.Error().Err(e).
			Msgf("could not apply runtime configuration (%s)", runtimeChangesHumanize[patch.Type])
	}

	return
}

func (*App) isBlocklistEnabled() bool {
	gBListLock.RLock()
	defer gBListLock.RUnlock()

	switch gBlocklistEnabled {
	case 1:
		return true
	default:
		return false
	}
}

func (m *Runtime) applyBlocklistChanges(input []byte) (e error) {
	gLog.Debug().Msgf("runtime config - blocklist update requested")
	gLog.Debug().Msgf("apply blocklist - old size - %d", m.blocklist.size())

	if bytes.Equal(input, []byte("_")) {
		m.blocklist.reset()
		gLog.Info().Msg("runtime config - blocklist has been reseted")
		return
	}

	ips := strings.Split(string(input), ",")
	m.blocklist.push(ips...)

	gLog.Info().Msg("runtime config - blocklist update completed")
	gLog.Debug().Msgf("apply blocklist - updated size - %d", m.blocklist.size())
	return
}

func (*Runtime) applyBlocklistSwitch(input []byte) (e error) {

	var enabled int
	if enabled, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	gLog.Trace().Msgf("runtime config - blocklist apply value %d", enabled)

	switch enabled {
	case 0:
		fallthrough
	case 1:
		gBListLock.Lock()
		gBlocklistEnabled = enabled
		gBListLock.Unlock()

		gLog.Info().Msg("runtime config - blocklist status updated")
		gLog.Debug().Msgf("apply blocklist - updated value - %d", enabled)
	default:
		gLog.Warn().Int("enabled", enabled).
			Msg("runtime config - blocklist switcher could not be non 0 or non 1")
		return
	}
	return
}

func (*Runtime) applyLimitterSwitch(input []byte) (e error) {
	var enabled int
	if enabled, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	gLog.Trace().Msgf("runtime config - limiter apply value %d", enabled)

	switch enabled {
	case 0:
		fallthrough
	case 1:
		gLimiterLock.Lock()
		gLimiterEnabled = enabled
		gLimiterLock.Unlock()

		gLog.Info().Msg("runtime config - limiter status updated")
		gLog.Debug().Msgf("apply limiter - updated value - %d", enabled)
	default:
		gLog.Warn().Int("enabled", enabled).
			Msg("runtime config - limiter switcher could not be non 0 or non 1")
		return
	}
	return
}

func (*Runtime) applyLotteryChance(input []byte) (e error) {
	var chance int
	if chance, e = strconv.Atoi(string(input)); e != nil {
		return
	}

	if chance < 0 || chance > 100 {
		gLog.Warn().Int("chance", chance).Msg("chance could not be less than 0 and more than 100")
		return
	}

	gLog.Info().Msgf("runtime config - applied lottery chance %s", string(input))

	gLotteryLock.Lock()
	gLotteryChance = chance
	gLotteryLock.Unlock()

	return
}

func (*Runtime) applyQualityLevel(input []byte) (e error) {
	gLog.Debug().Msg("quality settings change requested")

	var quality titleQuality

	switch string(input) {
	case "480":
		quality = titleQualitySD
	case "720":
		quality = titleQualityHD
	case "1080":
		quality = titleQualityFHD
	default:
		gLog.Warn().Str("input", string(input)).Msg("qulity level can be 480 720 or 1080 only")
		return
	}

	gQualityLock.Lock()
	gQualityLevel = quality
	gQualityLock.Unlock()

	gLog.Info().Msgf("runtime config - applied quality %s", string(input))
	return
}
