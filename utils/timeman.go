package utils

import (
	"context"
	"sync"
	"time"
)

type TimeManager struct {
	tick *time.Ticker

	sync.RWMutex
	now time.Time
}

func NewTimeManager() *TimeManager {
	return &TimeManager{}
}

func (m *TimeManager) Bootstrap(ctx context.Context) {
	m.tick = time.NewTicker(time.Second)
	defer m.tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.tick.C:
			m.updateCurrentTime()
		}
	}
}

func (m *TimeManager) Now() (t time.Time) {
	if !m.TryRLock() {
		return time.Now()
	}
	defer m.RUnlock()

	t = m.now
	return
}

func (m *TimeManager) updateCurrentTime() {
	m.Lock()
	defer m.Unlock()

	m.now = time.Now().Round(time.Second)
}
