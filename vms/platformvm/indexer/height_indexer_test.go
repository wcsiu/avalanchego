// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package indexer

import (
	"math/rand"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/manager"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/version"
	heightIndex "github.com/ava-labs/avalanchego/vms/components/block_height_index"
	"github.com/stretchr/testify/assert"
)

var (
	genesisUnixTimestamp int64 = 1000
	genesisTimestamp           = time.Unix(genesisUnixTimestamp, 0)
)

func TestHeightBlockIndex(t *testing.T) {
	assert := assert.New(t)

	// Build a chain
	blkID := ids.Empty.Prefix(0)
	genesisBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID,
			StatusV: choices.Accepted,
		},
		HeightV:    0,
		TimestampV: genesisTimestamp,
		BytesV:     []byte{0},
	}

	var (
		blkNumber = uint64(10)
		lastBlk   = snowman.Block(genesisBlk)
		blocks    = make(map[ids.ID]snowman.Block)
	)
	blocks[genesisBlk.ID()] = genesisBlk

	for blkHeight := uint64(1); blkHeight <= blkNumber; blkHeight++ {
		blkID := ids.Empty.Prefix(blkHeight)
		blk := &snowman.TestBlock{
			TestDecidable: choices.TestDecidable{
				IDV:     blkID,
				StatusV: choices.Accepted,
			},
			BytesV:  []byte{uint8(blkHeight)},
			ParentV: lastBlk.ID(),
			HeightV: blkHeight,
		}
		blocks[blk.ID()] = blk
		lastBlk = blk
	}

	blkSrv := &TestBlockServer{
		CantLastAcceptedBlkID: true,
		CantGetBlk:            true,

		LastAcceptedBlkIDF: func() ids.ID { return lastBlk.ID() },
		GetBlkF: func(id ids.ID) (snowman.Block, error) {
			blk, found := blocks[id]
			if !found {
				return nil, database.ErrNotFound
			}
			return blk, nil
		},
	}

	dbMan := manager.NewMemDB(version.DefaultVersion1_0_0)
	storedState := heightIndex.New(dbMan.Current().Database)
	hIndex := newHeightIndexer(blkSrv,
		logging.NoLog{},
		storedState,
	)
	hIndex.commitMaxSize = 0 // commit each block

	// show that height index should be rebuild and it is
	doRepair, startBlkID, err := hIndex.shouldRepair()
	assert.NoError(err)
	assert.True(doRepair)
	assert.True(startBlkID == lastBlk.ID())
	assert.NoError(hIndex.doRepair(startBlkID))
	assert.NoError(hIndex.batch.Write()) // batch write responsibility is on doRepair caller

	// check that height index is fully built
	for height := uint64(1); height <= blkNumber; height++ {
		_, err := storedState.GetBlockIDAtHeight(height)
		assert.NoError(err)
	}

	// check that height index wont' be rebuild anymore
	assert.False(hIndex.shouldRepair())
	assert.True(hIndex.IsRepaired())
}

func TestHeightBlockIndexResumeFromCheckPoint(t *testing.T) {
	assert := assert.New(t)

	// Build a chain
	blkID := ids.Empty.Prefix(0)
	genesisBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID,
			StatusV: choices.Accepted,
		},
		HeightV:    0,
		TimestampV: genesisTimestamp,
		BytesV:     []byte{0},
	}

	var (
		blkNumber = uint64(10)
		lastBlk   = snowman.Block(genesisBlk)
		blocks    = make(map[ids.ID]snowman.Block)
	)
	blocks[genesisBlk.ID()] = genesisBlk

	for blkHeight := uint64(1); blkHeight < blkNumber; blkHeight++ {
		blkID := ids.Empty.Prefix(blkHeight)
		lastBlk = &snowman.TestBlock{
			TestDecidable: choices.TestDecidable{
				IDV:     blkID,
				StatusV: choices.Accepted,
			},
			BytesV:  []byte{uint8(blkHeight)},
			ParentV: lastBlk.ID(),
			HeightV: blkHeight,
		}
		blocks[lastBlk.ID()] = lastBlk
	}

	blkSrv := &TestBlockServer{
		CantLastAcceptedBlkID: true,
		CantGetBlk:            true,

		LastAcceptedBlkIDF: func() ids.ID { return lastBlk.ID() },
		GetBlkF: func(id ids.ID) (snowman.Block, error) {
			blk, found := blocks[id]
			if !found {
				return nil, database.ErrNotFound
			}
			return blk, nil
		},
	}

	dbMan := manager.NewMemDB(version.DefaultVersion1_0_0)
	storedState := heightIndex.New(dbMan.Current().Database)
	hIndex := newHeightIndexer(blkSrv,
		logging.NoLog{},
		storedState,
	)
	hIndex.commitMaxSize = 0 // commit each block

	// with no checkpoints repair starts from last accepted block
	doRepair, startBlkID, err := hIndex.shouldRepair()
	assert.NoError(hIndex.batch.Write()) // batch write responsibility is on shouldRepair caller
	assert.True(doRepair)
	assert.NoError(err)
	assert.True(startBlkID == lastBlk.ID())

	// pick a random block in the chain and checkpoint it;...
	rndPostForkHeight := rand.Intn(int(blkNumber)) // #nosec G404
	var checkpointBlk snowman.Block
	for _, blk := range blocks {
		if blk.Height() != uint64(rndPostForkHeight) {
			continue // not the blk we are looking for
		}

		checkpointBlk = blk
		assert.NoError(hIndex.indexState.SetCheckpoint(checkpointBlk.ID()))
		break
	}

	// ...show that repair starts from the checkpoint
	doRepair, startBlkID, err = hIndex.shouldRepair()
	assert.True(doRepair)
	assert.NoError(err)
	assert.NoError(hIndex.batch.Write()) // batch write responsibility is on shouldRepair caller
	assert.True(startBlkID == checkpointBlk.ID())
	assert.False(hIndex.IsRepaired())

	// perform repair and show index is built
	assert.NoError(hIndex.doRepair(startBlkID))
	assert.NoError(hIndex.batch.Write()) // batch write responsibility is on doRepair caller

	// check that height index is fully built
	for height := uint64(0); height <= checkpointBlk.Height(); height++ {
		_, err := storedState.GetBlockIDAtHeight(height)
		assert.NoError(err)
	}
}
