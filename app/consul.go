package app

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/rs/zerolog"
)

type consulClient struct {
	*capi.Client
	balancer *balancer
}

func newConsulClient(b *balancer) (client *consulClient, e error) {
	cfg := capi.DefaultConfig()

	cfg.Address = gCli.String("consul-address")
	if cfg.Address == "" {
		gLog.Warn().Msg("given consul address could not be empty")
		return nil, errors.New("given consul address could not be empty")
	}

	if gCli.String("consul-service-name") == "" {
		gLog.Warn().Msg("given consul service name could not be empty")
		return nil, errors.New("given consul service name could not be empty")
	}

	client = new(consulClient)
	client.Client, e = capi.NewClient(cfg)
	client.balancer = b
	return
}

func (m *consulClient) bootstrap() {
	gLog.Debug().Msg("consul bootrap started")

	eventCtx, eventDone := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var errs = make(chan error, 1)

	wg.Add(1)
	go func() {
		gLog.Debug().Msg("consul event listener started")

		if e := m.listenEvents(eventCtx); e != nil {
			errs <- e
		}

		wg.Done()
	}()

loop:
	for {
		select {
		case err := <-errs:
			gLog.Error().Err(err).Msg("")
			break loop
		case <-gCtx.Done():
			gLog.Debug().Msg("internal abort() has been caught")
			break loop
		}
	}

	eventDone()
	gLog.Debug().Msg("waiting for goroutines")
	wg.Wait()
}

// !! context
func (m *consulClient) listenEvents(ctx context.Context) (e error) {
	var idx uint64
	var servers map[string]net.IP
	var fails uint8

	for {
		if gCtx.Err() != nil {
			break
		}

		if fails > uint8(3) && !gCli.Bool("consul-ignore-errors") {
			gLog.Error().Msg("too many unsuccessfully tempts of serverlist receiving")
			break
		}

		if servers, idx, e = m.getHealthServiceServers(ctx, idx); e != nil {
			gLog.Warn().Uint8("fails", fails).Err(e).
				Msg("there some problems with serverlist receiving from consul")
			fails = fails + 1

			time.Sleep(1 * time.Second)
			continue
		}

		gLog.Debug().Msg("consul listenEvents iteration triggered")
		fails = 0

		if len(servers) == 0 {
			gLog.Warn().Msg("received empty serverlist from consul")
		}

		if gLog.GetLevel() == zerolog.TraceLevel {
			gLog.Trace().Msg("received serverlist debug:")

			for _, ip := range servers {
				gLog.Trace().Msgf("received serverlist entry - %s", ip.String())
			}
		}

		m.balancer.updateUpstream(servers)
	}

	return
}

func (m *consulClient) getHealthServiceServers(ctx context.Context, idx uint64) (_ map[string]net.IP, _ uint64, e error) {
	opts := &capi.QueryOptions{
		WaitIndex:         idx,
		AllowStale:        true,
		UseCache:          true,
		RequireConsistent: false,
	}

	entries, meta, e := m.Health().Service(gCli.String("consul-service-name"), "", true, opts.WithContext(ctx))
	if e != nil {
		return nil, idx, e
	}

	var ip net.IP
	var servers = make(map[string]net.IP)

	for _, entry := range entries {
		gLog.Debug().Msgf("new health service entry %s:%d", entry.Node.Address, entry.Service.Port)

		ip = net.ParseIP(entry.Node.Address)
		if ip == nil {
			gLog.Warn().Msgf("there is invalid server address from consul - %s", entry.Node.Address)
			continue
		}

		servers[entry.Node.Node] = ip
	}

	return servers, meta.LastIndex, e
}

func (m *consulClient) watchForKVChanges(ctx context.Context, idx uint64) (e error) {
	return
}
