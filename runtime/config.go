package runtime

import (
	"errors"
	"sync"
	"time"

	"github.com/MindHunter86/addie/utils"
)

type (
	ConfigEid   uint8
	ConfigParam uint8

	// ConfigEntry   map[ConfigEid]interface{}
	ConfigStorage map[ConfigParam]*ConfigEntry

	ConfigEntry struct {
		sync.RWMutex

		Payload interface{}
		Target  interface{}
		Step    int
	}
)

const (
	ConfigParamLottery ConfigParam = iota
	ConfigParamQuality
	ConfigParamBlocklist
	ConfigParamBlocklistIps
	ConfigParamLimiter

	_configParamMaxSize
)

var ConfigParamDefaults = map[ConfigParam]interface{}{
	ConfigParamLottery:      0,
	ConfigParamQuality:      utils.TitleQualityFHD,
	ConfigParamBlocklist:    0,
	ConfigParamBlocklistIps: []string{},
	ConfigParamLimiter:      0,
}

const (
	configEntryLocker  ConfigEid = iota // sync.RWMutex
	configEntryPayload                  // interface{}
	configEntryTarget                   // interface{}
	configEntryStep                     // int

	_configEntryMaxSize
)

var sLocker sync.RWMutex

var deployStep int
var deployInteration time.Duration

var (
	ErrConfigStorageLockFailure = errors.New("config storage - could not lock storage")
	ErrConfigEntryLockFailure   = errors.New("config storage - could not lock entry")
	ErrConfigInvalidParam       = errors.New("config storage - invalid param or internal map error")
)

func NewConfigStorage() ConfigStorage {
	deployStep = ccx.Int("balancer-softer-step")
	deployInteration = ccx.Duration("balancer-softer-tick")

	return make(ConfigStorage, _configParamMaxSize)
}

func (m ConfigStorage) GetValue(param ConfigParam) (val interface{}, ok bool, e error) {
	return m.getEntry(param)
}

func (m ConfigStorage) SetValue(param ConfigParam, val interface{}) (e error) {
	var ok bool
	var entry *ConfigEntry

	if entry, ok, e = m.getEntry(param); e != nil {
		return
	} else if !ok {
		entry, ok = newConfigEntry(param), true
	}

	if !entry.TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer entry.Unlock()

	entry.Payload = val

	return m.setEntry(param, entry)
}

func (m ConfigStorage) SetValueSmoothly(param ConfigParam, val interface{}) (e error) {
	var ok bool
	var entry *ConfigEntry

	// os.Exit(1)
	// if value != target - continue
	// !!!
	// !!!
	// !!!

	if entry, ok, e = m.getEntry(param); e != nil {
		return
	} else if !ok {
		entry, ok = newConfigEntry(param), true
	}

	entry.nextDeployStep(true)

	if !entry.TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer entry.Unlock()

	entry.Target = val
	go entry.bootstrapDeploy()

	return m.setEntry(param, entry)
}

//

func (m ConfigStorage) getEntry(param ConfigParam) (entry *ConfigEntry, ok bool, e error) {
	if !sLocker.TryRLock() {
		e = ErrConfigStorageLockFailure
		return
	}
	defer sLocker.RUnlock()

	entry, ok = m[param]
	return
}

func (m ConfigStorage) setEntry(param ConfigParam, entry *ConfigEntry) (e error) {
	if !sLocker.TryLock() {
		e = ErrConfigStorageLockFailure
		return
	}
	defer sLocker.Unlock()

	m[param] = entry
	return
}

//

func newConfigEntry(param ConfigParam) *ConfigEntry {
	return &ConfigEntry{
		Payload: ConfigParamDefaults[param],
		Step:    -1,
	}
}

func (m *ConfigEntry) bootstrapDeploy() error {
	log.Trace().Msg("smooth deploy has been started")
	defer log.Trace().Msg("smooth deploy has been stopped")

	ticker := time.NewTicker(deployInteration)

loop:
	for {
		select {
		case <-ticker.C:
			if ok, e := m.nextDeployStep(); e != nil {
				log.Warn().Err(e).Msg("smooth_deploy - there some problems in runtime config deploying")
				continue
			} else if ok {
				log.Debug().Msg("smooth_deploy - deploy prcess has been completed")
				ticker.Stop()
				break loop
			}
			log.Trace().Msg("smooth_deploy - tick called, descrease entry's steps")
		case <-ctx.Done():
			log.Trace().Msg("smooth_deploy - internal abort() has been caught")
			break loop
		}
	}

	return m.commitTargetValue()
}

func (m *ConfigEntry) getLotteryResult(key int) (val interface{}, ok bool) {
	switch key % m.Step {
	case 0:
		val = m.Target
	default:
		val = m.Payload
	}

	return
}

func (m *ConfigEntry) getPayload(bkey ...int) (val interface{}, ok bool, e error) {
	bkey = append(bkey, 0)

	if !m.TryRLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer m.Unlock()

	switch m.Step {
	case -1:
		val, ok = m.getLotteryResult(bkey[0])
	default:
		val = m.Payload
	}

	return
}

func (m *ConfigEntry) nextDeployStep(init ...bool) (_ bool, e error) {
	init = append(init, false)

	if !m.TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer m.Unlock()

	if init[0] {
		m.Step = deployStep
		return
	}

	log.Trace().Int("step", m.Step).Msg("")
	m.Step = m.Step - 1
	return m.Step == 0, e
}

func (m *ConfigEntry) commitTargetValue() (e error) {
	if !m.TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}

	log.Debug().Interface("old", m.Payload).Interface("new", m.Target).
		Msg("config entry - new value has been commited")

	m.Payload, m.Step = m.Target, -1
	m.Unlock()
	return
}
