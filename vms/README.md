# Snowman VMs

## Recap of Avalanche Subnets

Quick recap:

Avalanche is a network of subnets. A subnet consists of two things: a validator set and a set of blockchains. On the primary network, the validator set is every validator on the network. The validator set is required to validate the three blockchains on the primary network: the P-Chain, C-Chain, and X-Chain.

A subnet created on the Avalanche network gets created on the P-Chain. The P-Chain can create subnets, add blockchains to those subnets, and add validators to the validator set of subnets. Therefore the P-Chain defines both the set of blockchains on a subnet and the validator set that performs consensus for each blockchain on the subnet.

For each blockchain, consensus is driven by the consensus engine. For each subnet, the P-Chain provides the validator set, and each subnet provides the validator set to each consensus engine on the subnet.

The last piece of the puzzle is VMs. A VM is to a blockchain what a class is to an instance of that class. The consensus engine takes care of driving all of the voting logic necessary to decide what blocks are accepted and rejected.

The VM defines the blockchain's state machine logic. Its primary responsibility is to expose all of the required block handling logic to the consensus engine.

The VM handles building blocks, parsing blocks received from the network, and retrieving blocks from storage. The blocks that it returns follow the (snowman.Block)[https://github.com/ava-labs/avalanchego/blob/master/snow/consensus/snowman/block.go] interface and maintain all of the invariants required by the consensus engine.


## Snowman VM From the Perspective of the Consensus Engine

To the consensus engine, the Snowman VM is a black box that handles all block building, parsing, and storage and provides a simple block interface for the consensus engine to call as it decides blocks.

### Snowman VM Block Handling

The Snowman VM needs to implement the following functions used by the consensus engine during the consensus process.

#### Build Block

Build block allows the VM to propose a new block to be added to consensus.

The VM can send messages to the consensus engine through a `toEngine` channel that is passed in when the VM is initialized. This channel allows the VM to send the consensus engine a message when it is ready to build a block. For example, if the VM receives some transactions via gossip or from an API, then it will signal that it is ready to build a block by sending a `PendingTxs` message to the consensus engine. The PendingTxs message signals to the consensus engine that it should call `BuildBlock()` so that the block can be added to consensus. The major caveat to this is the Snowman VMs are wrapped with Snowman++. Snowman++ provides congestion control by using a soft leader, where a leader is designated as the proposer that should create a block at a given time. Snowman++ gracefully falls back to increase the number of validators that are allowed to propose a block to handle the case that the leader does not propose a block in a timely manner.

Since a VM may be ready to build a block before its turn to propose a block according to Snowman++, the proposer VM will buffer PendingTxs messages until the ProposerVM agrees that it is time to build a block as well.

When the consensus engine does call `BuildBlock`, the VM should build a block on top of the currently preferred block#Set Preference. This increases the likelihood that the block will be accepted since if the VM builds on top of a block that is not preferred, then the consensus engine is already leaning towards accepting something else, such that the newly created block will be more likely to get rejected.

#### Parse Block

Parse block provides the consensus engine the ability to parse a byte array into the block interface.

`ParseBlock(bytes []byte)` attempts to parse a byte array into a block, so that it can return the block interface to the consensus engine. ParseBlock can perform syntactic verification to ensure that a block is well formed. For example, if a certain field of a block is invalid such that the block can be immediately determined to not be valid, ParseBlock can immediately return an error so that the consensus engine does not need to do the extra work of adding it to consensus.

#### GetBlock

GetBlock fetches blocks that are already known to the VM. GetBlock must return a uniquified block if the block is currently in processing. The VM should be able to fetch blocks that have been accepted. However, the VM is not required to fetch blocks that have been marked as rejected.


#### Set Preference

The VM implements the function `SetPreference(blkID ids.ID)` to allow the consensus engine to notify the VM which block is currently preferred to be accepted. The VM should use this information to set the head of its blockchain. Most importantly, when the consensus engine calls BuildBlock, the VM should be sure to build on top of the block that is the most recently set preference.

Note: SetPreference should always be called with a block that has no children known to consnsus.

### Implementing the Snowman VM Block

From the perspective of the consensus engine, the state of the VM can be defined as a linear chain starting from the
genesis block through to the last accepted block.

Following the last accepted block, the consensus engine may have any number of different blocks that are
in processing. The configuration of the processing set can be defined as a tree with the last accepted
block as the root.

In practice, this looks like the following:

    G
    |
    .
    .
    .
    |
    A
    |
    B
  /   \
 C     D

In this example, G -> ... -> A is the linear chain of blocks that have already been accepted by the consensus
engine.

B, with parent block A, has been issued into consensus and is currently in processing.
Blocks C and D, both with parent block B, have also been issued into consensus and are currently in processing
as well.

We will call this state a possible configuration of the ChainVM from the view of the consensus engine, and
we will try to define clearly the set of possible steps from this configuration to the next, which the ChainVM
must implement correctly.

Given a configuration of the consensus engine C, there are three possible actions the consensus engine may take:
1. The consensus engine will attempt to verify a block and issue it to consensus.
2. The consensus engine may change its preference (update the block that it currently prefers to accept).
3. The consensus engine may arrive at a decision and call Accept/Reject on a series of blocks.

If the consensus engine arrives at a decision, then it may have decided one or more blocks and will perform
the following steps:

1. Call Accept sequentially on the decided blocks
2. Call Reject on any blocks that conflict with the just decided block(s) in BFS order.

Therefore, if the tree of blocks in consensus (with root L, the last accepted block), looks like the following:

      L
     / \
    A   B
    |   |\
    C   | \
        D  G
       / \
      E   F

If the consensus engine decides A and C simultaneously, the consensus engine would perform the following ordered
operations:
1. Accept(A)
2. Accept(C)
3. Reject(B)
4. Reject(D)
5. Reject(G)
5. Reject(D)
6. Reject(E)
7. Reject(F)

Note that because rejection occurs in BFS order, G is rejected before E and F are rejected. To see the actual code where
Accept/Reject are performed look in snow/consensus/snowman/topological.go.

### Block Statuses

A block can either be `Acceped`, `Rejected`, `Processing`, or `Unknown`.

A block that is either `Accepted` or `Rejected` is considered to be `Decided`.

#### Unknown Blocks
A block that reports its status as `Unknown` means that we do not know what is in the block ie. we do not have the bytes that represent the block and can only report that it is unkown. For examnple, if we receive a blockID from another node and we don't know what it is, then the consensus engine will consider that block to be unknown.

#### Processing Blocks

A block that reports its status as `Processing` is a block that we have the bytes for and have parsed it, but it has not been decided yet. Processing blocks are a bit annoying because there are actually two distinct states of Processing blocks. If the consensus engine just parsed a block that it is hearing of for the first time, then it should report its status as Processing.

However, this does not indicate that the consensus engine has issued the block into consensus. Before issuing a block to consensus, the consensus engine ensures the invariant that the parent of the block has been successfully verified.

TODO give example

#### Accepted Blocks

After a block has been marked as Accepted, it should report its status as accepted. Accepted blocks should still be retrievable via GetBlock.

#### Rejected Blocks

After a block has been marked as Rejected, it should report its status as rejected.

## Snowman VM APIs

The VM must also implement `CreateHandlers()` which can return a map of extensions mapped to HTTP handlers that will be added to the node's API server. This allows the VM to expose APIs for querying and interacting with the blockchain implemented by the API.