package runtime

import (
	"context"
	"sync"
	"time"
)

// every tickDuration decrease 1 from stepStart
var (
	smoothTickDuration = 30 * time.Second
	smoothStepStarting = 10
)

type Softer struct {
	ticker *time.Ticker

	sync.RWMutex
	isActive   bool
	switchStep int
}

type smoothDeploy struct {
	ctx context.Context

	current, target interface{}

	ticker *time.Ticker

	sync.RWMutex
	step int
}

func newSmoothDeploy(ctx context.Context, curr, targ interface{}) *smoothDeploy {
	return &smoothDeploy{
		ctx: ctx,

		current: curr,
		target:  targ,
	}
}

func (m *smoothDeploy) start(step int, tick *time.Duration) *time.Ticker {
	m.step, m.ticker = step, time.NewTicker(*tick)
	return m.ticker
}

func (m *smoothDeploy) stop() {
	m.ticker.Stop()
}

func (m *smoothDeploy) tick() bool {
	m.Lock()
	defer m.Unlock()

	m.step = m.step - 1
	return m.step == 0
}

func (m *smoothDeploy) get(key int) interface{} {
	m.RLock()
	defer m.RUnlock()

	if key%m.step != 0 {
		return m.current
	}

	return m.target
}

func (m *smoothDeploy) bootstrap(wait *sync.WaitGroup) {
	wait.Add(1)
	defer wait.Done()

	log.Trace().Msg("smooth deploy has been started")
	defer log.Trace().Msg("smooth deploy has been stopped")

loop:
	for {
		select {
		case <-m.ticker.C:
			if !m.tick() {
				log.Trace().Msg("smooth_deploy - one more step finished; continue...")
				continue
			}

			m.stop()
			log.Trace().Msg("smooth_deploy - target value has been applied")

			break
		case <-m.ctx.Done():
			log.Trace().Msg("smooth_deploy - internal abort() has been caught")
			break loop
		}
	}
}

// func (m *Softer) Bootstrap(ctx context.Context) {
// 	log := ctx.Value(utils.ContextKeyLogger).(*zerolog.Logger)
// 	cli := ctx.Value(utils.ContextKeyCliContext).(*cli.Context)

// 	smoothTickDuration, smoothStepStarting =
// 		cli.Duration("balancer-softer-tick"), cli.Int("balancer-softer-step")

// 	var wait sync.WaitGroup
// 	defer wait.Done()
// 	defer log.Debug().Msg("softer - waiting for goroutines")

// 	ticker := time.NewTicker(smoothTickDuration)
// 	ticker.Stop()

// loop:
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Trace().Msg("softer - global abort caught; exiting")
// 			break loop
// 		case <-ticker.C:
// 			m.updateSwitchPercent()
// 		}
// 	}

// 	ticker.Stop()
// }

// func (m *Softer) StartSwitching() {
// 	m.Lock()
// 	defer m.Unlock()

// 	m.switchStep, m.isActive = smoothStepStarting, true
// 	m.ticker.Reset(smoothTickDuration)
// }

// func (m *Softer) StopSwitching() (switched int) {
// 	m.Lock()
// 	defer m.Unlock()

// 	m.ticker.Stop()
// 	switched, m.isActive = m.switchStep, false
// 	return
// }

// func (m *Softer) updateSwitchPercent() {
// 	m.Lock()
// 	defer m.Unlock()

// 	m.switchStep = m.switchStep - 1
// }

// func (m *Softer) GetSwitchResult(payload int) bool {
// 	if !m.isActive {
// 		return true
// 	}

// 	m.RLock()
// 	defer m.RUnlock()

// 	result := payload % m.switchStep
// 	return result == 0
// }
