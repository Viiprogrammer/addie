package app

import (
	"net"
	"sync"
	"time"
)

type balancer struct {
	serversLock sync.RWMutex
	servers     map[*net.IP]*server

	idx, midx int
	blistLock sync.RWMutex
	blist     *balancerList

	routerLock sync.RWMutex
	router     *router
}

type server struct {
	isDead bool

	// stats
	proxiedReqs uint64
	lastRequest *time.Time
}

type router map[string]*net.IP

func (m router) get(l *sync.RWMutex, k string) (ip *net.IP) {
	l.RLock()
	ip = m[k]
	l.RUnlock()

	return ip
}

func (m router) set(l *sync.RWMutex, k string, ip *net.IP) {
	l.Lock()
	m[k] = ip
	l.Unlock()
}

type balancerList []*net.IP

func (m balancerList) pop(l *sync.RWMutex, i int) (ip *net.IP) {
	l.RLock()
	ip = m[i]
	l.RUnlock()

	return
}

func (m balancerList) push(l *sync.RWMutex, ip *net.IP) {
	l.Lock()
	m = append(m, ip)
	l.Unlock()
}

func newBalancer() *balancer {
	return &balancer{
		servers: make(map[*net.IP]*server),
	}
}

func (m *balancer) addPassingServer(ip *net.IP) {
	m.blist.push(&m.blistLock, ip)
}

func (m *balancer) getPassingServer() *net.IP {
	if m.idx = m.idx << 1; m.idx > m.midx {
		m.idx = 0
	}

	return m.blist.pop(&m.blistLock, m.idx)
}

func (m *balancer) getServerByKey(key string) (ip *net.IP) {
	if ip = m.router.get(&m.routerLock, key); ip != nil {
		return
	}

	ip = m.getPassingServer()

	m.router.set(&m.routerLock, key, ip)
	return
}
