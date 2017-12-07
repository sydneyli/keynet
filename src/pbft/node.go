package pbft

import (
	"distributepki/util"

	"errors"
	"net"
	"net/http"
	"net/rpc"
)

type PBFTNode struct {
	id         NodeId
	host       string
	port       int
	primary    bool
	peermap    map[NodeId]string // id => hostname
	hostToPeer map[string]NodeId // hostname => id
	startup    chan bool
	cluster    ClusterConfig

	// node channels
	debugChannel      chan *DebugMessage
	requestChannel    chan *ClientRequest
	preprepareChannel chan *PrePrepareFull
	prepareChannel    chan *Prepare
	commitChannel     chan *Commit

	// client library channels
	errorChannel     chan error
	committedChannel chan *Operation

	// local
	KeyRequest chan *KeyRequest // hostname

	// TODO: handle client request timeouts with the below
	requests            map[int64]ClientRequest
	log                 map[SlotId]Slot
	viewNumber          int
	sequenceNumber      int
	mostRecentCommitted SlotId // most recently committed view/sequence num
}

type KeyRequest struct {
	Hostname string
	Reply    chan *string
}

// helpers

// ensure mapping from SlotId exists in PBFTNode
func (n *PBFTNode) ensureMapping(num SlotId) Slot {
	slot, ok := n.log[num]
	if !ok {
		slot = Slot{
			request:    nil,
			preprepare: nil,
			prepares:   make(map[NodeId]*Prepare),
			commits:    make(map[NodeId]*Commit),
			prepared:   false,
			committed:  false,
		}
		/*
			// TODO: necessary?
			if n.mostRecent.Before(num) {
				n.mostRecent = num
			}
		*/
		n.log[num] = slot
	}
	return slot
}

func (n PBFTNode) keyFor(hostname string) (*string, error) {
	request := KeyRequest{
		Hostname: hostname,
		Reply:    make(chan *string),
	}
	n.KeyRequest <- &request
	if response := <-request.Reply; response != nil {
		return response, nil
	}
	return nil, errors.New("eft, no key")
}

func (n PBFTNode) isPrimary() bool {
	return n.cluster.LeaderFor(n.sequenceNumber) == n.id
}

func (n PBFTNode) isPrepared(slot Slot) bool {
	// # Prepares received >= 2f = 2 * ((N - 1) / 3)
	return slot.preprepare != nil && len(slot.prepares) >= 2*(len(n.peermap)/3)
}

func (n PBFTNode) isCommitted(slot Slot) bool {
	// # Commits received >= 2f + 1 = 2 * ((N - 1) / 3) + 1
	return n.isPrepared(slot) && len(slot.commits) >= 2*(len(n.peermap)/3)+1
}

// Entry point for each PBFT node.
// NodeConfig: configuration for this node
// ClusterConfig: configuration for entire cluster
// ready channel: send item when node is up & running
func StartNode(host NodeConfig, cluster ClusterConfig) *PBFTNode {
	// 1. Create id <=> peer hostname maps from list
	peermap := make(map[NodeId]string)
	hostToPeer := make(map[string]NodeId)
	for _, p := range cluster.Nodes {
		if p.Id != host.Id {
			hostname := util.GetHostname(p.Host, p.Port)
			peermap[p.Id] = hostname
			hostToPeer[hostname] = p.Id
		}
	}
	initSlotId := SlotId{
		ViewNumber: 0,
		SeqNumber:  0,
	}
	// 2. Create the node
	node := PBFTNode{
		id:                  host.Id,
		host:                host.Host,
		port:                host.Port,
		peermap:             peermap,
		hostToPeer:          hostToPeer,
		cluster:             cluster,
		debugChannel:        make(chan *DebugMessage),
		committedChannel:    make(chan *Operation),
		errorChannel:        make(chan error),
		requestChannel:      make(chan *ClientRequest, 10), // some nice inherent rate limiting
		preprepareChannel:   make(chan *PrePrepareFull),
		prepareChannel:      make(chan *Prepare),
		commitChannel:       make(chan *Commit),
		requests:            make(map[int64]ClientRequest),
		log:                 make(map[SlotId]Slot),
		viewNumber:          0,
		sequenceNumber:      0,
		mostRecentCommitted: initSlotId,
	}
	// 3. Start RPC server
	server := rpc.NewServer()
	server.Register(&node)
	server.HandleHTTP(cluster.Endpoint, "/debug/"+cluster.Endpoint)
	node.Log("Listening on %v", cluster.Endpoint)
	listener, e := net.Listen("tcp", util.GetHostname("", node.port))
	if e != nil {
		node.Error("Listen error: %v", e)
		return nil
	}
	go http.Serve(listener, nil)
	// 4. Start exec loop
	go node.handleMessages()
	return &node
}

