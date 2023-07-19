package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

// every tickDuration decrease 1 from stepStart
var (
	softerTickDuration = 30 * time.Second
	softerStepStarting = 10
)

type Softer struct {
	ticker *time.Ticker

	sync.RWMutex
	isActive   bool
	switchStep int
}

func (m *Softer) Bootstrap(ctx context.Context) {
	log := ctx.Value(utils.ContextKeyLogger).(*zerolog.Logger)
	cli := ctx.Value(utils.ContextKeyCliContext).(*cli.Context)

	softerTickDuration, softerStepStarting =
		cli.Duration("balancer-softer-tick"), cli.Int("balancer-softer-step")

	var wait sync.WaitGroup
	defer wait.Done()
	defer log.Debug().Msg("softer - waiting for goroutines")

	ticker := time.NewTicker(softerTickDuration)
	ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			log.Trace().Msg("softer - global abort caught; exiting")
			break loop
		case <-ticker.C:
			m.updateSwitchPercent()
		}
	}

	ticker.Stop()
}

func (m *Softer) StartSwitching() {
	m.Lock()
	defer m.Unlock()

	m.switchStep, m.isActive = softerStepStarting, true
	m.ticker.Reset(softerTickDuration)
}

func (m *Softer) StopSwitching() (switched int) {
	m.Lock()
	defer m.Unlock()

	m.ticker.Stop()
	switched, m.isActive = m.switchStep, false
	return
}

func (m *Softer) updateSwitchPercent() {
	m.Lock()
	defer m.Unlock()

	m.switchStep = m.switchStep - 1
}

func (m *Softer) GetSwitchResult(payload int) bool {
	if !m.isActive {
		return true
	}

	m.RLock()
	defer m.RUnlock()

	result := payload % m.switchStep
	return result == 0
}
