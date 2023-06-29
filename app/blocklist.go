package app

import "sync"

type blocklist []string

var blLocker sync.RWMutex

func newBlocklist() *blocklist {
	return &blocklist{}
}

func (m *blocklist) reset() {
	*m = blocklist{}
}

func (m *blocklist) push(ips ...string) {
	if len(ips) == 0 {
		gLog.Warn().Interface("ips", ips).Msg("internal error, empty slice in blocklist")
		return
	}

	gLog.Trace().Strs("ips", ips).Msg("blocklist push has been called")

	blLocker.Lock()
	defer blLocker.Unlock()

	m.reset()

	for _, ip := range ips {
		if ip == "" {
			continue
		}

		gLog.Trace().Str("ip", ip).Msg("new ip commited to blocklist")
		*m = append(*m, ip)
	}

	gLog.Trace().Strs("ips", ips).Msg("blocklist push has been called")
}

func (m *blocklist) isExists(ip string) (ok bool) {
	if ip == "" {
		gLog.Warn().Str("ip", ip).Msg("internal error, empty string in blocklist")
		return
	}

	gLog.Trace().Str("ip", ip).Msg("blocklist isExists has been called")

	blLocker.RLock()
	for _, v := range *m {
		if v == ip {
			ok = true
		}
	}
	blLocker.RUnlock()

	return
}

func (m *blocklist) size() (size int) {
	gLog.Trace().Msg("blocklist size has been called")

	blLocker.RLock()
	size = len(*m)
	blLocker.RUnlock()

	return
}
