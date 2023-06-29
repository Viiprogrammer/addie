package app

import "sync"

type blocklist []string

var blLocker sync.RWMutex

func (m blocklist) push(ips ...string) {
	if ips = append(ips, ""); ips[0] == "" {
		gLog.Warn().Interface("ips", ips).Msg("internal error, empty slice in blocklist")
		return
	}

	gLog.Trace().Strs("ips", ips).Msg("blocklist push has been called")

	blLocker.Lock()
	for _, ip := range ips {
		m = append(m, ip)
	}
	blLocker.Unlock()
}

func (m blocklist) reset() bool {
	blLocker.Lock()
	defer blLocker.Unlock()

	gLog.Trace().Msg("blocklist reset has been called")

	m = nil
	return len(m) == 0
}

func (m blocklist) update(ips ...string) bool {
	if ips = append(ips, ""); ips[0] == "" {
		gLog.Warn().Interface("ips", ips).Msg("internal error, empty slice in blocklist")
		return false
	}

	gLog.Trace().Strs("ips", ips).Msg("blocklist update has been called")

	if !m.reset() {
		gLog.Warn().Msg("internal error, blocklist reset failure")
		return false
	}

	m.push(ips...)
	return true
}

func (m blocklist) isExists(ip string) (ok bool) {
	if ip == "" {
		gLog.Warn().Str("ip", ip).Msg("internal error, empty string in blocklist")
		return
	}

	gLog.Trace().Str("ip", ip).Msg("blocklist isExists has been called")

	blLocker.RLock()
	for _, v := range m {
		if v == ip {
			ok = true
		}
	}
	blLocker.RUnlock()

	return
}

func (m blocklist) size() (size int) {
	gLog.Trace().Msg("blocklist size has been called")

	blLocker.RLock()
	size = len(m)
	blLocker.RUnlock()

	return
}

// type blocklist struct {
// 	locker sync.RWMutex
// 	list   map[netip.Addr]time.Time

// 	ticklock sync.Mutex
// 	ticker   *time.Ticker
// 	wg       sync.WaitGroup

// 	isEnabled bool
// }

// func newBlocklist(enabled ...bool) *blocklist {
// 	enabled = append(enabled, false) // add default

// 	return &blocklist{
// 		list:      make(map[netip.Addr]time.Time),
// 		isEnabled: enabled[0],
// 	}
// }

// func (m *blocklist) run(done func()) {
// 	defer done()

// 	if !m.isEnabled {
// 		return
// 	}

// 	gLog.Debug().Msg("strating blocklist loop...")
// 	m.ticker = time.NewTicker(time.Minute)

// loop:
// 	for {
// 		select {
// 		case <-gCtx.Done():
// 			gLog.Warn().Msg("context done() has been caught; stopping cron subservice...")
// 			break loop
// 		case <-m.ticker.C:
// 			gLog.Trace().Msg("start searching of expired bans...")

// 			m.wg.Add(1)
// 			m.findExpiredBans(m.wg.Done)
// 		}
// 	}

// 	m.ticker.Stop()

// 	gLog.Debug().Msg("waiting for goroutines with blocklist jobs...")
// 	m.wg.Wait()
// }

// func (m *blocklist) findExpiredBans(done func()) {
// 	defer done()

// 	if !m.ticklock.TryLock() {
// 		gLog.Debug().Msg("could not get lock for the job, seems previous call has not been completed yet")
// 		return
// 	}
// 	defer m.ticklock.Unlock()

// 	m.locker.RLock()

// 	blist := m.list
// 	m.locker.RUnlock()

// 	now := time.Now()
// 	for addr, time := range blist {
// 		if time.Before(now) {
// 			gLog.Debug().Str("ip", addr.String()).Time("now", now).Time("ban_time", time).Msg("unblocking ip address...")
// 			m.pop(addr)
// 		}
// 	}
// }

// func (m *blocklist) push(ip []byte) bool {
// 	if !m.isEnabled {
// 		return true
// 	}

// 	addr, ok := netip.AddrFromSlice(ip)
// 	if !ok {
// 		return ok
// 	}

// 	m.locker.RLock()

// 	m.list[addr] = time.Now().Add(gCli.Duration("ip-ban-time"))
// 	m.locker.RUnlock()

// 	gLog.Debug().Str("addr", addr.String()).Time("ban_time", time.Now().Add(gCli.Duration("ip-ban-time"))).
// 		Msg("ip addres has been added to blocklist")
// 	return ok
// }

// func (m *blocklist) pop(addr netip.Addr) {
// 	if !m.isEnabled {
// 		return
// 	}

// 	m.locker.Lock()

// 	delete(m.list, addr)
// 	m.locker.Unlock()

// 	gLog.Debug().Str("addr", addr.String()).Msg("ip addres has been remove from blocklist")
// }

// func (m *blocklist) isExists(ip []byte) (ok bool) {
// 	if !m.isEnabled {
// 		return false
// 	}

// 	var addr netip.Addr
// 	if addr, ok = netip.AddrFromSlice(ip); !ok {
// 		return false
// 	}

// 	m.locker.RLock()

// 	_, ok = m.list[addr]
// 	m.locker.RUnlock()

// 	return
// }
