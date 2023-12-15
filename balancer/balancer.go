package balancer

import (
	"errors"
	"io"
	"net"
)

type Balancer interface {
	BalanceByChunkname(prefix, chunkname string, try uint8) (_ string, server *BalancerServer, e error)
	BalanceRandom(force bool) (_ string, server *BalancerServer, e error)
	UpdateServers(servers map[string]net.IP)
	GetStats() io.Reader
	ResetStats()
	ResetUpstream()
	GetClusterName() string
}

var (
	ErrUpstreamUnavailable = errors.New("upstream is empty or undefined - balancing is impossible")
	ErrServerIsDown        = errors.New("rolled server is marked as down")
)

type BalancerCluster uint8

const (
	BalancerClusterNodes BalancerCluster = iota
	BalancerClusterCloud
)

const MaxTries = uint8(3)
