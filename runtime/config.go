package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/MindHunter86/addie/utils"
	"github.com/urfave/cli/v2"
)

var (
	ErrConfigStorageLockFailure = errors.New("config storage - could not lock storage")
	ErrConfigEntryLockFailure   = errors.New("config storage - could not lock entry")
	ErrConfigInvalidParam       = errors.New("config storage - invalid param or internal map error")
	ErrConfigInvalidStep        = errors.New("config storage - softer-step must be >= 0 and < 100")
)

type StorageParam uint8

const (
	ParamLottery StorageParam = iota
	ParamQuality
	ParamBlocklist
	ParamBlocklistIps
	ParamLimiter
	ParamA5bility
	ParamStdoutAccess

	paramMaxSize // used only for make(maxvalue)
)

var ParamDefaults = map[StorageParam]interface{}{
	ParamLottery:      100,
	ParamQuality:      utils.TitleQualityHD,
	ParamBlocklist:    1,
	ParamBlocklistIps: []string{},
	ParamLimiter:      0,
	ParamA5bility:     90,
	ParamStdoutAccess: 0,
}

var GetNameByParam = map[StorageParam]string{
	ParamLottery:      "lottery",
	ParamQuality:      "quality",
	ParamBlocklist:    "blocklist",
	ParamBlocklistIps: "blocklist_ips",
	ParamLimiter:      "limiter",
	ParamA5bility:     "cluster-availability",
	ParamStdoutAccess: "stdout-access-log",
}

type Storage struct {
	mu sync.RWMutex
	st map[StorageParam]*Entry
}

var (
	done func() <-chan struct{}

	deployStep       int
	deployInteration time.Duration
)

func NewStorage(c context.Context) (st *Storage, _ error) {
	done = c.Done

	ccx := c.Value(utils.ContextKeyCliContext).(*cli.Context)
	deployStep, deployInteration =
		ccx.Int("balancer-softer-step"),
		ccx.Duration("balancer-softer-tick")

	if deployStep < 0 || deployStep > 99 {
		return nil, ErrConfigInvalidStep
	}

	if deployInteration < 10*time.Second {
		log.Warn().Msg("low value detected for softer-tick arg")
	}

	st = &Storage{
		st: make(map[StorageParam]*Entry, paramMaxSize),
	}

	st.loadDefaults()
	return
}

func (m *Storage) loadDefaults() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for param, val := range ParamDefaults {
		e := newEntry(val)
		m.st[param] = e

		log.Trace().Msgf("loaded default value for %s - %+v", GetNameByParam[param], val)
	}
}

func (m *Storage) getEntry(param StorageParam) (e *Entry, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok = m.st[param]
	return e, ok && e != nil
}

func (m *Storage) setEntryByValue(param StorageParam, val interface{}) (e *Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if val == nil {
		val = ParamDefaults[param]
	}

	e = newEntry(val)
	m.st[param] = e

	log.Trace().Msgf("config param %s updated with value %+v", GetNameByParam[param], val)
	return
}

func (m *Storage) Get(param StorageParam) interface{} {
	if e, ok := m.getEntry(param); ok {
		return e.get()
	}

	// TODO: remove this
	panic("undefined entry or param not found in config storage")
}

func (m *Storage) Set(param StorageParam, val interface{}) {
	e, ok := m.getEntry(param)
	if !ok && e != nil {
		panic("value is not nil but param not found in config storage")
	} else if !ok {
		m.setEntryByValue(param, val)
		return
	}

	e.set(val)
}

func (m *Storage) SetSmoothly(param StorageParam, val interface{}) {
	e, ok := m.getEntry(param)
	if !ok && e != nil {
		panic("value is not nil but param not found in config storage")
	} else if !ok {
		m.setEntryByValue(param, val)
		return
	}

	if deployStep == 0 {
		e.set(val)
		return
	}

	go e.deploy(val)
}
