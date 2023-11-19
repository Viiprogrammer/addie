package balancer

import (
	"net"
	"sort"
	"sync"
)

type upstream map[string]*BalancerServer

var uplock sync.RWMutex

func (m upstream) getServer(ip string) (server *BalancerServer, ok bool) {
	uplock.RLock()
	defer uplock.RUnlock()

	server, ok = m[ip]
	return
}

func (m upstream) putServer(ip string, server *BalancerServer) {
	uplock.Lock()
	defer uplock.Unlock()

	m[ip] = server
}

func (m upstream) copy() (_ map[string]*BalancerServer) {
	var buf = make(map[string]*BalancerServer)

	uplock.RLock()
	defer uplock.RUnlock()

	buf = m
	return buf
}

func (m upstream) resetServersStats() {
	buf := m.copy()

	for _, server := range buf {
		server.resetStats()
	}
}

func (m upstream) getServers() (servers []*BalancerServer) {
	buf := m.copy()

	for _, server := range buf {
		servers = append(servers, server)
	}

	return
}

func (m upstream) getIps() (ips []*net.IP, _ int) {
	buf := m.copy()

	for _, server := range buf {
		ips = append(ips, &server.Ip)
	}

	sort.Slice(ips, func(i, j int) bool {
		return ips[i].String() < ips[j].String()
	})

	return ips, len(ips)
}
