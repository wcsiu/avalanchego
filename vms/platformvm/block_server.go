// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/vms/platformvm/indexer"
)

var _ indexer.BlockServer = &VM{}

// LastAcceptedBlkID implements BlockServer interface
func (vm *VM) LastAcceptedBlkID() ids.ID {
	vm.ctx.Lock.Lock()
	defer vm.ctx.Lock.Unlock()
	return vm.internalState.GetLastAccepted()
}

// GetBlk implements BlockServer interface
func (vm *VM) GetBlk(blkID ids.ID) (snowman.Block, error) {
	vm.ctx.Lock.Lock()
	defer vm.ctx.Lock.Unlock()
	return vm.internalState.GetBlock(blkID)
}
