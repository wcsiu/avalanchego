package test

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
)

// From the perspective of the consensus engine, the state of the VM can be defined as a linear chain starting from the
// genesis block through to the last accepted block.
//
// Following the last accepted block, the consensus engine may have any number of different blocks that are
// in processing. The configuration of the processing set can be defined as a tree with the last accepted
// block as the root.
//
// In practice, this looks like the following:
//
//     G
//     |
//     .
//     .
//     .
//     |
//     A
//     |
//     B
//   /   \
//  C     D
//
// In this example, G -> ... -> A is the linear chain of blocks that have already been accepted by the consensus
// engine.
//
// B, with parent block A, has been issued into consensus and is currently in processing.
// Blocks C and D, both with parent block B, have also been issued into consensus and are currently in processing
// as well.
//
// We will call this state a possible configuration of the ChainVM from the view of the consensus engine, and
// we will try to define clearly the set of possible steps from this configuration to the next, which the ChainVM
// must implement correctly.
//
// Given a configuration of the consensus engine C, there are three possible actions the consensus engine may take:
// 1. The consensus engine will attempt to verify a block and issue it to consensus.
// 2. The consensus engine may change its preference (update the block that it currently prefers to accept).
// 3. The consensus engine may arrive at a decision and call Accept/Reject on a series of blocks.
//
// If the consensus engine arrives at a decision, then it may have decided one or more blocks and will perform
// the following steps:
//
// 1. Call Accept sequentially on the decided blocks
// 2. Call Reject on any blocks that conflict with the just decided block(s) in BFS order.
//
// Therefore, if the tree of blocks in consensus (with root L, the last accepted block), looks like the following:
//
//       L
//      / \
//     A   B
//     |   |\
//     C   | \
//         D  G
//        / \
//       E   F
//
// If the consensus engine decides A and C simultaneously, the consensus engine would perform the following ordered
// operations:
// 1. Accept(A)
// 2. Accept(C)
// 3. Reject(B)
// 4. Reject(D)
// 5. Reject(G)
// 5. Reject(D)
// 6. Reject(E)
// 7. Reject(F)
//
// Note that because rejection occurs in BFS order, G is rejected before E and F are rejected. To see the actual code where
// Accept/Reject are performed look in snow/consensus/snowman/topological.go.

type Block struct {
	parent   *Block
	block    snowman.Block
	children []*Block
}

// Configuration defines the last accepted block, the tree of blocks currently in consensus,
// and the currently preferred block.
type Configuration struct {
	lastAcceptedBlock *Block

	preferredBlock   *Block
	processingBlocks map[ids.ID]*Block
}

type OperationHandler interface {
	HandleNextBlock(block snowman.Block)
	HandleSetPreference(blockID ids.ID)
	HandleAccept(block snowman.Block)
}

// operation exists and we implement Handle on each operation and pass in the OperationHandler

type TestableVM interface {
	// NextBlock returns a new block to be issued to consensus
	NextBlock() (snowman.Block, error)
}

func executeTest(t *testing.T) {
}
