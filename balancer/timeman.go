package balancer

import (
	"context"
	"sync"
	"time"
)

var gTimer *timeManager

type timeManager struct {
	tick *time.Ticker

	sync.RWMutex
	now time.Time
}

func (m *timeManager) bootstrap(ctx context.Context) {
	m.tick = time.NewTicker(1 * time.Second)
	defer m.tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.tick.C:
			m.getCurrentTime()
		}
	}
}

func (m *timeManager) getCurrentTime() {
	m.Lock()
	defer m.Unlock()

	m.now = time.Now().Round(time.Second)
}

func (m *timeManager) time() (t time.Time) {
	m.RLock()
	defer m.RUnlock()

	t = m.now
	return
}
