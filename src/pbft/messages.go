package pbft

import (
	"crypto/sha256"
	"net"
	"time"
)

// REPLY:
// viewnum, timestamp, client addr, node addr, result
// (signed by node)
type ClientReply struct {
	result     string
	viewNumber int
	timestamp  time.Time
	client     net.Addr
	node       net.Addr
	digest     [sha256.Size]byte
}

// PRE-PREPARE:
// viewnum, seqnum, client message (digest)
// (signed by node)
type PrePrepare struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
}

type SignedPrePrepare struct {
	PrePrepareMessage PrePrepare
	Signature         []byte
}

type FullPrePrepare struct {
	SignedMessage SignedPrePrepare
	Request       string
}

// hehehe peepee
type PPResponse struct {
	SeqNumber int
}

type SignedPPResponse struct {
	Response  PPResponse
	Signature []byte
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
	Node          NodeId
}

type SignedPrepare struct {
	PrepareMessage Prepare
	Signature      []byte
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
	Node          NodeId
	Digest        [sha256.Size]byte
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
	Number   SlotId
	Snapshot []byte
	Node     NodeId
	// TODO: also needs a digest i think
}

type CheckpointProof struct {
	Number   SlotId
	Snapshot []byte
	Proof    map[NodeId]Checkpoint
}

type PreparedProof struct {
	Number        SlotId
	Preprepare    SignedPrePrepare
	Request       string
	RequestDigest [sha256.Size]byte
	Prepares      map[NodeId]SignedPrepare
}

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
	Digest          [sha256.Size]byte
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
	PrePrepares map[SlotId]FullPrePrepare // O
	Node        NodeId                    // i
	Digest      [sha256.Size]byte
}

// Backup verifies O, multicasts prepares for every message
// in O, then enters new view.

func (v NewView) verify() bool {
	return true
}

type Ack struct {
}

type Message int
