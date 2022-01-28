// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package proposervm

import (
	"errors"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
)

var errIndexIncomplete = errors.New("query failed because height index is incomplete")

// IsEnabled implements HeightIndexedChainVM interface
// vm.ctx.Lock should be held
func (vm *VM) IsHeightIndexingEnabled() bool {
	innerHVM, ok := vm.ChainVM.(block.HeightIndexedChainVM)
	if !ok {
		return false
	}
	return innerHVM.IsHeightIndexingEnabled()
}

// IsHeightIndexComplete implements HeightIndexedChainVM interface
// vm.ctx.Lock should be held
func (vm *VM) IsHeightIndexComplete() bool {
	return vm.hIndexer.IsRepaired()
}

// GetBlockIDByHeight implements HeightIndexedChainVM interface
// vm.ctx.Lock should be held
func (vm *VM) GetBlockIDByHeight(height uint64) (ids.ID, error) {
	if !vm.hIndexer.IsRepaired() {
		return ids.Empty, errIndexIncomplete
	}

	// preFork blocks are indexed in innerVM only
	forkHeight, err := vm.State.GetForkHeight()
	if err != nil {
		return ids.Empty, err
	}

	if height < forkHeight {
		innerHVM, _ := vm.ChainVM.(block.HeightIndexedChainVM)
		return innerHVM.GetBlockIDByHeight(height)
	}

	// postFork blocks are indexed in proposerVM
	return vm.State.GetBlockIDAtHeight(height)
}

// As postFork blocks/options are accepted, height index is updated
// even if its repairing is ongoing.
// updateHeightIndex should not be called for preFork blocks. Moreover
// vm.ctx.Lock should be held
func (vm *VM) updateHeightIndex(height uint64, blkID ids.ID) error {
	checkpoint, err := vm.State.GetCheckpoint()
	switch err {
	case nil:
		// index rebuilding is ongoing. We can update the index,
		// stepping away from checkpointed blk, which will be handled by indexer.
		if blkID != checkpoint {
			return vm.storeHeightEntry(height, blkID)
		}

	case database.ErrNotFound:
		// no checkpoint means indexing is not started or is already done
		if vm.hIndexer.IsRepaired() {
			return vm.storeHeightEntry(height, blkID)
		}

	default:
		return err
	}
	return nil
}

func (vm *VM) storeHeightEntry(height uint64, blkID ids.ID) error {
	forkHeight, err := vm.State.GetForkHeight()
	if err != nil {
		vm.ctx.Log.Warn("Block indexing by height: new block. Could not load fork height %v", err)
		return err
	}

	if forkHeight > height {
		vm.ctx.Log.Info("Block indexing by height: new block. Moved fork height from %d to %d with block %v",
			forkHeight, height, blkID)

		if err := vm.State.SetForkHeight(height); err != nil {
			vm.ctx.Log.Warn("Block indexing by height: new block. Failed storing new fork height %v", err)
			return err
		}
	}

	if err = vm.State.SetBlockIDAtHeight(height, blkID); err != nil {
		vm.ctx.Log.Warn("Block indexing by height: new block. Failed updating index %v", err)
		return err
	}

	vm.ctx.Log.Debug("Block indexing by height: added block %s at height %d", blkID, height)
	return vm.db.Commit()
}
