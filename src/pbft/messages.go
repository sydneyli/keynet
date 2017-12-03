package pbft

import (
	"net"
	"time"
)

type SlotId struct {
	ViewNumber int
	SeqNumber  int
}

func (slot SlotId) Before(other SlotId) bool {
	if slot.ViewNumber == other.ViewNumber {
		return slot.SeqNumber < other.SeqNumber
	}
	return slot.ViewNumber < other.ViewNumber
}

// REQUEST:
// op, timestamp, client addr (signed by client)
type ClientRequest struct {
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
	Number  SlotId
	Message ClientRequest
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	Number  SlotId
	Message ClientRequest
	Node    int // id
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	Number  SlotId
	Message ClientRequest
	Node    int // id
}

type Ack struct {
	Success bool
}

type Message int
