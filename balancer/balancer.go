package balancer

import (
	"errors"
	"io"
	"net"
)

type Balancer interface {
	BalanceByChunk(prefix, chunkname string) (_ string, server *BalancerServer, e error)
	BalanceRandom(force bool) (_ string, server *BalancerServer, e error)
	UpdateServers(servers map[string]net.IP)
	GetStats() io.Reader
	ResetStats()
	ResetUpstream()
	GetClusterName() string

	// V3
	BalanceByChunkname(prefix, chunkname string, try uint8) (_ string, server *BalancerServer, e error)
}

var (
	ErrUnparsableChunk  = errors.New("could not get server because of invalid chunk name")
	ErrUpstreamLockMiss = errors.New("could not get balancer read-lock")

	//
	ErrUpstreamUnavailable = errors.New("upstream is empty or undefined - balancing is impossible")
	ErrUpstreamIsDown      = errors.New("upstream is marked as down - balancing is impossible")
	ErrUpstreamInternal    = errors.New("internal upstream error")
	ErrUpstreamEmpty       = errors.New("upstream has no server")

	ErrServerIsDown = errors.New("rolled server is marked as down")
)

const (
	errUnparsableChunkname = "could not get key from given chunkname"

	errBalancerLockMiss  = "could not get balancer read-lock"
	errBalancerIsDown    = "balancer is marked as down"
	errBalancerEmpty     = "balancer's upstream has no servers"
	errBalancerUndefined = "undefined error occured"

	// ---
)

type BalancerCluster uint8

const (
	BalancerClusterNodes BalancerCluster = iota
	BalancerClusterCloud
)

const MaxTries = uint8(3)
