package balancer

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rs/zerolog"
	"github.com/tysonmote/rendezvous"
	"github.com/urfave/cli/v2"
)

type Rendezvous struct {
	log *zerolog.Logger

	ccx *cli.Context

	cluster BalancerCluster

	maxResults uint8

	rendezvous *rendezvous.Hash
	upstream   *upstream
	uplock     sync.Mutex

	rlock sync.Mutex
}

func NewRendezvous(ctx context.Context, cluster BalancerCluster, maxsrvs uint8) *Rendezvous {
	upstream, log := make(upstream), ctx.Value(utils.ContextKeyLogger).(*zerolog.Logger)
	log.Hook(RvhLogHook{})

	return &Rendezvous{
		maxResults: maxsrvs,
		log:        log,
		cluster:    cluster,
		upstream:   &upstream,

		ccx: ctx.Value(utils.ContextKeyCliContext).(*cli.Context),
	}
}

func (m *Rendezvous) status() *Status {
	return NewStatus(m.cluster)
}

func (m *Rendezvous) getKeyFromChunkName(chunkname *string) (key string) {
	if strings.Contains(*chunkname, "_") {
		key = strings.Split(*chunkname, "_")[1]
	} else if strings.Contains(*chunkname, "fff") {
		key = strings.ReplaceAll(*chunkname, "fff", "")
	}

	return
}

func (m *Rendezvous) Balance(chunkname, prefix string) (e error) {
	m.rlock.Lock()
	defer m.rlock.Unlock()

	if m.upstream == nil {
		return m.status().SetError(ErrNotInitializedYet)
	}

	if key := m.getKeyFromChunkName(&chunkname); key == "" {
		return m.status().SetError(ErrUnparsableChunk).SetDescr("invalid chunkname '%s'", chunkname)
	} else {
		// chunkname = prefix + key
		chunkname = key
	}

	// var sips []string
	// if sips = m.rendezvous.GetN(int(m.maxResults), chunkname); len(sips) == 0 {
	// 	return m.status().SetError(ErrUpstreamUnavailable)
	// }

	var sip string
	if sip = m.rendezvous.Get(chunkname); sip == "" {
		return m.status().SetError(ErrUpstreamUnavailable)
	}

	// var found bool
	ok, server := false, &BalancerServer{}

	// for _, sip := range sip {
	if server, ok = m.upstream.getServer(sip); !ok || server == nil {
		panic("balance result could not be find in balancer's upstream")
	}

	// 	if !server.isDown {
	// 		found = true
	// 		break
	// 	}
	// }

	// if !found {
	// 	return m.status().SetError(ErrServerUnavailable)
	// }

	server.statRequest()
	return m.status().SetServers(server)
}

func (m *Rendezvous) UpdateUpstream(servers map[string]net.IP) {
	m.log.Info().Msg("upstream update triggered")

	m.uplock.Lock()
	defer m.uplock.Unlock()

	if m.rendezvous == nil {
		m.rendezvous = rendezvous.New()
		m.log.Info().Msg("rendezvous balancer has been initialized")
	}

	// find and append balancer's upstream
	for fqdn, ip := range servers {
		if server, ok := m.upstream.getServer(ip.String()); ok {
			server.disable(false)
		} else {
			m.upstream.putServer(ip.String(), newServer(fqdn, &ip))
			m.rendezvous.Add(ip.String())
		}

		m.log.Trace().Msgf("the server has already rendezvoused: %s (%s)", ip.String(), fqdn)
	}

	// find differs and disable dead servers
	upsnapshot := m.upstream.copy()
	for _, server := range upsnapshot {
		if _, ok := servers[server.Name]; ok {
			m.log.Trace().Msgf("the server is enabled and rendezvoused %s", server.Name)
			continue
		}

		server.disable()
		m.log.Trace().Msgf("the server is disabled due to its absence in consul %s", server.Name)
	}

	if zerolog.GlobalLevel() == zerolog.TraceLevel {
		ips, size := m.upstream.getIps()
		m.log.Trace().Interface("ips", ips).Msg("[II] UpdateUpstream() debug")
		m.log.Trace().Interface("size", size).Msgf("[II] UpdateUpstream() debug")
	}
}

func (m *Rendezvous) ResetUpstream() {
	m.uplock.Lock()
	defer m.uplock.Unlock()

	upstream := make(upstream)
	m.upstream, m.rendezvous = &upstream, nil
}

func (m *Rendezvous) GetStats() io.Reader {
	tb := table.NewWriter()
	defer tb.Render()

	isDownHumanize := func(i bool) string {
		switch i {
		case false:
			return "no"
		default:
			return "yes"
		}
	}

	buf := bytes.NewBuffer(nil)
	tb.SetOutputMirror(buf)
	tb.AppendHeader(table.Row{
		"Method", "Name", "Address", "Requests", "Last Request", "Disabled", "Update Time",
	})

	servers := m.upstream.getServers()
	for _, server := range servers {
		tb.AppendRow([]interface{}{
			"RVH", server.Name, server.Ip,
			server.handledRequests, server.lastRequestTime.String(),
			isDownHumanize(server.isDown), server.lastChanged.String(),
		})
	}

	tb.SortBy([]table.SortBy{
		{Number: 4, Mode: table.Dsc},
	})

	tb.Style().Options.SeparateRows = true

	return buf
}

func (m *Rendezvous) ResetStats() {
	m.upstream.resetServersStats()
}

func (m *Rendezvous) oops(e error) error {
	return NewError(m.cluster, e)
}

func (*Rendezvous) BalanceByChunk(prefix, chunkname string) (_ string, server *BalancerServer, e error) {
	return
}

func (*Rendezvous) BalanceRandom(force bool) (_ string, server *BalancerServer, e error) { return }

func (m *Rendezvous) UpdateServers(servers map[string]net.IP) {
	m.UpdateUpstream(servers)
}

func (m *Rendezvous) GetClusterName() string {
	switch m.cluster {
	case BalancerClusterNodes:
		return m.ccx.String("consul-service-nodes")
	case BalancerClusterCloud:
		return m.ccx.String("consul-service-cloud")
	default:
		return ""
	}
}
