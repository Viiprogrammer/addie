package balancer

import (
	"errors"
	"io"
	"net"
)

type Balancer interface {
	Balance(chunkname, prefix string) (e error)

	UpdateServers(servers map[string]net.IP)
	ResetUpstream()

	GetStats() io.Reader
	ResetStats()

	// deprecated
	BalanceByChunk(prefix, chunkname string) (_ string, server *BalancerServer, e error)
	BalanceRandom(force bool) (_ string, server *BalancerServer, e error)
	GetClusterName() string
}

type BalancerV2 interface {
	Balance(key, prefix string) (_ string, server *BalancerServer, e error)
	BalanceRandom() (_ string, server *BalancerServer, e error)

	GetClusterName() string

	GetStats() io.Reader
	ResetStats()

	UpdateUpstream(servers map[string]net.IP)
	ResetUpstream()
}

var (
	ErrUnparsableChunk     = errors.New("could not get server because of invalid chunk name")
	ErrServerUnavailable   = errors.New("rolled server is down now")
	ErrUpstreamUnavailable = errors.New("upstream is empty or undefined; balancing is not possible")
	ErrNotInitializedYet   = errors.New("balancer not initialized yet, try again later")
	ErrRVHEmptyFqdn        = errors.New("current rvh balancer returned the empty fqdn")
)

type BalancerCluster uint8

const (
	BalancerClusterNodes BalancerCluster = iota
	BalancerClusterCloud
)
