// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package peer

import (
	"context"
	"sync"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/message"
	"github.com/ava-labs/avalanchego/network/throttling"
	"github.com/ava-labs/avalanchego/utils/logging"
)

var (
	_ MessageQueue = &throttledMessageQueue{}
	_ MessageQueue = &blockingMessageQueue{}
)

type MessageQueue interface {
	Push(ctx context.Context, msg message.OutboundMessage) bool
	Pop() (message.OutboundMessage, bool)
	PopWithoutBlocking() (message.OutboundMessage, bool)
	Close()
}

type throttledMessageQueue struct {
	metrics              *Metrics
	id                   ids.NodeID
	log                  logging.Logger
	outboundMsgThrottler throttling.OutboundMsgThrottler

	// Signalled when a message is added to the queue and when Close() is
	// called.
	cond *sync.Cond

	// closed flags whether the send queue has been closed.
	closed bool

	// queue of the messages
	queue []message.OutboundMessage
}

func NewThrottledMessageQueue(
	metrics *Metrics,
	id ids.NodeID,
	log logging.Logger,
	outboundMsgThrottler throttling.OutboundMsgThrottler,
) MessageQueue {
	return &throttledMessageQueue{
		metrics:              metrics,
		id:                   id,
		log:                  log,
		outboundMsgThrottler: outboundMsgThrottler,

		cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (q *throttledMessageQueue) Push(_ context.Context, msg message.OutboundMessage) bool {
	// Acquire space on the outbound message queue, or drop [msg] if we can't.
	if !q.outboundMsgThrottler.Acquire(msg, q.id) {
		q.log.Debug(
			"dropping %s message to %s due to rate-limiting",
			msg.Op(), q.id,
		)
		q.metrics.SendFailed(msg)
		return false
	}

	// Invariant: must call p.outboundMsgThrottler.Release(msg, p.id) when done
	// sending [msg] or when we give up sending [msg].

	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		q.log.Debug(
			"dropping %s message to %s due to a closed connection",
			msg.Op(), q.id,
		)
		q.outboundMsgThrottler.Release(msg, q.id)
		q.metrics.SendFailed(msg)
		return false
	}

	q.queue = append(q.queue, msg)
	q.cond.Signal()
	return true
}

func (q *throttledMessageQueue) Pop() (message.OutboundMessage, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for {
		if q.closed {
			return nil, false
		}
		if len(q.queue) > 0 {
			// There is a message
			break
		}
		// Wait until there is a message
		q.cond.Wait()
	}

	msg := q.queue[0]
	q.queue[0] = nil
	q.queue = q.queue[1:]

	q.outboundMsgThrottler.Release(msg, q.id)
	return msg, true
}

func (q *throttledMessageQueue) PopWithoutBlocking() (message.OutboundMessage, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if len(q.queue) == 0 {
		// There isn't a message
		return nil, false
	}

	msg := q.queue[0]
	q.queue[0] = nil
	q.queue = q.queue[1:]

	q.outboundMsgThrottler.Release(msg, q.id)
	return msg, true
}

func (q *throttledMessageQueue) Close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true

	for _, msg := range q.queue {
		q.outboundMsgThrottler.Release(msg, q.id)
		q.metrics.SendFailed(msg)
	}
	q.queue = nil

	q.cond.Broadcast()
}

type blockingMessageQueue struct {
	metrics *Metrics
	log     logging.Logger

	closeOnce   sync.Once
	closingLock sync.RWMutex
	closing     chan struct{}

	// queue of the messages
	queue chan message.OutboundMessage
}

func NewBlockingMessageQueue(
	metrics *Metrics,
	log logging.Logger,
	bufferSize int,
) MessageQueue {
	return &blockingMessageQueue{
		metrics: metrics,
		log:     log,

		closing: make(chan struct{}),
		queue:   make(chan message.OutboundMessage, bufferSize),
	}
}

func (q *blockingMessageQueue) Push(ctx context.Context, msg message.OutboundMessage) bool {
	q.closingLock.RLock()
	defer q.closingLock.RUnlock()

	select {
	case <-q.closing:
		q.log.Debug(
			"dropping %s message due to a closed connection",
			msg.Op(),
		)
		q.metrics.SendFailed(msg)
		return false
	default:
	}

	select {
	case q.queue <- msg:
		return true
	case <-ctx.Done():
		q.log.Debug(
			"dropping %s message due to a cancelled context",
			msg.Op(),
		)
		q.metrics.SendFailed(msg)
		return false
	case <-q.closing:
		q.log.Debug(
			"dropping %s message due to a closed connection",
			msg.Op(),
		)
		q.metrics.SendFailed(msg)
		return false
	}
}

func (q *blockingMessageQueue) Pop() (message.OutboundMessage, bool) {
	select {
	case msg := <-q.queue:
		return msg, true
	case <-q.closing:
		return nil, false
	}
}

func (q *blockingMessageQueue) PopWithoutBlocking() (message.OutboundMessage, bool) {
	select {
	case msg := <-q.queue:
		return msg, true
	default:
		return nil, false
	}
}

func (q *blockingMessageQueue) Close() {
	q.closeOnce.Do(func() {
		close(q.closing)

		q.closingLock.Lock()
		defer q.closingLock.Unlock()

		for {
			select {
			case msg := <-q.queue:
				q.metrics.SendFailed(msg)
			default:
				return
			}
		}
	})
}
