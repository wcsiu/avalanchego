// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
)

var errIndexIncomplete = errors.New("query failed because height index is incomplete")

// HeightIndexingEnabled implements HeightIndexedChainVM interface
// vm.ctx.Lock should be held
func (vm *VM) IsHeightIndexComplete() bool {
	return vm.HeightIndexer.IsRepaired()
}

// GetBlockIDByHeight implements HeightIndexedChainVM interface
// vm.ctx.Lock should be held
func (vm *VM) GetBlockIDByHeight(height uint64) (ids.ID, error) {
	if !vm.IsHeightIndexComplete() {
		return ids.Empty, errIndexIncomplete
	}

	return vm.internalState.GetBlockIDAtHeight(height)
}

// As blocks/options are accepted, height index is updated
// even if its repairing is ongoing.
// vm.ctx.Lock should be held
func (vm *VM) updateHeightIndex(height uint64, blkID ids.ID) error {
	if err := vm.internalState.SetBlockIDAtHeight(height, blkID); err != nil {
		vm.ctx.Log.Warn("Block indexing by height: new block. Failed updating index %w", err)
		return err
	}

	vm.ctx.Log.Debug("Block indexing by height: added block %s at height %d", blkID, height)
	return nil
}
