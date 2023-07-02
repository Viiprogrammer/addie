package app

import (
	"bytes"
	"strconv"
	"strings"
)

type runtimeConfig struct {
	lotteryChance     []byte
	qualityLevel      []byte
	blocklistIps      []byte
	blocklistSwitcher []byte
	limiterSwitch     []byte
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

func (m *App) applyRuntimeConfig(cfg *runtimeConfig) (e error) {
	if len(cfg.lotteryChance) != 0 {
		if e = m.applyLotteryChance(cfg.lotteryChance); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (lottery chance)")
		}
	}

	if len(cfg.qualityLevel) != 0 {
		if e = m.applyQualityLevel(cfg.qualityLevel); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (quality level)")
		}
	}

	if len(cfg.blocklistIps) != 0 {
		if e = m.applyBlocklistChanges(cfg.blocklistIps); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (blocklist ips)")
		}
	}

	if len(cfg.blocklistSwitcher) != 0 {
		if e = m.applyBlocklistSwitch(cfg.blocklistSwitcher); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (blocklist switch)")
		}
	}

	if len(cfg.limiterSwitch) != 0 {
		if e = m.applyLimitterSwitch(cfg.limiterSwitch); e != nil {
			gLog.Error().Err(e).Msg("could not apply runtime configuration (limiter switch)")
		}
	}

	return
}

func (m *App) applyBlocklistChanges(input []byte) (e error) {
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

func (*App) applyBlocklistSwitch(input []byte) (e error) {
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

func (*App) applyLimitterSwitch(input []byte) (e error) {
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

func (*App) applyLotteryChance(input []byte) (e error) {
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

func (*App) applyQualityLevel(input []byte) (e error) {
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
