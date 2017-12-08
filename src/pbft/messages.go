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

// TODO: include all proofs/verification stuff
// VIEW CHANGE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type ViewChange struct {
	ViewNumber int
	// n: last checkpoint sequence num
	// C: Proof of checkpoint at n
	// P: Proof of all requests after n
	Node NodeId
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
	Node NodeId
}

func (v NewView) verify() bool {
	return true
}

type Ack struct {
}

type Message int
