package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	capi "github.com/hashicorp/consul/api"
	"github.com/rs/zerolog"
)

type consulClient struct {
	*capi.Client
	ctx context.Context

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

	var eventDone context.CancelFunc
	m.ctx, eventDone = context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var errs = make(chan error, 1)

	wg.Add(1)
	go func() {
		gLog.Debug().Msg("consul event listener started")

		if e := m.listenEvents(); e != nil {
			errs <- e
		}

		wg.Done()
	}()

	cfgchan := gCtx.Value(utils.ContextKeyCfgChan).(chan *runtimeConfig)

	wg.Add(1)
	go func() {
		gLog.Debug().Msg("consul config listener started (lottery)")
		m.listenRuntimeConfigKey(utils.CfgLotteryChance, cfgchan)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		gLog.Debug().Msg("consul config listener started (quality)")
		m.listenRuntimeConfigKey(utils.CfxQualityLevel, cfgchan)
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
func (m *consulClient) listenEvents() (e error) {
	var idx uint64
	var servers map[string]net.IP
	var fails uint8

	for {
		if gCtx.Err() != nil {
			return
		}

		if fails > uint8(3) && !gCli.Bool("consul-ignore-errors") {
			gLog.Error().Msg("too many unsuccessfully tempts of serverlist receiving")
			return
		}

		if servers, idx, e = m.getHealthServiceServers(idx); errors.Is(e, context.Canceled) {
			return
		} else if e != nil {
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
}

func (m *consulClient) getHealthServiceServers(idx uint64) (_ map[string]net.IP, _ uint64, e error) {
	opts := &capi.QueryOptions{
		WaitIndex:         idx,
		AllowStale:        true,
		UseCache:          false,
		RequireConsistent: false,
	}

	entries, meta, e := m.Health().Service(gCli.String("consul-service-name"), "", true, opts.WithContext(m.ctx))
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

func (m *consulClient) listenRuntimeConfigKey(key string, payload chan *runtimeConfig) {
	var opts = &capi.QueryOptions{
		AllowStale:        true,
		UseCache:          false,
		RequireConsistent: false,
	}

	var idx uint64
	var ckey = fmt.Sprintf("%s/settings/%s", gCli.String("consul-kv-prefix"), key)

loop:
	for {
		select {
		case <-m.ctx.Done():
			break loop
		default:
			opts.WaitIndex = idx
			pair, meta, e := m.KV().Get(ckey, opts.WithContext(m.ctx))

			if errors.Is(e, context.Canceled) {
				break loop
			} else if e != nil {
				gLog.Error().Err(e).Msgf("could not get consul value for %s key", key)
				time.Sleep(5 * time.Second)
				continue
			}

			if pair == nil {
				gLog.Warn().Msg("consul sent empty values while runtime config getting")
				time.Sleep(5 * time.Second)
				continue
			}

			rconfig := &runtimeConfig{}

			switch key {
			case utils.CfgLotteryChance:
				rconfig.lotteryChance = pair.Value
			case utils.CfxQualityLevel:
				rconfig.qualityLevel = pair.Value
			}

			payload <- rconfig
			idx = meta.LastIndex
		}
	}
}
