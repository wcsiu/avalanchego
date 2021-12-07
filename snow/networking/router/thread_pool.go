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

var _ TPool = &ThreadPool{}

type TPool interface {
	worker(int, chan ThreadPoolRequest)
	Len() int
	CloseCh()
}

type ThreadPoolRequest struct {
	Request func() error
	NodeID  ids.ShortID
	Op      string
}

type ThreadPool struct {
	sync.Mutex
	size       int
	DataCh     chan ThreadPoolRequest
	clock      mockable.Clock
	cpuTracker tracker.TimeTracker
	log        logging.Logger
}

func NewThreadPool(size int, cpuTracker tracker.TimeTracker) *ThreadPool {
	tPool := new(ThreadPool)
	tPool.size = size
	tPool.cpuTracker = cpuTracker
	tPool.DataCh = make(chan ThreadPoolRequest, size)
	for w := 1; w <= size; w++ {
		go tPool.worker(w, tPool.DataCh)
	}
	return tPool
}

func (t *ThreadPool) worker(id int, dataCh chan ThreadPoolRequest) {
	for request := range dataCh {
		t.cpuTracker.StartCPU(request.NodeID, t.clock.Time())
		err := request.Request()
		t.cpuTracker.StopCPU(request.NodeID, t.clock.Time())
		if err != nil {
			t.log.Info("Request of type %s from node ID %s on worker ID %d failed with err: %s", request.Op, request.NodeID, id, err)
		}
	}
}

func (t *ThreadPool) Len() int {
	return t.size
}

func (t *ThreadPool) CloseCh() {
	close(t.DataCh)
}
