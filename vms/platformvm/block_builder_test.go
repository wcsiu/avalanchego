// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
)

// shows that a locally generated CreateChainTx can be added to mempool and then
// removed by inclusion in a block
func TestBlockBuilderAddLocalTx(t *testing.T) {
	assert := assert.New(t)
	vm, _, _, _ := defaultVM()
	vm.ctx.Lock.Lock()
	defer func() {
		assert.NoError(vm.Shutdown())
		vm.ctx.Lock.Unlock()
	}()
	vm.gossipActivationTime = time.Unix(0, 0) // enable mempool gossiping
	mempool := &vm.blockBuilder

	// add a tx to it
	tx := getValidTx(vm, t)
	txID := tx.ID()

	err := mempool.AddUnverifiedTx(tx)
	assert.NoError(err, "couldn't add tx to mempool")

	has := mempool.Has(txID)
	assert.True(has, "valid tx not recorded into mempool")

	// show that build block include that tx and removes it from mempool
	blkIntf, err := vm.BuildBlock()
	assert.NoError(err, "couldn't build block out of mempool")

	blk, ok := blkIntf.(*StandardBlock)
	assert.True(ok, "expected standard block")
	assert.Len(blk.Txs, 1, "standard block should include a single transaction")
	assert.Equal(txID, blk.Txs[0].ID(), "standard block does not include expected transaction")

	has = mempool.Has(txID)
	assert.False(has, "tx included in block is still recorded into mempool")
}

// shows that valid tx is not added to mempool if this would exceed its maximum
// size
func TestBlockBuilderMaxMempoolSizeHandling(t *testing.T) {
	assert := assert.New(t)
	vm, _, _, _ := defaultVM()
	vm.ctx.Lock.Lock()
	defer func() {
		assert.NoError(vm.Shutdown())
		vm.ctx.Lock.Unlock()
	}()
	vm.gossipActivationTime = time.Unix(0, 0) // enable mempool gossiping
	blockBuilder := &vm.blockBuilder
	mempool := blockBuilder.Mempool.(*mempool)

	// create candidate tx
	tx := getValidTx(vm, t)

	// shortcut to simulated almost filled mempool
	mempool.bytesAvailable = len(tx.Bytes()) - 1

	err := blockBuilder.AddVerifiedTx(tx)
	assert.Equal(errMempoolFull, err, "max mempool size breached")

	// shortcut to simulated almost filled mempool
	mempool.bytesAvailable = len(tx.Bytes())

	err = blockBuilder.AddVerifiedTx(tx)
	assert.NoError(err, "should have added tx to mempool")
}

func TestPreviouslyDroppedTxsCanBeReAddedToMempool(t *testing.T) {
	assert := assert.New(t)
	vm, _, _, _ := defaultVM()
	vm.ctx.Lock.Lock()
	defer func() {
		assert.NoError(vm.Shutdown())
		vm.ctx.Lock.Unlock()
	}()
	vm.gossipActivationTime = time.Unix(0, 0) // enable mempool gossiping
	blockBuilder := &vm.blockBuilder
	mempool := blockBuilder.Mempool.(*mempool)

	// create candidate tx
	tx := getValidTx(vm, t)
	txID := tx.ID()

	// A tx simply added to mempool is obviously not marked as dropped
	assert.NoError(mempool.Add(tx))
	assert.True(mempool.Has(txID))
	_, isDropped := mempool.GetDropReason(txID)
	assert.False(isDropped)

	// When a tx is marked as dropped, it is still available to allow re-issuance
	vm.mempool.MarkDropped(txID, "dropped for testing")
	assert.True(mempool.Has(txID)) // still available
	_, isDropped = mempool.GetDropReason(txID)
	assert.True(isDropped)

	// A previously dropped tx, popped then re-added to mempool,
	// is not dropped anymore
	switch tx.Unsigned.(type) {
	case *txs.AddValidatorTx, *txs.AddDelegatorTx, *txs.AddSubnetValidatorTx:
		mempool.PopProposalTx()
	case *txs.CreateChainTx, *txs.CreateSubnetTx, *txs.ImportTx, *txs.ExportTx:
		mempool.PopDecisionTxs(math.MaxInt64)
	default:
		t.Fatal("unknown tx type")
	}
	assert.NoError(mempool.Add(tx))

	assert.True(mempool.Has(txID))
	_, isDropped = mempool.GetDropReason(txID)
	assert.False(isDropped)
}
