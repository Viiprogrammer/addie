package balancer

import (
	"context"
	"errors"
	"io"
	"net"
)

func Init(ctx context.Context) {
	gTimer = new(timeManager)
	gTimer.bootstrap(ctx)
}

type Balancer interface {
	BalanceByChunk(prefix, chunkname string) (_ string, server *BalancerServer, e error)
	UpdateServers(servers map[string]net.IP)
	GetStats() io.Reader
	ResetStats()
	ResetUpstream()
}

var ErrUnparsableChunk = errors.New("could not get server because of invalid chunk name")
var ErrServerUnavailable = errors.New("rolled server is down now")
