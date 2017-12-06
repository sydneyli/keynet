package pbft

import (
	"net"
	"time"
)

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
	PrePrepareMessage PrePrepare
	Request           ClientRequest
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
}

type Message int
