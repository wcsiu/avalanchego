// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	heightIndex "github.com/ava-labs/avalanchego/vms/components/block_height_index"
)

var (
	chainStatePrefix  = []byte("chain")
	blockStatePrefix  = []byte("block")
	heightIndexPrefix = []byte("heightBlk")
)

type State interface {
	ChainState
	BlockState
	heightIndex.Index
}

type state struct {
	ChainState
	BlockState
	heightIndex.Index
}

func New(db database.Database) State {
	chainDB := prefixdb.New(chainStatePrefix, db)
	blockDB := prefixdb.New(blockStatePrefix, db)
	heightIndexDB := prefixdb.New(heightIndexPrefix, db)
	return &state{
		ChainState: NewChainState(chainDB),
		BlockState: NewBlockState(blockDB),
		Index:      heightIndex.New(heightIndexDB),
	}
}

func NewMetered(db database.Database, namespace string, metrics prometheus.Registerer) (State, error) {
	chainDB := prefixdb.New(chainStatePrefix, db)
	blockDB := prefixdb.New(blockStatePrefix, db)
	heightIndexDB := prefixdb.New(heightIndexPrefix, db)

	blockState, err := NewMeteredBlockState(blockDB, namespace, metrics)
	if err != nil {
		return nil, err
	}

	return &state{
		ChainState: NewChainState(chainDB),
		BlockState: blockState,
		Index:      heightIndex.New(heightIndexDB),
	}, nil
}

func (s *state) clearCache() {
	s.ChainState.clearCache()
	s.BlockState.clearCache()
	s.Index.ClearCache()
}
