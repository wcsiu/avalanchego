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

var _ tPool = &threadPool{}

type tPool interface {
	worker()
	send(threadPoolRequest)
}

type threadPoolRequest struct {
	Request func() error
	NodeID  ids.ShortID
	Op      string
}

type threadPool struct {
	sync.Mutex
	size       int
	dataCh     chan threadPoolRequest
	clock      mockable.Clock
	cpuTracker tracker.TimeTracker
	log        logging.Logger
}

func newThreadPool(size int, cpuTracker tracker.TimeTracker) *threadPool {
	tPool := new(threadPool)
	tPool.size = size
	tPool.cpuTracker = cpuTracker
	tPool.dataCh = make(chan threadPoolRequest)
	for w := 1; w <= size; w++ {
		go tPool.worker()
	}
	return tPool
}

func (t *threadPool) worker() {
	for request := range t.dataCh {
		t.cpuTracker.StartCPU(request.NodeID, t.clock.Time())
		err := request.Request()
		t.cpuTracker.StopCPU(request.NodeID, t.clock.Time())
		if err != nil {
			t.log.Info("Request of type %s from node ID %s failed with err: %s", request.Op, request.NodeID, err)
		}
	}
}

func (t *threadPool) send(msg threadPoolRequest) {
	t.dataCh <- msg
}
