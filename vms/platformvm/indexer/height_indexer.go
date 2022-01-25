// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package indexer

import (
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/units"
	heightIndex "github.com/ava-labs/avalanchego/vms/components/block_height_index"
)

const defaultCommitSizeCap = 1 * units.MiB

var _ HeightIndexer = &heightIndexer{}

type HeightIndexer interface {
	// checks whether the index is fully repaired or not
	IsRepaired() bool

	// checks whether index rebuilding is needed and if so, performs it
	RepairHeightIndex() error
}

func NewHeightIndexer(srv BlockServer,
	log logging.Logger,
	indexState heightIndexDBOps) HeightIndexer {
	return newHeightIndexer(srv, log, indexState)
}

func newHeightIndexer(srv BlockServer,
	log logging.Logger,
	indexState heightIndexDBOps) *heightIndexer {
	res := &heightIndexer{
		server:        srv,
		log:           log,
		indexState:    indexState,
		batch:         indexState.NewBatch(),
		commitMaxSize: defaultCommitSizeCap,
	}

	return res
}

type heightIndexer struct {
	server BlockServer
	log    logging.Logger

	jobDone    utils.AtomicBool
	indexState heightIndexDBOps
	batch      database.Batch

	commitMaxSize int
}

func (hi *heightIndexer) IsRepaired() bool {
	return hi.jobDone.GetValue()
}

// RepairHeightIndex ensures the height -> blkID height block index is well formed.
// Starting from last accepted block, it will go back to genesis.
// RepairHeightIndex can take a non-trivial time to complete; hence we make sure
// the process has limited memory footprint, can be resumed from periodic checkpoints
// and works asynchronously without blocking the VM.
func (hi *heightIndexer) RepairHeightIndex() error {
	needRepair, startBlkID, err := hi.shouldRepair()
	if err != nil {
		hi.log.Error("Block indexing by height starting: failed. Could not determine if index is complete, error %v", err)
		return err
	}
	if err := hi.batch.Write(); err != nil {
		hi.log.Warn("Failed writing height index batch, err %w", err)
		return err
	}

	if !needRepair {
		return nil
	}

	if err := hi.doRepair(startBlkID); err != nil {
		return err
	}
	if err := hi.batch.Write(); err != nil {
		hi.log.Warn("Failed writing height index batch, err %w", err)
		return err
	}
	return nil
}

// shouldRepair checks if height index is complete;
// if not, it returns the checkpoint from which repairing should start.
// Note: batch commit is deferred to shouldRepair caller
func (hi *heightIndexer) shouldRepair() (bool, ids.ID, error) {
	switch checkpointID, err := hi.indexState.GetCheckpoint(); err {
	case nil:
		// checkpoint found, repair must be resumed
		hi.log.Info("Block indexing by height starting: success. Retrieved checkpoint %v", checkpointID)
		return true, checkpointID, nil

	case database.ErrNotFound:
		// no checkpoint. Either index is complete or repair was never attempted.
		hi.log.Info("Block indexing by height starting: checkpoint not found. Verifying index is complete...")

	default:
		return true, ids.Empty, err
	}

	// index is complete iff lastAcceptedBlock is indexed
	latestBlkID := hi.server.LastAcceptedBlkID()
	lastAcceptedBlk, err := hi.server.GetBlk(latestBlkID)
	if err != nil {
		hi.log.Warn("Block indexing by height starting: could not retrieve last accepted block, err %w", err)
		return true, ids.Empty, err
	}

	switch _, err = hi.indexState.GetBlockIDAtHeight(lastAcceptedBlk.Height()); err {
	case nil:
		// index is complete already.
		hi.jobDone.SetValue(true)
		hi.log.Info("Block indexing by height starting: Index already complete, nothing to do.")
		return false, ids.Empty, nil

	case database.ErrNotFound:
		// Index needs repairing. Mark the checkpoint so that,
		// in case new blocks are accepted while indexing is ongoing,
		// and the process is terminated before first commit,
		// we do not miss rebuilding the full index.
		if err := hi.batch.Put(heightIndex.GetCheckpointKey(), latestBlkID[:]); err != nil {
			return true, ids.Empty, err
		}

		// it will commit on exit
		hi.log.Info("Block indexing by height starting: index incomplete. Rebuilding from %v", latestBlkID)
		return true, latestBlkID, nil

	default:
		return true, ids.Empty, err
	}
}

// if height index needs repairing, doRepair would do that. It
// iterates back via parents, checking and rebuilding height indexing.
// Note: batch commit is deferred to doRepair caller
func (hi *heightIndexer) doRepair(repairStartBlkID ids.ID) error {
	var (
		currentBlkID = repairStartBlkID

		start       = time.Now()
		lastLogTime = start
		indexedBlks = 0
	)
	for {
		currentAcceptedBlk, err := hi.server.GetBlk(currentBlkID)
		switch err {
		case nil:

		case database.ErrNotFound:
			// visited all blocks. Let's delete checkpoint
			if err := hi.batch.Delete(heightIndex.GetCheckpointKey()); err != nil {
				return err
			}
			hi.jobDone.SetValue(true)

			// it will commit on exit
			hi.log.Info("Block indexing by height: completed. Indexed %d blocks, duration %v", indexedBlks, time.Since(start))
			return nil

		default:
			return err
		}

		switch _, err = hi.indexState.GetBlockIDAtHeight(currentAcceptedBlk.Height()); err {
		case nil:
			hi.log.AssertTrue(err != nil, "unexpected height index entry at height %d", currentAcceptedBlk.Height())

		case database.ErrNotFound:
			// Rebuild height block index.
			entryKey := heightIndex.GetEntryKey(currentAcceptedBlk.Height())
			if err := hi.batch.Put(entryKey, currentBlkID[:]); err != nil {
				return err
			}

			// Keep memory footprint under control by committing when a size threshold is reached
			if hi.batch.Size() > hi.commitMaxSize {
				// find and store checkpoint
				if err := hi.doCheckpoint(currentAcceptedBlk); err != nil {
					return err
				}

				// finally commit and reset batch for reuse
				committedSize := hi.batch.Size()
				if err := hi.batch.Write(); err != nil {
					return err
				}
				hi.batch.Reset()

				hi.log.Info("Block indexing by height: ongoing. Indexed %d blocks, latest committed height %d, committed %d bytes",
					indexedBlks, currentAcceptedBlk.Height()+1, committedSize)
			}

			// Periodically log progress
			indexedBlks++
			if time.Since(lastLogTime) > 15*time.Second {
				lastLogTime = time.Now()
				hi.log.Info("Block indexing by height: ongoing. Indexed %d blocks, latest indexed height %d",
					indexedBlks, currentAcceptedBlk.Height()+1)
			}

			// keep checking the parent
			currentBlkID = currentAcceptedBlk.Parent()
		default:
			return err
		}
	}
}

func (hi *heightIndexer) doCheckpoint(currentBlk snowman.Block) error {
	// checkpoint is current block's parent, if it exists
	var checkpoint ids.ID
	parentBlkID := currentBlk.Parent()
	switch checkpointBlk, err := hi.server.GetBlk(parentBlkID); err {
	case nil:
		checkpoint = checkpointBlk.ID()
		if err := hi.batch.Put(heightIndex.GetCheckpointKey(), checkpoint[:]); err != nil {
			return err
		}
		hi.log.Info("Block indexing by height. Stored checkpoint %v at height %d",
			currentBlk.ID(), currentBlk.Height())
		return nil

	case database.ErrNotFound:
		// checkpointBlk is genesis. We do not checkpoint here.
		// Caller will handle this and terminate
		return nil

	default:
		return err
	}
}
