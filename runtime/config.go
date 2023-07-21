package runtime

import (
	"errors"
	"sync"
	"time"
)

type (
	ConfigEid   uint8
	ConfigParam uint8

	ConfigEntry   map[ConfigEid]interface{}
	ConfigStorage map[ConfigParam]ConfigEntry
)

const (
	ConfigParamLottery ConfigParam = iota
	ConfigParamQuality
	ConfigParamBlocklist
	ConfigParamBlocklistIps
	ConfigParamLimiter

	_configParamSize
)

var ConfigParamDefaults = map[ConfigParam]interface{}{
	ConfigParamLottery:      0,
	ConfigParamQuality:      1080,
	ConfigParamBlocklist:    0,
	ConfigParamBlocklistIps: []string{},
	ConfigParamLimiter:      0,
}

const (
	configEntryLocker  ConfigEid = iota //sync.RWMutex
	configEntryPayload                  // interface{}
	configEntryTarget                   // interface{}
	configEntryStep                     // int

	_configEntrySize
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

	return make(ConfigStorage, _configParamSize)
}

func (m ConfigStorage) GetValue(param ConfigParam) (val interface{}, ok bool, e error) {
	var entry ConfigEntry
	if entry, ok, e = m.getEntry(&param); e != nil || !ok {
		return
	}

	return entry.getPayload()
}

func (m ConfigStorage) SetValue(param ConfigParam, val interface{}) (e error) {
	var ok bool
	var entry ConfigEntry

	if entry, ok, e = m.getEntry(&param); e != nil {
		return
	} else if !ok {
		entry, ok = newEntry(&param), true
	}

	if !entry[configEntryLocker].(*sync.RWMutex).TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}

	entry[configEntryPayload] = val
	entry[configEntryLocker].(*sync.RWMutex).Unlock()

	return m.setEntry(&param, entry)
}

func (m ConfigStorage) SetValueSmoothly(param ConfigParam, val interface{}) (e error) {
	var ok bool
	var entry ConfigEntry

	if entry, ok, e = m.getEntry(&param); e != nil {
		return
	} else if !ok {
		entry, ok = newEntry(&param), true
	}

	entry.nextDeployStep(true)

	if !entry[configEntryLocker].(*sync.RWMutex).TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer entry[configEntryLocker].(*sync.RWMutex).Unlock()

	entry[configEntryTarget] = val
	go entry.bootstrapDeploy()

	return m.setEntry(&param, entry)
}

//

func (m ConfigStorage) getEntry(param *ConfigParam) (entry ConfigEntry, ok bool, e error) {
	if !sLocker.TryRLock() {
		e = ErrConfigStorageLockFailure
		return
	}
	defer sLocker.RUnlock()

	entry, ok = m[*param]
	return
}

func (m ConfigStorage) setEntry(param *ConfigParam, entry ConfigEntry) (e error) {
	if !sLocker.TryLock() {
		e = ErrConfigStorageLockFailure
		return
	}
	defer sLocker.Unlock()

	m[*param] = entry
	return
}

//

func newEntry(param *ConfigParam) ConfigEntry {
	entry := make(ConfigEntry, _configEntrySize)

	entry[configEntryLocker] = new(sync.RWMutex)
	entry[configEntryPayload] = ConfigParamDefaults[*param]
	entry[configEntryStep] = -1

	return entry
}

func (m ConfigEntry) bootstrapDeploy() error {
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

func (m ConfigEntry) getLotteryResult(key int) (val interface{}, ok bool) {
	switch key % m[configEntryStep].(int) {
	case 0:
		val, ok = m[configEntryTarget]
	default:
		val, ok = m[configEntryPayload]
	}

	return
}

func (m ConfigEntry) getPayload(bkey ...int) (val interface{}, ok bool, e error) {
	bkey = append(bkey, 0)

	if !m[configEntryLocker].(*sync.RWMutex).TryRLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer m[configEntryLocker].(*sync.RWMutex).RUnlock()

	switch m[configEntryStep] {
	case -1:
		val, ok = m.getLotteryResult(bkey[0])
	default:
		val, ok = m[configEntryPayload]
	}

	return
}

func (m ConfigEntry) nextDeployStep(init ...bool) (_ bool, e error) {
	init = append(init, false)

	if !m[configEntryLocker].(*sync.RWMutex).TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}
	defer m[configEntryLocker].(*sync.RWMutex).Unlock()

	if init[0] {
		m[configEntryStep] = deployStep
		return
	}

	log.Trace().Int("step", m[configEntryStep].(int)).Msg("")
	m[configEntryStep] = m[configEntryStep].(int) - 1
	return m[configEntryStep] == 0, e
}

func (m ConfigEntry) commitTargetValue() (e error) {
	if !m[configEntryLocker].(*sync.RWMutex).TryLock() {
		e = ErrConfigEntryLockFailure
		return
	}

	log.Debug().Interface("old", m[configEntryPayload]).Interface("new", m[configEntryTarget]).
		Msg("config entry - new value has been commited")

	m[configEntryPayload], m[configEntryStep] = m[configEntryTarget], -1
	m[configEntryLocker].(*sync.RWMutex).Unlock()
	return
}
