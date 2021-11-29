// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/utils/timer/mockable"
)

type ThreadPool struct {
	sync.Mutex
	size          int
	activeWorkers int
	signalCh      chan struct{}
	closeCh       chan struct{}
	clock         mockable.Clock
}

func NewThreadPool(size int) *ThreadPool {
	tPool := new(ThreadPool)
	// use a better data structure ?
	tPool.size = size
	tPool.activeWorkers = 0
	tPool.signalCh = make(chan struct{}, size)
	tPool.closeCh = make(chan struct{})
	return tPool
}

func (t *ThreadPool) freeWorkerExists() bool {
	return t.size > t.activeWorkers
}

type result struct {
	start time.Time
	end   time.Time
	err   error
}

func (t *ThreadPool) handleMessage(appFunc func() error) (s time.Time, end time.Time, err error) {
	// increment active workers
	t.incrementWorkers()
	// release active worker
	defer t.releaseWorker()
	ch := make(chan result)
	start := t.clock.Time()
	go func() {
		res := new(result)
		if err := appFunc(); err != nil {
			res.start = time.Time{}
			res.end = time.Time{}
			res.err = err
		} else {
			res.start = start
			res.end = t.clock.Time()
			res.err = nil
		}
		ch <- *res
	}()

	res := <-ch

	return res.start, res.end, res.err
}

func (t *ThreadPool) SendMessage(appFunc func() error) (start time.Time, end time.Time, err error) {
	if t.freeWorkerExists() {
		return t.handleMessage(appFunc)
	}
	return t.waitForWorker(appFunc)
}

func (t *ThreadPool) Len() int {
	return t.size
}

func (t *ThreadPool) incrementWorkers() {
	t.activeWorkers++
	if t.activeWorkers > t.size {
		t.activeWorkers = t.size
	}
}

func (t *ThreadPool) decrementWorkers() {
	t.activeWorkers--
	if t.activeWorkers < 0 {
		t.activeWorkers = 0
	}
}

func (t *ThreadPool) releaseWorker() {
	t.Lock()
	defer t.Unlock()
	t.decrementWorkers()
	// dont signal if the buffer is full
	if len(t.signalCh) != cap(t.signalCh) {
		t.signalCh <- struct{}{}
	}
}

func (t *ThreadPool) CloseCh() {
	t.closeCh <- struct{}{}
}

func (t *ThreadPool) waitForWorker(appFunc func() error) (s time.Time, end time.Time, err error) {
	for {
		select {
		case <-t.closeCh:
			close(t.signalCh)
		case <-t.signalCh:
			return t.handleMessage(appFunc)
		}
	}
}
