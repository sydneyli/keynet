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
	Digest        [sha256.Size]byte
}

type PrePrepareFull struct {
	PrePrepareMessage PrePrepare
	Request           string
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
	Node          NodeId
	Digest        [sha256.Size]byte
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

// TODO: include all proofs/verification stuff
// VIEW CHANGE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type ViewChange struct {
	ViewNumber int
	// n: last checkpoint sequence num
	// C: Proof of checkpoint at n
	// P: Proof of all requests after n
	Node   NodeId
	Digest [sha256.Size]byte
}

func (v ViewChange) verify() bool {
	return true
}

// NEW VIEW
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type NewView struct {
	ViewNumber int
	// V: set of valid view-change messages
	// O: a set of pre-prepare messages
	// TODO (sydli): I have no idea what O is
	Node   NodeId
	Digest [sha256.Size]byte
}

func (v NewView) verify() bool {
	return true
}

type Ack struct {
}

type Message int
