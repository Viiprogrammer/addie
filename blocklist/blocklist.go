package blocklist

import (
	"context"
	"sync"

	"github.com/MindHunter86/anilibria-hlp-service/utils"
	"github.com/rs/zerolog"
)

type Blocklist []string

var (
	eLocker sync.RWMutex
	Enabled bool

	blLocker sync.RWMutex
	log      *zerolog.Logger
)

func NewBlocklist(ctx context.Context) *Blocklist {
	log = ctx.Value(utils.ContextKeyLogger).(*zerolog.Logger)
	return &Blocklist{}
}

func (*Blocklist) IsEnabled() (enabled bool) {
	eLocker.RLock()
	defer eLocker.RUnlock()

	enabled = Enabled
	return
}

func (*Blocklist) Disable(disabled ...bool) {
	eLocker.Lock()
	defer eLocker.Unlock()

	disabled = append(disabled, true)
	Enabled = disabled[0] == false
}

func (m *Blocklist) Reset() {
	*m = Blocklist{}
}

func (m *Blocklist) Push(ips ...string) {
	if len(ips) == 0 {
		log.Warn().Interface("ips", ips).Msg("internal error, empty slice in Blocklist")
		return
	}

	log.Trace().Strs("ips", ips).Msg("Blocklist push has been called")

	blLocker.Lock()
	defer blLocker.Unlock()

	m.Reset()

	for _, ip := range ips {
		if ip == "" {
			continue
		}

		log.Trace().Str("ip", ip).Msg("new ip commited to Blocklist")
		*m = append(*m, ip)
	}

	log.Trace().Strs("ips", ips).Msg("Blocklist push has been called")
}

func (m *Blocklist) IsExists(ip string) (ok bool) {
	if ip == "" {
		log.Warn().Str("ip", ip).Msg("internal error, empty string in Blocklist")
		return
	}

	log.Trace().Str("ip", ip).Msg("Blocklist isExists has been called")

	blLocker.RLock()
	for _, v := range *m {
		if v == ip {
			ok = true
		}
	}
	blLocker.RUnlock()

	return
}

func (m *Blocklist) Size() (size int) {
	log.Trace().Msg("Blocklist size has been called")

	blLocker.RLock()
	size = len(*m)
	blLocker.RUnlock()

	return
}
