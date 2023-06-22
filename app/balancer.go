package app

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/jedib0t/go-pretty/v6/table"
)

var (
	routerLocker   sync.RWMutex
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

		idx, midx int64
		//
	}
	server struct {
		name string

		sync.RWMutex
		lastRequestTime time.Time
		proxiedRequests uint64
	}
)

func newServer(name string) *server {
	return &server{
		name:            name,
		lastRequestTime: time.Now(),
	}
}

func (m *server) updateStat() {
	m.RLock()

	m.proxiedRequests = m.proxiedRequests + 1
	m.lastRequestTime = time.Now()

	gLog.Trace().Msgf("new server request #%d in %s", m.proxiedRequests, m.lastRequestTime.String())
	m.RUnlock()
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

	m.commitUpstream(&newbalancer)
}

func (m *balancer) commitUpstream(newbalancer *[]net.IP) {
	gLog.Info().Msg("new list commiting...")

	balancerLocker.Lock()
	m.balancer = *newbalancer
	m.midx = int64(len(*newbalancer))
	// m.router = make(map[string]*net.IP)
	balancerLocker.Unlock()

	gLog.Debug().Msgf("new list has been commited, srvs: %d", m.midx)
}

func (m *balancer) getServer(key string) (s *server) {
	if s = m.upstream.get(key); s == nil {
		return
	}

	s.updateStat()
	return
}

// ! can be empty
func (m *balancer) getOrCreateRouter(key string) (serverip string, server *server) {
	if serverip = m.router.get(key); serverip != "" {
		return serverip, m.getServer(serverip)
	}

	serverip = m.createRoute(key)
	return serverip, m.getServer(serverip)
}

func (m *balancer) createRoute(key string) (server string) {
	var serverip *net.IP
	if serverip = m.getNextServer(); serverip == nil {
		gLog.Warn().Msg("there is no servers in upstream, fallback to legacy balancing...")
		return
	}

	server = serverip.String()

	if ok, e := m.storeRouteToConsul(key, server); e != nil {
		gLog.Error().Err(e).Msg("could not acquire route, fallback to legacy balancing...")
		return
	} else if !ok {
		gLog.Warn().Msg("consul api sent nonok while router storing, trying to get router from cache...")

		if server, e = m.getRouteFromConsul(key); e != nil {
			gLog.Error().Err(e).Msg("could not get route from consul after CAS, fallback to legacy balancing...")
			return
		}
	}

	m.router.set(key, server)
	return
}

func (m *balancer) getNextServer() *net.IP {
	balancerLocker.Lock()
	defer balancerLocker.Unlock()

	if m.midx == 0 {
		return nil
	}

	if m.idx = m.idx + 1; m.idx >= m.midx {
		m.idx = 0
	}

	return &m.balancer[m.idx]
}

func (m *balancer) storeRouteToConsul(key, server string) (ok bool, e error) {
	p := &capi.KVPair{
		Key:   fmt.Sprintf("%s/balancer/%s", gCli.String("consul-kv-prefix"), key),
		Value: []byte(server),
	}

	var meta *capi.WriteMeta
	if ok, meta, e = gConsul.KV().CAS(p, nil); e != nil {
		return
	}

	gLog.Trace().Dur("took", meta.RequestTime).Msgf("consul write wrote with %s status", ok)
	return
}

func (m *balancer) getRouteFromConsul(key string) (value string, e error) {
	var opts = &capi.QueryOptions{
		AllowStale:        true,
		UseCache:          false,
		RequireConsistent: false,
	}

	var kv *capi.KVPair
	var meta *capi.QueryMeta
	ckey := fmt.Sprintf("%s/balancer/%s", gCli.String("consul-kv-prefix"), key)
	if kv, meta, e = gConsul.KV().Get(ckey, opts); e != nil {
		return
	}

	if kv == nil {
		gLog.Warn().Msgf("consul sent empty KV while get route is called for key %s", key)
		return
	}

	value = string(kv.Value)
	m.router.set(key, string(kv.Value))
	gLog.Trace().Dur("took", meta.RequestTime).Msg("consul get from KV debug")
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

func (m *balancer) getServerByChunkName(chunk string) (sip string, server *server) {
	if strings.Contains(chunk, "_") {
		sip = strings.Split(chunk, "_")[1]
	} else if strings.Contains(chunk, "fff") {
		sip = strings.ReplaceAll(chunk, "fff", "")
	} else {
		gLog.Warn().Msgf("could not get server because of invalid chunk name '%s'; fallback to legacy balancing", chunk)
		return
	}

	gLog.Trace().Msgf("sip - %s", sip)

	sid, e := strconv.Atoi(sip)
	if e != nil {
		return
	}

	gLog.Trace().Msgf("sid - %d", sid)

	sip = m.getNextServer2(sid).String()
	gLog.Trace().Msgf("sip - %s", sip)
	return sip, m.getServer(sip)
}

func (m *balancer) getNextServer2(idx int) (_ *net.IP) {
	balancerLocker.RLock()
	defer balancerLocker.RUnlock()

	if m.midx == 0 {
		return
	}

	gLog.Trace().Msgf("m.midx - %d", int(m.midx))

	idx = idx % int(m.midx)
	gLog.Trace().Msgf("m.idx - %d", idx)
	// idx = idx + 1
	gLog.Trace().Msgf("m.idx - %d", idx)

	gLog.Trace().Msgf("------------m.idx - %d------------", m.idx)

	return &m.balancer[idx]
}
