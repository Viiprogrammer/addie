package app

import "sync"

type CachedTitlesBucket struct {
	locker sync.RWMutex
	bucket map[uint32]*TitleSerie
}

func (m *CachedTitlesBucket) PullSerie(tid, sid uint16) (serie *TitleSerie) {
	m.locker.RLock()

	serie = m.bucket[uint32(tid)<<16|uint32(sid)]
	m.locker.RUnlock()

	return
}

func (m *CachedTitlesBucket) PushSerie(serie *TitleSerie) {
	m.locker.Lock()

	m.bucket[uint32(serie.Title)<<16|uint32(serie.Serie)] = serie
	m.locker.Unlock()
}
