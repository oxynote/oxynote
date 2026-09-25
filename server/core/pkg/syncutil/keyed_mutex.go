// Package syncutil provides synchronization helpers.
package syncutil

import (
	"context"
	"sync"
)

// KeyedMutex is a set of mutexes, one per key, each held by one caller at
// a time. A key's mutex exists only while someone uses it.
type KeyedMutex[K comparable] struct {
	mu    sync.Mutex
	locks map[K]*keyedLock
}

// NewKeyedMutex returns an empty keyed mutex.
func NewKeyedMutex[K comparable]() *KeyedMutex[K] {
	return &KeyedMutex[K]{
		locks: make(map[K]*keyedLock),
	}
}

// Lock waits for the key's mutex until the context ends and returns the
// function that unlocks it.
func (km *KeyedMutex[K]) Lock(ctx context.Context, key K) (func(), error) {
	l := km.ref(key)

	select {
	case l.ch <- struct{}{}:
		return km.unlocker(key, l), nil
	case <-ctx.Done():
		km.unref(key, l)

		return nil, ctx.Err()
	}
}

// TryLock takes the key's mutex when no one holds it and returns the
// function that unlocks it.
func (km *KeyedMutex[K]) TryLock(key K) (func(), bool) {
	l := km.ref(key)

	select {
	case l.ch <- struct{}{}:
		return km.unlocker(key, l), true
	default:
		km.unref(key, l)

		return nil, false
	}
}

// ref returns the key's lock, counting the caller as its user.
func (km *KeyedMutex[K]) ref(key K) *keyedLock {
	km.mu.Lock()
	defer km.mu.Unlock()

	l, ok := km.locks[key]
	if !ok {
		l = &keyedLock{ch: make(chan struct{}, 1)}
		km.locks[key] = l
	}

	l.users++

	return l
}

// unref drops the caller as the lock's user, and the lock once no one
// uses it.
func (km *KeyedMutex[K]) unref(key K, l *keyedLock) {
	km.mu.Lock()
	defer km.mu.Unlock()

	l.users--

	if l.users == 0 {
		delete(km.locks, key)
	}
}

// unlocker returns the function that unlocks a held lock.
func (km *KeyedMutex[K]) unlocker(key K, l *keyedLock) func() {
	return func() {
		<-l.ch
		km.unref(key, l)
	}
}

// keyedLock is one key's lock: holding it is a value in ch.
type keyedLock struct {
	ch    chan struct{}
	users int
}
