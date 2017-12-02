package pbft

import (
	"net"
	"time"
)

// REQUEST:
// op, timestamp, client addr (signed by client)
type ClientRequest struct {
	op        string
	timestamp time.Time
	client    net.Addr
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
	viewNumber int
	seqNumber  int
}

type PrePrepareFull struct {
	pp      PrePrepare
	message ClientRequest
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	viewNumber int
	seqNumber  int
	message    ClientRequest
	node       net.Addr
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	viewNumber int
	seqNumber  int
	message    ClientRequest
	node       net.Addr
}

type Ack struct {
	success bool
}

type Message int

func (t *Message) ClientMessage(request *ClientRequest, reply *ClientReply) error {
	// TODO implement
	return nil
}

func (t *Message) PrePrepare(data *PrePrepare, reply *Ack) error {
	// TODO implement
	return nil
}

func (t *Message) Prepare(data *Prepare, reply *Ack) error {
	// TODO implement
	return nil
}

func (t *Message) Commit(data *Commit, reply *Ack) error {
	// TODO implement
	return nil
}
