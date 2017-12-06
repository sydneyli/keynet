package pbft

import (
	"distributepki/util"

	"net"
	"net/http"
	"net/rpc"
	"time"
)

var machineId int

type PBFTNode struct {
	id      int
	host    string
	port    int
	primary bool
	peers   []string
	peermap map[int]string // id => hostname
	startup chan bool

	errorChannel      chan error
	debugChannel      chan *DebugMessage
	requestChannel    chan *ClientRequest
	preprepareChannel chan *PrePrepareFull
	prepareChannel    chan *Prepare
	commitChannel     chan *Commit
	committedChannel  chan *string

	log                 map[SlotId]Slot
	viewNumber          int
	sequenceNumber      int
	mostRecentCommitted SlotId // most recently committed view/sequence num
}

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

type Slot struct {
	// number     SlotId
	request    *ClientRequest
	preprepare *PrePrepareFull
	prepares   map[int]*Prepare
	commits    map[int]*Commit
}

type ReadyMsg int
type ReadyResp bool

func StartNode(host NodeConfig, cluster ClusterConfig, ready chan<- *PBFTNode) {
	peers := make([]string, 0)
	peermap := make(map[int]string)
	for _, p := range cluster.Nodes {
		if p.Id != host.Id {
			peermap[p.Id] = util.GetHostname(p.Host, p.Port)
			peers = append(peers, util.GetHostname(p.Host, p.Port))
		}
	}
	initSlotId := SlotId{
		ViewNumber: 0,
		SeqNumber:  0,
	}
	machineId = host.Id

	node := PBFTNode{
		id:                  host.Id,
		host:                host.Host,
		port:                host.Port,
		primary:             cluster.Primary.Id == host.Id,
		peers:               peers,
		peermap:             peermap,
		startup:             make(chan bool, len(cluster.Nodes)-1),
		debugChannel:        make(chan *DebugMessage),
		errorChannel:        make(chan error),
		requestChannel:      make(chan *ClientRequest, 10), // some nice inherent rate limiting
		preprepareChannel:   make(chan *PrePrepareFull),
		prepareChannel:      make(chan *Prepare),
		commitChannel:       make(chan *Commit),
		log:                 make(map[SlotId]Slot),
		viewNumber:          0,
		sequenceNumber:      0,
		mostRecentCommitted: initSlotId,
	}
	server := rpc.NewServer()
	server.Register(&node)
	server.HandleHTTP("/pbft", "/debug/pbft")

	listener, e := net.Listen("tcp", util.GetHostname("", node.port))
	if e != nil {
		node.Error("Listen error: %v", e)
	}
	go http.Serve(listener, nil)

	if node.primary {
		for i := 0; i < len(cluster.Nodes)-1; i++ {
			<-node.startup
		}
		// Try to commit a fake request
		fakeRequest := ClientRequest{
			Op:        "bingo",
			Timestamp: time.Now(),
			Client:    nil,
		}
		node.handleClientRequest(&fakeRequest)
	} else {
		node.signalReady(cluster)
	}
	go node.handleMessages()
	ready <- &node
}

func (n PBFTNode) Log(format string, args ...interface{}) {
	plog.Infof("[Node %d] "+format, n.id, args)
}

func (n PBFTNode) Error(format string, args ...interface{}) {
	plog.Fatalf("[Node %d] "+format, n.id, args)
}

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Committed() chan *string {
	return n.committedChannel
}

