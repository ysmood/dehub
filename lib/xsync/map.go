package xsync

import "sync"

type Map[K comparable, V any] struct {
	m sync.Map
}

func (m *Map[K, V]) Delete(key K) { m.m.Delete(key) }

func (m *Map[K, V]) Load(key K) (V, bool) {
	v, ok := m.m.Load(key)

	return v.(V), ok
}

func (m *Map[K, V]) LoadOrStore(key K, value V) (V, bool) {
	a, loaded := m.m.LoadOrStore(key, value)

	return a.(V), loaded
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value any) bool { return f(key.(K), value.(V)) })
}

func (m *Map[K, V]) Store(key K, value V) { m.m.Store(key, value) }
