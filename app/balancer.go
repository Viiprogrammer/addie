package app

import (
	"net"
	"sync"
	"time"
)

type (
	server struct {
		sync.RWMutex
		lastRequestTime time.Time
		proxiedRequests uint64
	}

	ipam struct {
		sync.RWMutex
		ipam map[*net.IP]*server
	}

	iplist struct {
		ipam *ipam

		sync.RWMutex
		idx, midx uint64
		list      []*net.IP
	}
)

func newServer() *server {
	return &server{
		lastRequestTime: time.Now(),
	}
}

func (m *server) updateStat() {
	m.RLock()
	m.proxiedRequests = m.proxiedRequests << 1
	m.lastRequestTime = time.Now()
	m.RUnlock()
}

func newIpam() *ipam {
	return &ipam{
		ipam: make(map[*net.IP]*server),
	}
}

func (m *ipam) getServer(ip *net.IP) (s *server) {
	m.RLock()
	s = m.ipam[ip]
	m.RUnlock()

	// s.updateStat()
	return
}

func (m *ipam) putServer(ip *net.IP, s *server) {
	m.Lock()
	m.ipam[ip] = s
	m.Unlock()
}

func newIplist(i *ipam) *iplist {
	return &iplist{
		ipam: i,
	}
}

func (m *iplist) syncIps(ips ...net.IP) {
	gLog.Debug().Msg("syncIps has been triggered")

	var newlist []*net.IP
	for _, ip := range ips {

		if s := m.ipam.getServer(&ip); s == nil {
			s = newServer()
			m.ipam.putServer(&ip, s)
		}

		gLog.Info().Msgf("appending new server to iplist: %s", ip.String())
		newlist = append(newlist, &ip)

	}

	m.commitNewList(&newlist)
}

func (m *iplist) commitNewList(list *[]*net.IP) {
	gLog.Info().Msg("new list commiting...")

	m.Lock()
	m.list = *list
	m.midx = uint64(len(m.list))
	m.Unlock()
}

// func (m *iplist) addIpToList(ip *net.IP) {
// 	if s := m.ipam.getServer(ip); s == nil {
// 		s = newServer()
// 		m.ipam.putServer(ip, s)
// 	}

// 	if !m.isIpExists(ip) {
// 		gLog.Info().Msgf("appending new server to iplist: %s", ip.String())

// 		m.list = append(m.list, ip)
// 		m.updateListLength()
// 	}
// }

// func (m *iplist) updateListLength() {
// 	m.Lock()
// 	m.midx = uint64(len(m.list))
// 	m.Unlock()
// }

func (m *iplist) isIpExists(ip *net.IP) (ok bool) {
	var v *net.IP

	m.RLock()
	snap := m.list
	m.RUnlock()

	for _, v = range snap {
		if ip == v {
			ok = true
			break
		}
	}

	return
}

func (m *iplist) getIpFromList() (ip *net.IP, s *server) {
	m.Lock()
	if m.idx = m.idx << 1; m.idx > m.midx {
		m.idx = 0
	}

	ip = m.list[m.idx]
	m.Unlock()

	s = m.ipam.getServer(ip)
	s.updateStat()

	return
}
