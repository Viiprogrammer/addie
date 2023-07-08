package app

import (
	"bytes"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rs/zerolog"
)

var (
	upstreamLocker sync.RWMutex
	balancerLocker sync.RWMutex
)

type (
	balancerRouter   map[string]string
	balancerUpstream map[string]*server
	balancer         struct {
		router   *balancerRouter
		upstream *balancerUpstream

		balancer []net.IP

		midx int64
	}
	server struct {
		name string

		isActive      bool
		statusChanged time.Time

		sync.RWMutex
		lastRequestTime time.Time
		proxiedRequests uint64
	}
)

func (m balancerUpstream) get(k string) (v *server) {
	upstreamLocker.RLock()
	v = m[k]
	upstreamLocker.RUnlock()

	return
}

func (m balancerUpstream) set(k string, v *server) {
	upstreamLocker.Lock()
	m[k] = v
	upstreamLocker.Unlock()
}

func (m balancerUpstream) copy() (bupstream balancerUpstream) {
	upstreamLocker.RLock()
	bupstream = m
	upstreamLocker.RUnlock()

	return
}

func (m balancerUpstream) resetStats() {
	upstreamLocker.Lock()

	for _, server := range m {
		server.resetStat()
	}

	upstreamLocker.Unlock()
}

func newServer(name string) *server {
	return &server{
		name:            name,
		lastRequestTime: time.Now(),

		isActive:      true,
		statusChanged: time.Now(),
	}
}

func (m *server) updateStat() {
	m.Lock()

	m.proxiedRequests = m.proxiedRequests + 1
	m.lastRequestTime = time.Now()

	gLog.Trace().Msgf("new server request #%d in %s", m.proxiedRequests, m.lastRequestTime.String())
	m.Unlock()
}

func (m *server) resetStat() {
	m.Lock()

	m.proxiedRequests = 0
	m.lastRequestTime = time.Time{}

	gLog.Trace().Msg("server's stats was resetted")
	m.Unlock()
}

func (m *server) disableServer() {
	m.Lock()
	defer m.Unlock()

	m.isActive = false
	m.statusChanged = time.Now()
}

func (m *server) enableServer() {
	m.Lock()
	defer m.Unlock()

	m.isActive = true
	m.statusChanged = time.Now()
}

func newBalancer() *balancer {
	router, upstream := make(balancerRouter), make(balancerUpstream)
	return &balancer{
		router:   &router,
		upstream: &upstream,
	}
}

func (m *balancer) updateUpstream(servers map[string]net.IP) {
	gLog.Debug().Msg("upstream update triggered")
	gLog.Trace().Interface("servers", servers).Msg("")

	var newbalancer []net.IP
	for hostname, ip := range servers {
		if server := m.upstream.get(ip.String()); server == nil {
			server = newServer(hostname)
			m.upstream.set(ip.String(), server)
		}

		gLog.Debug().Msgf("new server appending to balancer : %s", ip.String())
		newbalancer = append(newbalancer, ip)
	}

	sort.Slice(newbalancer, func(i, j int) bool {
		return newbalancer[i].String() < newbalancer[j].String()
	})

	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		gLog.Trace().Msg("sorted upstream debug")

		for _, v := range newbalancer {
			gLog.Trace().Msgf("upstream server - %s", v.String())
		}
	}

	m.commitUpstream(&newbalancer)
}

// get new servers, then check upstream existance. Set isActive = true
// get current servers, then check current server existance in new servers; if !found = set isActive = false
func (m *balancer) updateUpstream2(servers map[string]net.IP) {
	var keys []string
	for k := range servers {
		keys = append(keys, k)
	}

	upstream := m.upstream.copy()

	// check new servers; if no server in current upstream - add new server to it
	for k, v := range servers {
		if _, found := upstream[k]; !found {
			srv := newServer(k)
			upstream[v.String()] = srv
		}
	}

	// check current upstream; if there is no server in a new upstream - set server.isAlive = false
	for k := range upstream {
		if _, found := servers[k]; !found {
			upstream[k].isActive = false
			upstream[k].statusChanged = time.Now()
		}
	}
}

func (m *balancer) commitUpstream(newbalancer *[]net.IP) {
	gLog.Info().Msg("balancer - new upstream commiting...")

	balancerLocker.Lock()
	m.balancer = *newbalancer
	balancerLocker.Unlock()

	balancerLocker.RLock()
	m.midx = int64(len(*newbalancer))
	balancerLocker.RUnlock()

	gLog.Debug().Msgf("new list has been commited, srvs: %d", m.midx)
}

func (m *balancer) getServer(key string) (s *server) {
	if s = m.upstream.get(key); s == nil {
		return
	}

	s.updateStat()
	return
}

func (m *balancer) getUpstreamStats() io.ReadWriter {
	tb := table.NewWriter()
	defer tb.Render()

	buf := bytes.NewBuffer(nil)
	tb.SetOutputMirror(buf)
	tb.AppendHeader(table.Row{
		"Address", "Name", "Requests", "LastRequest",
	})

	var upstream *balancerUpstream
	balancerLocker.RLock()
	upstream = m.upstream
	balancerLocker.RUnlock()

	for ip, server := range *upstream {
		tb.AppendRow([]interface{}{
			ip, server.name, server.proxiedRequests, server.lastRequestTime.String(),
		})
	}

	tb.SortBy([]table.SortBy{
		{Number: 3, Mode: table.Dsc},
	})

	tb.Style().Options.SeparateRows = true

	return buf
}

func (m *balancer) getServerByChunkName(prefix, chunk string) (_ string, server *server) {
	var buf string

	if strings.Contains(chunk, "_") {
		buf = strings.Split(chunk, "_")[1]
	} else if strings.Contains(chunk, "fff") {
		buf = strings.ReplaceAll(chunk, "fff", "")
	} else {
		gLog.Warn().Msgf("could not get server because of invalid chunk name '%s'; fallback to legacy balancing", chunk)
		return
	}

	sid, e := strconv.Atoi(prefix + buf)
	if e != nil {
		gLog.Warn().Err(e).Msgf("could not get server because of Atoi error (%s); fallback to legacy balancing", chunk)
		return
	}

	var ip *net.IP
	if ip = m.getNextServer(sid); ip == nil {
		return
	}

	return ip.String(), m.getServer(ip.String())
}

func (m *balancer) getNextServer(idx int) (_ *net.IP) {
	if !balancerLocker.TryRLock() {
		gLog.Warn().Msg("could not get lock for reading upstream; fallback to legacy balancing")
		return nil
	}
	defer balancerLocker.RUnlock()

	if m.midx == 0 {
		return
	}

	return &m.balancer[idx%int(m.midx)]
}

func (m *balancer) resetServersStats() {
	gLog.Debug().Msg("upstream reset stats called")

	balancerLocker.Lock()
	m.upstream.resetStats()
	balancerLocker.Unlock()
}
