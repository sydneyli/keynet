package main

import (
	"net"
	"time"
)

// REQUEST:
// op, timestamp, client addr (signed by client)
type ClientRequest struct {
	op        string
	timestamp Time
	client    Addr
}

// REPLY:
// viewnum, timestamp, client addr, node addr, result
// (signed by node)
type ClientReply struct {
	result     string
	viewNumber int
	timestamp  Time
	client     Addr
	node       Addr
}

// PRE-PREPARE:
// viewnum, seqnum, client message (digest)
// (signed by node)
type PrePrepare struct {
	viewNumber int
	seqNumber  int
	message    ClientRequest
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	viewNumber int
	seqNumber  int
	message    ClientRequest
	node       Addr
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	viewNumber int
	seqNumber  int
	message    ClientRequest
	node       Addr
}

type Ack struct {
	success bool
}

type Message int

func (t *Message) ClientMessage(request *ClientRequest, reply *ClientReply) {
	// TODO implement
	return nil
}

func (t *Message) PrePrepare(data *PrePrepare, reply *Ack) {
	// TODO implement
	return nil
}

func (t *Message) Prepare(data *Prepare, reply *Ack) {
	// TODO implement
	return nil
}

func (t *Message) Commit(data *Commit, reply *Ack) {
	// TODO implement
	return nil
}