// Helper functions for logging! (prepends node id to logs)

func (n PBFTNode) Log(format string, args ...interface{}) {
	args = append([]interface{}{n.id}, args...)
	plog.Infof("[Node %d] "+format, args...)
}

func (n PBFTNode) Error(format string, args ...interface{}) {
	args = append([]interface{}{n.id}, args...)
	plog.Fatalf("[Node %d] "+format, args...)
}

// Message handlers!

func (n *PBFTNode) handleMessages() {
	for {
		select {
		case msg := <-n.debugChannel:
			n.handleDebug(msg)
		case msg := <-n.preprepareChannel:
			n.handlePrePrepare(msg)
		case msg := <-n.requestChannel:
			n.handleClientRequest(msg)
		case msg := <-n.prepareChannel:
			n.handlePrepare(msg)
		case msg := <-n.commitChannel:
			n.handleCommit(msg)
		}
	}
}

// does appropriate actions after receivin a client request
// i.e. send out preprepares and stuff
func (n *PBFTNode) handleClientRequest(request *ClientRequest) {
	// don't re-process already processed requests
	if _, ok := n.requests[request.Id]; ok {
		return // return with answer?
	}
	if n.isPrimary() {
		n.Log("I'm the leader!")
		if request == nil {
			return
		}
		n.sequenceNumber = n.sequenceNumber + 1
		id := SlotId{
			ViewNumber: n.viewNumber,
			SeqNumber:  n.sequenceNumber,
		}
		n.Log("Received new request (%+v) - View Number: %d, Sequence Number: %d", *request, n.viewNumber, n.sequenceNumber)
		fullMessage := PrePrepareFull{
			PrePrepareMessage: PrePrepare{
				Number: id,
			},
			Request: *request,
		}
		n.log[id] = Slot{
			request:    request,
			preprepare: &fullMessage,
			prepares:   make(map[NodeId]*Prepare),
			commits:    make(map[NodeId]*Commit),
			prepared:   false,
			committed:  false,
		}
		go n.broadcast("PBFTNode.PrePrepare", &fullMessage)
	} else {
		// forward to all ma frandz
		go n.broadcast("PBFTNode.ClientRequest", request)
	}
	n.requests[request.Id] = *request
}

// TODO: verification should include:
//     1. only the leader of the specified view can send preprepares
func (n *PBFTNode) handlePrePrepare(preprepare *PrePrepareFull) {
	preprepareMessage := preprepare.PrePrepareMessage
	prepare := Prepare{
		Number:  preprepareMessage.Number,
		Message: preprepare.Request,
		Node:    n.id,
	}
	matchingSlot := n.ensureMapping(preprepareMessage.Number)
	if matchingSlot.preprepare != nil {
		//TODO: Only throw here if digest is different
		plog.Fatalf("Received more than one pre-prepare for slot id %+v", preprepareMessage.Number)
		return
	}
	matchingSlot.request = &preprepare.Request
	matchingSlot.preprepare = preprepare
	n.log[preprepareMessage.Number] = matchingSlot
	go n.broadcast("PBFTNode.Prepare", &prepare)
}

