package app

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

type (
	server struct {
		name string

		sync.RWMutex
		lastRequestTime time.Time
		proxiedRequests uint64
	}

	ipam struct {
		sync.RWMutex
		ipam map[string]*server
	}

	iplist struct {
		ipam *ipam

		sync.RWMutex
		idx, midx uint64

		list   []net.IP
		router map[string]*net.IP
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

func newIpam() *ipam {
	return &ipam{
		ipam: make(map[string]*server),
	}
}

func (m *ipam) getServer(ip *net.IP) (s *server) {
	gLog.Trace().Msgf("getServer ip - %s", ip.String())
	m.RLock()
	s = m.ipam[ip.String()]
	m.RUnlock()
	return
}

func (m *ipam) putServer(ip *net.IP, s *server) {
	m.Lock()
	m.ipam[ip.String()] = s
	m.Unlock()
}

func (m *ipam) getIpamCopy() (serverlist map[string]*server) {
	m.RLock()
	serverlist = m.ipam
	m.RUnlock()

	return serverlist
}

func newIplist(i *ipam) *iplist {
	return &iplist{
		ipam: i,
	}
}

func (m *iplist) syncIps(srvs map[string]net.IP) {
	gLog.Debug().Msg("syncIps has been triggered")
	gLog.Trace().Interface("ips", srvs).Msg("")

	var newlist []net.IP
	for name, ip := range srvs {

		if s := m.ipam.getServer(&ip); s == nil {
			s = newServer(name)
			m.ipam.putServer(&ip, s)
		}

		gLog.Info().Msgf("appending new server to iplist: %s", ip.String())
		newlist = append(newlist, ip)
		gLog.Trace().Interface("newlist", newlist).Msg("")
	}

	m.commitNewList(&newlist)
}

func (m *iplist) commitNewList(list *[]net.IP) {
	gLog.Info().Msg("new list commiting...")

	m.Lock()
	m.list = *list
	m.midx = uint64(len(*list))
	m.router = make(map[string]*net.IP)
	m.Unlock()

	gLog.Debug().Msgf("new list has been commited, srvs: %d", m.midx)
}

func (m *iplist) addRouterEntry(k string, ip *net.IP) {
	m.Lock()
	m.router[k] = ip
	m.Unlock()
}

func (m *iplist) getRouterEntry(k string) (ip *net.IP) {
	m.RLock()
	ip = m.router[k]
	m.RUnlock()

	return
}

func (m *iplist) getIpByKey(k string) (ip *net.IP) {
	if ip = m.getRouterEntry(k); ip != nil {
		return
	}

	m.Lock()
	if m.midx == 0 {
		m.Unlock()
		return nil
	}

	if m.idx = m.idx + 1; m.idx >= m.midx {
		gLog.Trace().Msg("idx reseted")
		m.idx = 0
	}

	gLog.Trace().Msgf("idx - %d", m.idx)

	ip = &m.list[m.idx]
	gLog.Trace().Interface("asd", m.list).Msg("")
	m.Unlock()

	m.addRouterEntry(k, ip)
	return
}

func (m *iplist) getIp(k string) (ip *net.IP, s *server) {
	if ip = m.getIpByKey(k); ip == nil {
		return
	}

	s = m.ipam.getServer(ip)
	s.updateStat()

	return
}

func (m *iplist) getServersStats() io.ReadWriter {
	tb := table.NewWriter()
	defer tb.Render()

	buf := bytes.NewBuffer(nil)
	tb.SetOutputMirror(buf)
	tb.AppendHeader(table.Row{
		"Address", "Name", "Requests", "LastRequest",
	})

	var serverlist = m.ipam.getIpamCopy()
	for ip, server := range serverlist {
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

func (m *iplist) getRouterStats() io.ReadWriter {
	tb := table.NewWriter()
	defer tb.Render()

	buf := bytes.NewBuffer(nil)
	tb.SetOutputMirror(buf)
	tb.AppendHeader(table.Row{
		"URI", "Server",
	})

	m.RLock()
	router := m.router
	m.RUnlock()

	for uri, server := range router {
		tb.AppendRow([]interface{}{
			uri, server.String(),
		})
	}

	// tb.SortBy([]table.SortBy{
	// 	{Number: 2, Mode: table.Asc},
	// })

	tb.Style().Options.SeparateRows = true

	return buf
}
