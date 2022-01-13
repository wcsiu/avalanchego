// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	reply  APIHealthReply
	err    error
	onCall func()
}

func (mc *mockClient) SendRequest(ctx context.Context, method string, params interface{}, replyIntf interface{}) error {
	reply := replyIntf.(*APIHealthReply)
	*reply = mc.reply
	mc.onCall()
	return mc.err
}

func TestNewClient(t *testing.T) {
	assert := assert.New(t)

	c := NewClient("")
	assert.NotNil(c)
}

func TestClient(t *testing.T) {
	assert := assert.New(t)

	mc := &mockClient{
		reply: APIHealthReply{
			Healthy: true,
		},
		err:    nil,
		onCall: func() {},
	}
	c := client{
		requester: mc,
	}

	{
		readiness, err := c.Readiness(context.Background())
		assert.NoError(err)
		assert.True(readiness.Healthy)
	}

	{
		health, err := c.Health(context.Background())
		assert.NoError(err)
		assert.True(health.Healthy)
	}

	{
		liveness, err := c.Liveness(context.Background())
		assert.NoError(err)
		assert.True(liveness.Healthy)
	}

	{
		gctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
		healthy, err := c.AwaitHealthy(gctx, time.Second)
		assert.NoError(err)
		assert.True(healthy)
	}

	mc.reply.Healthy = false

	{
		gctx, _ := context.WithTimeout(context.Background(), 20*time.Microsecond)
		healthy, err := c.AwaitHealthy(gctx, time.Microsecond)
		assert.NoError(err)
		assert.False(healthy)
	}

	mc.onCall = func() {
		mc.reply.Healthy = true
	}

	{
		healthy, err := c.AwaitHealthy(context.Background(), time.Microsecond)
		assert.NoError(err)
		assert.True(healthy)
	}
}
