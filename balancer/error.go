package balancer

import (
	"context"
	"fmt"
)

const (
	errUndefined = "undefined error occurred"

	errUnparsableChunk = "could not get key from given chunkname %s"
	errLockMiss        = "could not get balancer read-lock"

	errUpstreamIsDown  = "balancer is marked as down"
	errUpstreamIsEmpty = "balancer's upstream has no servers"
	errServerIsDown2   = "server %s is marked as down"
)

type BalancerEFlag uint8

// IsRetriable - can be retried in current upstream
// IsNextServerRouted = must be routed to the next cluster's server
// IsNextClusterRouted - must be routed to the next cluster
const (
	IsRetriable BalancerEFlag = 1 << iota
	IsNextServerRouted
	IsNextClusterRouted
	IsBackupable
	IsReroutable
)

type BalancerError struct {
	Balancer Balancer

	Err  string
	Errs []string

	flags BalancerEFlag
	errs  uint8

	try func(uint8, ...any) context.Context
}

func NewError(m Balancer, e string) (b *BalancerError) {
	b = &BalancerError{Balancer: m}
	return b.NewError(e)
}

func NewErrorF(m Balancer, format string, args ...any) (b *BalancerError) {
	b = &BalancerError{Balancer: m}
	return b.NewErrorF(format, args...)
}

func (m *BalancerError) NewError(e string) *BalancerError {
	if m.Err != "" {
		m.Errs = append(m.Errs, m.Err)
	}

	m.errs += 1
	m.Err = e
	return m
}

func (m *BalancerError) NewErrorF(format string, args ...any) *BalancerError {
	if m.Err != "" {
		m.Errs = append(m.Errs, m.Err)
	}

	m.errs += 1
	m.Err = fmt.Sprintf(format+"\n", args...)
	return m
}

func (m *BalancerError) HasError() bool {
	return m.errs != 0
}

func (m *BalancerError) Error() string {
	return fmt.Sprintf("cluster %s failed with %s", m.Balancer.GetClusterName(), m.Err)
}

func (m *BalancerError) Errors() []string {
	return m.Errs
}

func (m *BalancerError) MultipleErrors() bool {
	return len(m.Errs) > 1
}

func (m *BalancerError) ResetFlags() *BalancerError {
	m.flags = 0
	return m
}

func (m *BalancerError) SetFlag(flag BalancerEFlag) *BalancerError {
	m.flags = m.flags ^ flag
	return m
}

func (m *BalancerError) Has(flag BalancerEFlag) bool {
	return m.flags&flag != 0
}

func (m *BalancerError) RemoveLastError() *BalancerError {
	if m.errs != 0 {
		m.errs -= 1
	}

	return m
}

func (m *BalancerError) HasChance() bool {
	return m.errs < MaxTries
}

func (m *BalancerError) TryFunc(try func(uint8, ...any) context.Context) *BalancerError {
	m.try = try
	return m
}

func (m *BalancerError) Try() context.Context {
	return m.try(m.errs)
}
