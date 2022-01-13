// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"context"
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/utils/rpc"
)

var errInvalidNumberOfChecks = errors.New("expected at least 1 check attempt")

// Interface compliance
var _ Client = &client{}

// Client interface for Avalanche Health API Endpoint
type Client interface {
	// Readiness returns if the node has finished initialization
	Readiness(ctx context.Context) (*APIHealthReply, error)
	// Health returns a summation of the health of the node
	Health(ctx context.Context) (*APIHealthReply, error)
	// Liveness returns if the node is in need of a restart
	Liveness(ctx context.Context) (*APIHealthReply, error)
	// AwaitHealthy queries the Health endpoint [checks] times, with a pause of
	// [interval] in between checks and returns early if Health returns healthy
	AwaitHealthy(ctx context.Context, numChecks int, freq time.Duration) (bool, error)
}

// Client implementation for Avalanche Health API Endpoint
type client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a client to interact with Health API endpoint
func NewClient(uri string) Client {
	return &client{
		requester: rpc.NewEndpointRequester(uri, "/ext/health", "health"),
	}
}

func (c *client) Readiness(ctx context.Context) (*APIHealthReply, error) {
	res := &APIHealthReply{}
	err := c.requester.SendRequest(ctx, "readiness", struct{}{}, res)
	return res, err
}

func (c *client) Health(ctx context.Context) (*APIHealthReply, error) {
	res := &APIHealthReply{}
	err := c.requester.SendRequest(ctx, "health", struct{}{}, res)
	return res, err
}

func (c *client) Liveness(ctx context.Context) (*APIHealthReply, error) {
	res := &APIHealthReply{}
	err := c.requester.SendRequest(ctx, "liveness", struct{}{}, res)
	return res, err
}

func (c *client) AwaitHealthy(ctx context.Context, numChecks int, freq time.Duration) (bool, error) {
	if numChecks < 1 {
		return false, errInvalidNumberOfChecks
	}

	// Check health once outside the loop to avoid sleeping unnecessarily.
	res, err := c.Health(ctx)
	if err == nil && res.Healthy {
		return true, nil
	}

	for i := 1; i < numChecks; i++ {
		time.Sleep(freq)
		res, err = c.Health(ctx)
		if err == nil && res.Healthy {
			return true, nil
		}
	}
	return false, err
}
