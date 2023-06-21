package app

func (m balancerRouter) get(k string) (v string) {
	routerLocker.RLock()
	v = m[k]
	routerLocker.RUnlock()

	return
}

func (m balancerRouter) set(k, v string) {
	routerLocker.Lock()
	m[k] = v
	routerLocker.Unlock()
}

func (m balancerUpstream) get(k string) (v *server) {
	upstreamLocker.RLock()
	v = m[k]
	upstreamLocker.RUnlock()

	return
}

func (m balancerUpstream) set(k string, v *server) {
	upstreamLocker.Lock()
	m[k] = v
	upstreamLocker.Unlock()
}
