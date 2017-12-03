package pbft

import (
	"distributepki/util"
	"net"
	"net/http"
	"net/rpc"
)

type LogEntry struct {
	request    *ClientRequest
	preprepare PrePrepareFull
}

type PBFTNode struct {
	id      int
	host    string
	port    int
	primary bool
	peers   []string
	startup chan bool

	errorChannel      chan error
	requestChannel    chan *ClientRequest
	preprepareChannel chan *PrePrepareFull
	prepareChannel    chan *Prepare
	commitChannel     chan *Commit
	committedChannel  chan *string

	log                 map[SlotId]LogEntry
	mostRecent          SlotId // most recently seen view/sequence num
	mostRecentCommitted SlotId // most recently committed view/sequence num

	pendingSlots map[SlotId]Slot // slots that haven't been committed yet
}

type Slot struct {
	number   SlotId
	prepares map[int]Prepare
	commits  map[int]Commit
}

type ReadyMsg int
type ReadyResp bool

func StartNode(host NodeConfig, cluster ClusterConfig, ready chan<- *PBFTNode) {

	peers := make([]string, 0)
	for _, p := range cluster.Nodes {
		if p.Id != host.Id {
			peers = append(peers, util.GetHostname(p.Host, p.Port))
		}
	}
	initSlotId := SlotId{
		ViewNumber: 0,
		SeqNumber:  0,
	}

	node := PBFTNode{
		id:                  host.Id,
		host:                host.Host,
		port:                host.Port,
		primary:             cluster.Primary.Id == host.Id,
		peers:               peers,
		startup:             make(chan bool, len(cluster.Nodes)-1),
		errorChannel:        make(chan error),
		requestChannel:      make(chan *ClientRequest, 10), // some nice inherent rate limiting
		preprepareChannel:   make(chan *PrePrepareFull),
		prepareChannel:      make(chan *Prepare),
		commitChannel:       make(chan *Commit),
		log:                 make(map[SlotId]LogEntry),
		mostRecent:          initSlotId,
		mostRecentCommitted: initSlotId,
		pendingSlots:        make(map[SlotId]Slot),
	}
	server := rpc.NewServer()
	server.Register(&node)
	server.HandleHTTP("/pbft", "/debug/pbft")

	listener, e := net.Listen("tcp", util.GetHostname("", node.port))
	if e != nil {
		log.Fatal("Listen error:", e)
	}
	go http.Serve(listener, nil)

	if node.primary {
		for i := 0; i < len(cluster.Nodes)-1; i++ {
			<-node.startup
		}
		// // try to commit a request...
		// fakeRequest := ClientRequest{
		// 	Op:        "bingo",
		// 	Timestamp: time.Now(),
		// 	Client:    nil,
		// }
		// node.handleClientRequest(&fakeRequest)
	} else {
		node.signalReady(cluster)
	}
	go node.handleMessages()
	ready <- &node
}

func (n PBFTNode) ConfigChange(interface{}) {
	// XXX: Do nothing, cluster configuration changes not supported
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

	id := n.mostRecent
	n.mostRecent.SeqNumber += 1

	fullMessage := PrePrepareFull{
		Message: *request,
		Number:  n.mostRecent,
	}

	n.log[id] = LogEntry{
		request:    request,
		preprepare: fullMessage,
	}

	responses := make([]interface{}, len(n.peers))
	for i := 0; i < len(n.peers); i++ {
		responses[i] = new(Ack)
	}
	log.Infof("Sending PrePrepare messages for %+v", id)
	bcastRPC(n.peers, "PBFTNode.PrePrepare", &fullMessage, responses, 10)

	for i, r := range responses {
		log.Infof("PrePrepare response %d: %+v", i, r)
	}
}

// ** Remote Calls ** //
func (n PBFTNode) handleRequests() {
	requestC := n.requestChannel
	for {
		request := <-requestC
		n.handleClientRequest(request)
	}
}

func (n PBFTNode) handleMessages() {
	for {
		select {
		case msg := <-n.preprepareChannel:
			if !n.primary {
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

func (n PBFTNode) handlePrePrepare(preprepare *PrePrepareFull) {
	log.Infof("PrePrepare detected %b", preprepare)
	prepare := Prepare{
		Number:  preprepare.Number,
		Message: preprepare.Message,
		Node:    n.id,
	}
	// broadcast prepares
	responses := make([]interface{}, len(n.peers))
	for i := 0; i < len(n.peers); i++ {
		responses[i] = new(Ack)
	}
	bcastRPC(n.peers, "PBFTNode.Prepare", &prepare, responses, 10)

	for i, r := range responses {
		log.Infof("Prepare response %d: %+v", i, r)
	}
}

func (n PBFTNode) isPrepared(slot Slot) bool {
	return len(slot.prepares) > (len(n.peers)+1)*2/3
}

func (n PBFTNode) isCommitted(slot Slot) bool {
	return len(slot.commits) > (len(n.peers)+1)*2/3
}

// ensure mapping from SlotId exists in PBFTNode
func (n PBFTNode) ensureMapping(num SlotId) Slot {
	slot, ok := n.pendingSlots[num]
	if !ok {
		slot = Slot{
			number:   num,
			prepares: make(map[int]Prepare),
			commits:  make(map[int]Commit),
		}
		if n.mostRecent.Before(num) {
			n.mostRecent = num
		}
		n.pendingSlots[num] = slot
	}
	return slot
}

func (n PBFTNode) handlePrepare(message *Prepare) {
	// TODO: validate prepare message
	log.Infof("Received Prepare %b", message)
	if message.Number.Before(n.mostRecentCommitted) {
		return // ignore outdated slots
	}
	slot := n.ensureMapping(message.Number)
	slot.prepares[message.Node] = *message
	if n.isPrepared(slot) {
		log.Infof("PREPARED %+v", message.Number)
		commit := Commit{
			Number:  message.Number,
			Message: message.Message,
			Node:    n.id,
		}
		// broadcast commit
		responses := make([]interface{}, len(n.peers))
		for i := 0; i < len(n.peers); i++ {
			responses[i] = new(Ack)
		}
		bcastRPC(n.peers, "PBFTNode.Commit", &commit, responses, 10)
	}
}

func (n PBFTNode) handleCommit(message *Commit) {
	// TODO: validate commit message
	log.Infof("Received Commit %b", message)
	if message.Number.Before(n.mostRecentCommitted) {
		return // ignore outdated slots
	}
	slot := n.ensureMapping(message.Number)
	slot.commits[message.Node] = *message
	if n.isCommitted(slot) {
		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel
	}
}

func (n PBFTNode) handlePrePrepares() {
	incoming := n.preprepareChannel
	for {
		pp := <-incoming
		n.handlePrePrepare(pp)
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
	err := sendRPC(util.GetHostname(primary.Host, primary.Port), "PBFTNode.Ready", &message, new(ReadyResp), -1)
	if err != nil {
		log.Fatal(err)
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

func bcastRPC(peers []string, rpcName string, message interface{}, response []interface{}, retries int) {
	for i, p := range peers {
		err := sendRPC(p, rpcName, message, response[i], retries)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func sendRPC(hostName string, rpcName string, message interface{}, response interface{}, retries int) error {
	log.Infof("Sending rpc to %s", hostName)
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
