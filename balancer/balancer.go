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
	GetNextServer(prefix, chunkname string) (_ string, server *BalancerServer, e error)
	UpdateServers(servers map[string]net.IP)
	GetStats() io.Reader
	ResetStats()
}

var ErrUnparsableChunk = errors.New("could not get server because of invalid chunk name")
