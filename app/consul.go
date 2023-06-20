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
	balancer *iplist

	commitedServices []*net.IP
	services         map[string]*capi.AgentService
}

func newConsulClient(b *iplist) (client *consulClient, e error) {
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

	client.Client, e = capi.NewClient(cfg)
	client.balancer = b
	return
}

func (m *consulClient) bootstrap() (e error) {
	eventCtx, eventDone := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var errs = make(chan error, 1)

	wg.Add(1)
	go func() {
		if e = m.listenEvents(eventCtx); e != nil {
			errs <- e
		}

		wg.Done()
	}()

loop:
	for {
		select {
		case err := <-errs:
			gLog.Error().Err(err).Msg("")
			break
		case <-gCtx.Done():
			gLog.Info().Msg("internal abort() has been caught")
			eventDone()
			break loop
		}
	}

	if len(errs) != 0 {
		for err := range errs {
			gLog.Error().Err(err).Msg("cleaning buff")
		}
	}

	gLog.Info().Msg("waiting for goroutines")
	wg.Wait()
	return
}

// !! context
func (m *consulClient) listenEvents(ctx context.Context) (e error) {
	var idx uint64
	var servers []net.IP
	var fails uint8

	for {
		if fails > uint8(3) {
			gLog.Error().Msg("too many unsuccessfully tempts of serverlist receiving")
			break
		}

		if servers, idx, e = m.getHealthServiceServers(ctx, idx); e != nil {
			gLog.Warn().Err(e).Msg("there some problems with serverlist receiving from consul")
			fails = fails << 1

			time.Sleep(1 * time.Second)
			continue
		}

		if len(servers) == 0 {
			gLog.Error().Msg("received serverlist from consul is empty, retrying...")
			fails = fails << 1

			time.Sleep(1 * time.Second)
			continue
		}

		gLog.Debug().Msg("consul listenEvents iteration triggered")
		fails = 0

		if gLog.GetLevel() == zerolog.TraceLevel {
			gLog.Trace().Msg("received serverlist debug:")

			for _, ip := range servers {
				gLog.Trace().Msgf("received serverlist entry - %s", ip.String())
			}
		}

		m.updateServerList(servers...)
	}

	return
}

func (m *consulClient) updateServerList(ips ...net.IP) {
	m.balancer.syncIps(ips...)
}

func (m *consulClient) getHealthServiceServers(ctx context.Context, idx ...uint64) (srvs []net.IP, _ uint64, e error) {
	idx = append(idx, 0) //default value

	opts := &capi.QueryOptions{
		WaitIndex: idx[0],
	}
	opts.WithContext(ctx)

	entries, meta, e := m.Health().Service(gCli.String("consul-service-name"), "", true, opts)
	if e != nil {
		return srvs, idx[0], e
	}

	var ip net.IP
	for _, entry := range entries {
		gLog.Debug().Msgf("new health service entry %s:%d", entry.Node.Address, entry.Service.Port)

		ip = net.ParseIP(entry.Node.Address)
		if ip == nil {
			gLog.Warn().Msgf("there is invalid server address from consul - %s", entry.Node.Address)
			continue
		}

		srvs = append(srvs, ip)
	}

	return srvs, meta.LastIndex, e
}

// func (m *consulClient) bootstrap() (e error) {
// 	cfg := capi.DefaultConfig()
// 	cfg.Address = "http://116.202.101.219:8500"

// 	if m.client, e = capi.NewClient(cfg); e != nil {
// 		return e
// 	}

// 	if m.services, e = m.client.Agent().Services(); e != nil {
// 		return
// 	}

// if len(m.services) == 0 {
// 	return errors.New("there is no services found in consul cluster")
// }

// service := m.services["cache-cloud-ingress"]
// gLog.Debug().Msgf("service found %s:%d", service.Address, service.Port)

// - catalog
// catalog, _, e := m.client.Catalog().Service("cache-cloud-ingress", "", nil)
// if e != nil {
// 	return e
// }

// gLog.Debug().Msgf("catalog count %d", len(catalog))

// for _, service := range catalog {
// 	gLog.Debug().Msgf("tagged addresses for %s", service.ID)
// 	for k, addr := range service.TaggedAddresses {
// 		gLog.Debug().Msgf("tagged %s - %s", k, addr)
// 	}

// 	gLog.Debug().Msg("node meta")
// 	for k, v := range service.NodeMeta {
// 		gLog.Debug().Msgf("node meta - %s %s", k, v)
// 	}

// 	gLog.Debug().Msgf("service checks %d:", len(service.Checks))
// 	for k, check := range service.Checks {
// 		gLog.Debug().Msgf("health check %d %s %s", k, check.Name, check.Status)
// 	}

// 	gLog.Debug().Msgf("status - %s", service.Checks.AggregatedStatus())

// 	gLog.Debug().Msg("========[END]========")
// }

// gLog.Debug().Msg("========[END]========")
// gLog.Debug().Msg("========[END]========")
// gLog.Debug().Msg("========[END]========")
// gLog.Debug().Msg("========[END]========")

// 	entries, _, e := m.client.Health().Service("cache-cloud-ingress", "", true, nil)
// 	if e != nil {
// 		return e
// 	}

// 	for _, entry := range entries {
// 		gLog.Debug().Msgf("new health entry %s:%d", entry.Node.Address, entry.Service.Port)

// 		for _, check := range entry.Checks {
// 			gLog.Debug().Msgf("entry health %s - status %s", check.Name, check.Status)
// 		}
// 	}

// 	return
// }
