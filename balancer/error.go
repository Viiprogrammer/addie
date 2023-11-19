package balancer

import (
	"fmt"

	"github.com/rs/zerolog"
)

type Status struct {
	cluster BalancerCluster

	error error
	descr string

	Servers []*BalancerServer
}

func NewStatus(cluster BalancerCluster) *Status {
	return &Status{
		cluster: cluster,
	}
}

func (m *Status) Err() error {
	return m.error
}

func (m *Status) Descr() string {
	return m.descr
}

func (m *Status) Error() string {
	return m.error.Error()
}

func (m *Status) SetError(err error) *Status {
	m.error = err
	return m
}

func (m *Status) SetDescr(descr string, args ...interface{}) *Status {
	m.descr = fmt.Sprintf(descr, args...)
	return m
}

func (m *Status) SetServers(server ...*BalancerServer) *Status {
	m.Servers = append(m.Servers, server...)
	return m
}

func (m *Status) Cluster() BalancerCluster {
	return m.cluster
}

///
///
///
///

type BalancerError struct {
	Cluster BalancerCluster
	Err     error

	ErrCount uint8
}

func NewError(cluster BalancerCluster, err error) *BalancerError {
	return &BalancerError{
		Cluster: cluster,
		Err:     err,
	}
}

func (m *BalancerError) Error() string {
	return m.Err.Error()
}

func (m *BalancerError) Failovered() bool {
	return m.ErrCount >= 1
}

type RvhLogHook struct{}

func (RvhLogHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	e.Str("module", "rvh-balancer")
}
