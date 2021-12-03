// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"sync"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/networking/tracker"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer/mockable"
)

type ThreadPoolRequest struct {
	AppRequest func() error
	NodeID     ids.ShortID
}

type ThreadPool struct {
	sync.Mutex
	size          int
	activeWorkers int
	DataCh        chan ThreadPoolRequest
	signalCh      chan struct{}
	closeCh       chan struct{}
	clock         mockable.Clock
	cpuTracker    tracker.TimeTracker
	log           logging.Logger
}

func NewThreadPool(size int, cpuTracker tracker.TimeTracker) *ThreadPool {
	tPool := new(ThreadPool)
	tPool.size = size
	tPool.cpuTracker = cpuTracker
	tPool.activeWorkers = 0
	tPool.signalCh = make(chan struct{}, size)
	tPool.DataCh = make(chan ThreadPoolRequest)
	tPool.closeCh = make(chan struct{})
	go tPool.receiveMessages()
	return tPool
}

func (t *ThreadPool) freeWorkerExists() bool {
	return t.size > t.activeWorkers
}

func (t *ThreadPool) handleMessage(request ThreadPoolRequest) {
	// increment active workers
	t.incrementWorkers()
	// increase CPU cores
	t.cpuTracker.IncreaseCPUCount(request.NodeID)
	// decrease CPU cores
	defer t.cpuTracker.DecreaseCPUCount(request.NodeID)
	// release active worker
	defer t.releaseWorker()
	start := t.clock.Time()
	if err := request.AppRequest(); err != nil {
		t.log.Info("AppRequest from node ID %s failed with err: %s", request.NodeID, err)
		return
	}
	end := t.clock.Time()
	// Run callback to track time
	t.cpuTracker.UtilizeTime(request.NodeID, start, end)
}

func (t *ThreadPool) sendMessage(request ThreadPoolRequest) {
	// if worker exists, handle message in go routine
	if t.freeWorkerExists() {
		go t.handleMessage(request)
		return
	}
	// wait for free worker
	<-t.signalCh
	// A free worker should definitely exist
	if t.freeWorkerExists() {
		go t.handleMessage(request)
	}
}

func (t *ThreadPool) Len() int {
	return t.size
}

func (t *ThreadPool) incrementWorkers() {
	t.Lock()
	defer t.Unlock()
	t.activeWorkers++
	if t.activeWorkers > t.size {
		t.activeWorkers = t.size
	}
}

func (t *ThreadPool) decrementWorkers() {
	t.Lock()
	defer t.Unlock()
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

func (t *ThreadPool) receiveMessages() {
	for {
		select {
		case <-t.closeCh:
			close(t.DataCh)
			close(t.signalCh)
		case request, ok := <-t.DataCh:
			if !ok {
				return
			}
			t.sendMessage(request)
		}
	}
}
