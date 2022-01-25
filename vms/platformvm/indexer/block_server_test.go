// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package indexer

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
)

var (
	errLastAcceptedBlkID = errors.New("unexpectedly called LastAcceptedBlkID")
	errGetBlk            = errors.New("unexpectedly called GetBlk")

	_ BlockServer = &TestBlockServer{}
)

// TestBatchedVM is a BatchedVM that is useful for testing.
type TestBlockServer struct {
	T *testing.T

	CantLastAcceptedBlkID bool
	CantGetBlk            bool

	LastAcceptedBlkIDF func() ids.ID
	GetBlkF            func(blkID ids.ID) (snowman.Block, error)
}

func (tsb *TestBlockServer) LastAcceptedBlkID() ids.ID {
	if tsb.LastAcceptedBlkIDF != nil {
		return tsb.LastAcceptedBlkIDF()
	}
	if tsb.CantLastAcceptedBlkID && tsb.T != nil {
		tsb.T.Fatal(errLastAcceptedBlkID)
	}
	return ids.Empty
}

func (tsb *TestBlockServer) GetBlk(blkID ids.ID) (snowman.Block, error) {
	if tsb.GetBlkF != nil {
		return tsb.GetBlkF(blkID)
	}
	if tsb.CantGetBlk && tsb.T != nil {
		tsb.T.Fatal(errGetBlk)
	}
	return nil, errGetBlk
}
