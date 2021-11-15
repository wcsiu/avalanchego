// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import "sync"

type basicLock struct {
	free bool
	Lock sync.Mutex
}

type LockPool struct {
	pool     []*basicLock
	signalCh chan struct{}
	closeCh  chan struct{}
}

func NewLockPool(size int) *LockPool {
	lPool := new(LockPool)
	// use a better data structure ?
	var pool []*basicLock = make([]*basicLock, size)
	for i := 0; i < len(pool); i++ {
		pool[i] = &basicLock{free: true}
	}
	lPool.pool = pool
	lPool.signalCh = make(chan struct{}, size)
	return lPool
}

func (l *LockPool) GetFreeLock() (*basicLock, int, bool) {
	for i, lock := range l.pool {
		if lock.free {
			lock.free = false
			return lock, i, true
		}
	}
	return nil, 0, false
}

func (l *LockPool) Len() int {
	return len(l.pool)
}

func (l *LockPool) Free(index int) {
	if index < 0 || index >= l.Len() {
		return
	}
	lock := l.pool[index]
	lock.free = true
	lock.Lock.Unlock()
	l.signalCh <- struct{}{}
}

func (l *LockPool) CloseCh() {
	l.closeCh <- struct{}{}
}

func (l *LockPool) WaitForSignal() (*basicLock, int, bool) {
	for {
		select {
		case <-l.closeCh:
			close(l.signalCh)
		case <-l.signalCh:
			return l.GetFreeLock()
		}
	}
}
