package pbft

import (
	"net"
	"time"
)

type Operation struct {
	Opcode    int
	Timestamp time.Time
	Op        string
}

type requestInfo struct {
	id        int64
	committed bool
	request   *ClientRequest
}

// REQUEST:
// op, timestamp, client addr (signed by client)
type ClientRequest struct {
	Id        int64 // to prevent request replay
	Opcode    int
	Op        string
	Timestamp time.Time
	Client    net.Addr
}

// REPLY:
// viewnum, timestamp, client addr, node addr, result
// (signed by node)
type ClientReply struct {
	result     string
	viewNumber int
	timestamp  time.Time
	client     net.Addr
	node       net.Addr
}

// PRE-PREPARE:
// viewnum, seqnum, client message (digest)
// (signed by node)
type PrePrepare struct {
	Number SlotId
}

// hehehe peepee
type PPResponse struct {
	SeqNumber int
}

type PrePrepareFull struct {
	PrePrepareMessage PrePrepare
	Request           ClientRequest
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	Number  SlotId
	Message ClientRequest
	Node    NodeId
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	Number  SlotId
	Message ClientRequest
	Node    NodeId
}

// The checkpoint protocol is used to advance the low and high
// watermarks. Low watermark = sequence num of last checkpoint.
// High watermark = Low watermark + (checkpoint delta) * 2

// CHECKPOINT:
// seqnum, digest of state, node addr
// 2f + 1 of these is a proof for a particular seqnum's
// checkpoint
// (signed by node i)
type Checkpoint struct {
	Number SlotId
	Node   NodeId
}

type PreparedProof struct {
	Number     SlotId
	Preprepare PrePrepareFull
	Prepares   map[NodeId]Prepare
}

// TODO: include all proofs/verification stuff
// VIEW CHANGE:
// n: last checkpoint sequence num
// C: Proof of checkpoint at n
// P: Proof of all PREPARED requests after n
//    1 Preprepare message (signed) and 2f+1 prepares per
// (signed by node i)
type ViewChange struct {
	ViewNumber      int                      // v + 1
	Checkpoint      SlotId                   // n
	CheckpointProof map[NodeId]Checkpoint    // C
	Proofs          map[SlotId]PreparedProof // P
	Node            NodeId                   // i
}

func (v ViewChange) verify() bool {
	return true
}

// NEW VIEW
// viewnum, seqnum, client message (digest), node addr
// V: set of 2f valid view-change messages
// O: a set of pre-prepare messages
// (signed by node i)
type NewView struct {
	ViewNumber  int                       // v + 1
	ViewChanges map[NodeId]ViewChange     // V
	PrePrepares map[SlotId]PrePrepareFull // O
	Node        NodeId                    // i
}

// Backup verifies O, multicasts prepares for every message
// in O, then enters new view.

func (v NewView) verify() bool {
	return true
}

type Ack struct {
}

type Message int