func (n *PBFTNode) handlePrepare(message *Prepare) {
	// TODO: validate prepare message
	/*
		// TODO: come back and fix this
		if message.Number.Before(n.mostRecentCommitted) {
			return // ignore outdated slots
		}
	*/
	slot := n.ensureMapping(message.Number)
	//TODO: check that the prepare matches the pre-prepare message if it exists
	slot.prepares[message.Node] = message
	nowPrepared := !slot.prepared && n.isPrepared(slot)
	if nowPrepared {
		slot.prepared = true
	}
	n.log[message.Number] = slot
	if nowPrepared {
		n.Log("PREPARED %+v", message.Number)
		commit := Commit{
			Number:  message.Number,
			Message: message.Message,
			Node:    n.id,
		}
		go n.broadcast("PBFTNode.Commit", &commit)
	}
}

func (n *PBFTNode) handleCommit(message *Commit) {
	// TODO: validate commit message
	/*
		// TODO: come back and fix validation
		if message.Number.Before(n.mostRecentCommitted) {
			return // ignore outdated slots
		}
	*/
	slot := n.ensureMapping(message.Number)
	slot.commits[message.Node] = message
	nowCommitted := !slot.committed && n.isCommitted(slot)
	if nowCommitted {
		slot.committed = true
	}
	n.log[message.Number] = slot
	if nowCommitted {
		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel
		n.Log("COMMITTED %+v", message.Number)
		n.Committed() <- &Operation{Opcode: message.Message.Opcode, Op: message.Message.Op}
	}
}

// ** VIEW CHANGES ** //
// "view changes are triggered by timeouts that prevent backups from waiting indefinitely for requests to execute."
// "a backup is *waiting* for a request if it received a valid request and has not executed it."
//   backup starts a timer when it receives a new clientrequest. restarts if it's waiting to execute another 1
// VIEW CHANGE:
//   stops accepting messages except for NEW-VIEW and VIEW-CHANGE
//   multi-casts VIEW-CHANGE for view + 1, with proof of prepared messages
//     wait for 2f+1 VIEW-CHANGE messages
//     then start timer T. If primary does not broadcast NEW-VIEW by then, do a new VIEW-CHANGE
//	   but this time set timer to 2T
//   when primary for view + 1 gets 2F VIEW-CHANGE messages, multicast NEW-VIEW

func (n *PBFTNode) startViewChange() {
}

// ** Protocol **//

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Committed() chan *Operation {
	return n.committedChannel
}

func (n PBFTNode) Propose(operation Operation) {
	if !n.isPrimary() {
		// TODO: Relay request to primary!
		return // no proposals for non-primary nodes
	}
	request := new(ClientRequest)
	// TODO: put Operation type in request (does this require custome serialization stuff)
	request.Opcode = operation.Opcode
	request.Op = operation.Op
	n.requestChannel <- request
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

func (n PBFTNode) ClientRequest(req *ClientRequest, res *Ack) error {
	n.requestChannel <- req
	return nil
}

func (n PBFTNode) PrePrepare(req *PrePrepareFull, res *Ack) error {
	n.preprepareChannel <- req
	return nil
}

func (n PBFTNode) Prepare(req *Prepare, res *Ack) error {
	n.prepareChannel <- req
	return nil
}

func (n PBFTNode) Commit(req *Commit, res *Ack) error {
	n.commitChannel <- req
	return nil
}

func (n PBFTNode) broadcast(rpcName string, message interface{}) {
	broadcast(n.id, n.peermap, rpcName, n.cluster.Endpoint, message)
}

// ** RPC helpers ** //
func broadcast(fromId NodeId, peers map[NodeId]string, rpcName string, endpoint string, message interface{}) {
	for i, p := range peers {
		err := sendRpc(fromId, i, p, rpcName, endpoint, message, nil, 10)
		if err != nil {
			plog.Fatal(err)
		}
	}
}

func sendRpc(fromId NodeId, peerId NodeId, hostName string, rpcName string, endpoint string, message interface{}, response interface{}, retries int) error {
	plog.Infof("[Node %d] Sending RPC (%s) to Node %d", fromId, rpcName, peerId)
	return util.SendRpc(hostName, endpoint, rpcName, message, response, retries, 0)
}