func (n PBFTNode) Propose(opcode int, s string) {

	if !n.primary {
		return // no proposals for non-primary nodes
	}

	request := new(ClientRequest)
	request.Opcode = opcode
	request.Op = s

	n.requestChannel <- request
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

// does appropriate actions after receivin a client request
// i.e. send out preprepares and stuff
func (n PBFTNode) handleClientRequest(request *ClientRequest) {
	if request == nil {
		return
	}

	n.sequenceNumber += 1
	id := SlotId{
		ViewNumber: n.viewNumber,
		SeqNumber:  n.sequenceNumber,
	}

	fullMessage := PrePrepareFull{
		PrePrepareMessage: PrePrepare{
			Number: id,
		},
		Request: *request,
	}

	n.log[id] = Slot{
		request:    request,
		preprepare: &fullMessage,
		prepares:   make(map[int]*Prepare),
		commits:    make(map[int]*Commit),
	}

	responses := make(map[int]interface{})
	for i, _ := range n.peermap {
		responses[i] = new(Ack)
	}
	n.Log("Sending Preprepare messages for %+v", id)
	bcastRPC(n.peermap, "PBFTNode.PrePrepare", &fullMessage, responses, 10)

	for i, r := range responses {
		n.Log("Preprepare response %d: %+v", i, r)
		bcastRPC(n.peermap, "PBFTNode.PrePrepare", &fullMessage, responses, 10)
	}
}

// ** Remote Calls ** //
func (n PBFTNode) handleMessages() {
	for {
		select {
		case msg := <-n.debugChannel:
			n.handleDebug(msg)
		case msg := <-n.preprepareChannel:
			if !n.primary { // potential view change if fails?
				n.handlePrePrepare(msg)
			}
		case msg := <-n.requestChannel:
			if n.primary {
				n.handleClientRequest(msg)
			}
		case msg := <-n.prepareChannel:
			n.handlePrepare(msg)
		case msg := <-n.commitChannel:
			n.handleCommit(msg)
		}
	}
}

// ensure mapping from SlotId exists in PBFTNode
func (n PBFTNode) ensureMapping(num SlotId) Slot {
	slot, ok := n.log[num]
	if !ok {
		slot = Slot{
			request:    nil,
			preprepare: nil,
			prepares:   make(map[int]*Prepare),
			commits:    make(map[int]*Commit),
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

func (n PBFTNode) handleDebug(debug *DebugMessage) {
	switch op := debug.Op; op {
	case SLOW:
	case PUT:
	case GET:
	case DOWN:
	case UP:
	}
}

func (n PBFTNode) handlePrePrepare(preprepare *PrePrepareFull) {
	n.Log("PrePrepare detected %b", preprepare)
	preprepareMessage := preprepare.PrePrepareMessage
	prepare := Prepare{
		Number:  preprepareMessage.Number,
		Message: preprepare.Request,
		Node:    n.id,
	}

	matchingSlot := n.ensureMapping(preprepareMessage.Number)
	if matchingSlot.preprepare != nil {
		plog.Fatalf("Received more than one pre-prepare for slot id %+v", preprepareMessage.Number)
		return
	}
	matchingSlot.request = &preprepare.Request
	matchingSlot.preprepare = preprepare
	n.log[preprepareMessage.Number] = matchingSlot

	n.Log("PREPREPARED %+v", prepare.Number)
	// broadcast prepares
	responses := make(map[int]interface{})
	for i, _ := range n.peermap {
		responses[i] = new(Ack)
	}
	bcastRPC(n.peermap, "PBFTNode.Prepare", &prepare, responses, 10)

	for i, r := range responses {
		n.Log("Prepare response %d: %+v", i, r)
	}
}

func (n PBFTNode) isPrepared(slot Slot) bool {
	// # Prepares received >= 2f = 2 * ((N - 1) / 3)
	return slot.preprepare != nil && len(slot.prepares) >= 2*(len(n.peers)/3)
}

func (n PBFTNode) isCommitted(slot Slot) bool {
	// # Commits received >= 2f + 1 = 2 * ((N - 1) / 3) + 1
	return n.isPrepared(slot) && len(slot.commits) >= 2*(len(n.peers)/3)+1
}

func (n PBFTNode) handlePrepare(message *Prepare) {
	// TODO: validate prepare message
	plog.Infof("Received Prepare %b", message)
	/*
		// TODO: come back and fix this
		if message.Number.Before(n.mostRecentCommitted) {
			return // ignore outdated slots
		}
	*/
	slot := n.ensureMapping(message.Number)
	//TODO: check that the prepare matches the pre-prepare message if it exists
	slot.prepares[message.Node] = message
	n.log[message.Number] = slot

	if n.isPrepared(slot) {
		n.Log("PREPARED %+v", message.Number)
		commit := Commit{
			Number:  message.Number,
			Message: message.Message,
			Node:    n.id,
		}
		// broadcast commit
		responses := make(map[int]interface{})
		for i, _ := range n.peermap {
			responses[i] = new(Ack)
		}
		bcastRPC(n.peermap, "PBFTNode.Commit", &commit, responses, 10)
	}
}

func (n PBFTNode) handleCommit(message *Commit) {
	// TODO: validate commit message
	n.Log("Received Commit %b", message)
	/*
		// TODO: come back and fix validation
		if message.Number.Before(n.mostRecentCommitted) {
			return // ignore outdated slots
		}
	*/
	slot := n.ensureMapping(message.Number)
	slot.commits[message.Node] = message
	n.log[message.Number] = slot

	if n.isCommitted(slot) {
		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel
		plog.Infof("NODE %d COMMITTED %+v", n.id, message.Number)
	}
}

// ** Startup ** //

func (n PBFTNode) signalReady(cluster ClusterConfig) {

	var primary NodeConfig
	for _, n := range cluster.Nodes {
		if n.Id == cluster.Primary.Id {
			primary = n
			break
		}
	}

	message := ReadyMsg(cluster.Primary.Id)
	err := sendRPC(cluster.Primary.Id, util.GetHostname(primary.Host, primary.Port), "PBFTNode.Ready", &message, new(ReadyResp), -1)
	if err != nil {
		n.Error("%v", err)
	}
}

func (n *PBFTNode) Ready(req *ReadyMsg, res *ReadyResp) error {
	*res = ReadyResp(true)
	n.startup <- true
	return nil
}

// ** Protocol **//

func (n *PBFTNode) PrePrepare(req *PrePrepareFull, res *Ack) error {
	res.Success = true
	n.preprepareChannel <- req
	return nil
}

func (n *PBFTNode) Prepare(req *Prepare, res *Ack) error {
	res.Success = true
	n.prepareChannel <- req
	return nil
}

func (n *PBFTNode) Commit(req *Commit, res *Ack) error {
	res.Success = true
	n.commitChannel <- req
	return nil
}

// ** RPC ** //
// both currently sync

func bcastRPC(peers map[int]string, rpcName string, message interface{}, response map[int]interface{}, retries int) {
	for i, p := range peers {
		err := sendRPC(i, p, rpcName, message, response[i], retries)
		if err != nil {
			plog.Fatalf("[Node %d] %v", machineId, err)
		}
	}
}

func sendRPC(peerId int, hostName string, rpcName string, message interface{}, response interface{}, retries int) error {
	plog.Infof("[Node %d] Sending rpc to %d", machineId, peerId)
	rpcClient, err := rpc.DialHTTPPath("tcp", hostName, "/pbft")
	for nRetries := 0; err != nil && retries < nRetries; nRetries++ {
		rpcClient, err = rpc.DialHTTPPath("tcp", hostName, "/pbft")
	}
	if err != nil {
		return err
	}

	remoteCall := rpcClient.Go(rpcName, message, response, nil)
	result := <-remoteCall.Done
	if result.Error != nil {
		return result.Error
	}
	return nil
}
