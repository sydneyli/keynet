package pbft

import (
	"distributepki/util"

	"errors"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"
)

type PBFTNode struct {
	//////
	// Immutable config data
	//////
	id         NodeId
	host       string
	port       int
	primary    bool
	peermap    map[NodeId]string // id => hostname
	hostToPeer map[string]NodeId // hostname => id
	cluster    ClusterConfig

	// MAIN MESSAGE CHANNELS.
	// Main execution loop selects from these.
	debugChannel          chan *DebugMessage
	requestChannel        chan *ClientRequest
	preprepareChannel     chan *PrePrepareFull
	prepareChannel        chan *Prepare
	commitChannel         chan *Commit
	viewChangeChannel     chan *ViewChange
	newViewChannel        chan *NewView
	requestTimeoutChannel chan bool

	// CLIENT CHANNELS.
	// We write to these when we learn the
	// result of a client request.
	errorChannel     chan error
	committedChannel chan *Operation

	// local key grabbing hack
	KeyRequest chan *KeyRequest // hostname

	//////
	// The below are all mutable, but writes should ALWAYS
	// happen on the main routine unless otherwise indicated.
	//////

	// LEDGER (and where we are in it)
	log                  map[SlotId]Slot
	viewNumber           int
	sequenceNumber       int
	issuedSequenceNumber int

	// Used to track execution of client requests
	requests map[int64]requestInfo

	// VIEW CHANGE STATE. We are in the middle of a viewchange
	// if viewChange.inProgress.
	viewChange *viewChangeInfo

	// CHECKPOINT STATE.
	lastCheckpoint  SlotId
	checkpointProof map[NodeId]Checkpoint

	// TIMEOUTS. The heartbeat ticker allows the primary to
	// continually send timeouts; replicas use the timeout
	// timer to determine if the leader has been active.
	heartbeatTicker *time.Ticker
	timeoutTimer    *time.Timer

	// LEADER STATE (to catch up stragglers)
	// if some nodes aren't caught up, newView is broadcasted
	// to them instead of regular PrePrepare heartbeats.
	// note: caughtUp is written to in a goroutine, so we lock it.
	caughtUp    map[NodeId]bool //are all my peers caught up?
	caughtUpMux sync.RWMutex
	newView     *NewView // view message to continually broadcast

	// Debug states
	down bool
	slow bool
}

// Information associated with current view change.
type viewChangeInfo struct {
	messages   map[NodeId]ViewChange
	inProgress bool
	viewNumber int
}

type KeyRequest struct {
	Hostname string
	Reply    chan *string
}

// Heartbeat ticker
const TIMEOUT time.Duration = time.Duration(time.Second)

// How many sequence numbers to wait before checkpointing
const CHECKPOINT uint = 100

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
	// 2. Create the node
	node := PBFTNode{
		id:                    host.Id,
		host:                  host.Host,
		port:                  host.Port,
		peermap:               peermap,
		hostToPeer:            hostToPeer,
		cluster:               cluster,
		debugChannel:          make(chan *DebugMessage),
		committedChannel:      make(chan *Operation),
		errorChannel:          make(chan error),
		requestChannel:        make(chan *ClientRequest, 10), // some nice inherent rate limiting
		preprepareChannel:     make(chan *PrePrepareFull),
		prepareChannel:        make(chan *Prepare),
		commitChannel:         make(chan *Commit),
		viewChangeChannel:     make(chan *ViewChange),
		newViewChannel:        make(chan *NewView),
		requestTimeoutChannel: make(chan bool),
		requests:              make(map[int64]requestInfo),
		log:                   make(map[SlotId]Slot),
		viewNumber:            0,
		sequenceNumber:        0,
		issuedSequenceNumber:  0,
		viewChange:            &viewChangeInfo{inProgress: false, viewNumber: 0, messages: make(map[NodeId]ViewChange)},
		lastCheckpoint:        SlotId{ViewNumber: 0, SeqNumber: 0},
		checkpointProof:       make(map[NodeId]Checkpoint),
		heartbeatTicker:       nil,
		timeoutTimer:          nil,
		caughtUp:              make(map[NodeId]bool),
		newView:               &NewView{ViewNumber: 0, Node: host.Id},
		down:                  false,
		slow:                  false,
	}
	for p, _ := range node.peermap {
		node.caughtUp[p] = true
	}
	// 3. Start RPC server
	server := rpc.NewServer()
	server.Register(&node)
	node.Log(cluster.Endpoint)
	server.HandleHTTP(cluster.Endpoint, "/debug/"+cluster.Endpoint)
	node.Log("Listening on %v", cluster.Endpoint)
	listener, e := net.Listen("tcp", util.GetHostname("", node.port))
	if e != nil {
		node.Error("Listen error: %v", e)
		return nil
	}
	if node.isPrimary() {
		node.heartbeatTicker = time.NewTicker(node.getTimeout())
	} else {
		node.timeoutTimer = time.NewTimer(node.getTimeout())
	}
	go http.Serve(listener, nil)
	// 4. Start exec loop
	go node.handleMessages()
	return &node
}

// ** HELPERS ** //

// Helper functions for logging! (prepends node id to logs) //
func (n PBFTNode) Log(format string, args ...interface{}) {
	args = append([]interface{}{n.id}, args...)
	plog.Infof("[Node %d] "+format, args...)
}

func (n PBFTNode) Error(format string, args ...interface{}) {
	args = append([]interface{}{n.id}, args...)
	plog.Fatalf("[Node %d] "+format, args...)
}

// ensure mapping from SlotId exists in PBFTNode
func (n *PBFTNode) ensureMapping(num SlotId) Slot {
	slot, ok := n.log[num]
	if !ok {
		slot = Slot{
			request:    nil,
			preprepare: nil,
			prepares:   make(map[NodeId]Prepare),
			commits:    make(map[NodeId]*Commit),
			prepared:   false,
			committed:  false,
		}
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
	return n.cluster.LeaderFor(n.viewNumber) == n.id
}

func (n PBFTNode) getPrimary() (NodeId, string) {
	primaryId := n.cluster.LeaderFor(n.viewNumber)
	for i, p := range n.peermap {
		if i == primaryId {
			return i, p
		}
	}
	return NodeId(0), ""
}

func (n PBFTNode) isPrepared(slot Slot) bool {
	// # Prepares received >= 2f = 2 * ((N - 1) / 3)
	return slot.preprepare != nil && len(slot.prepares) >= 2*(len(n.peermap)/3)
}

func (n PBFTNode) isCommitted(slot Slot) bool {
	// # Commits received >= 2f + 1 = 2 * ((N - 1) / 3) + 1
	return n.isPrepared(slot) && len(slot.commits) >= 2*(len(n.peermap)/3)+1
}

// ** ALL THE MESSAGE HANDLERS ** //

// MAIN EXECUTION LOOP
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
		case <-n.requestTimeoutChannel: // one of my client requests timed out!
			n.startViewChange(n.viewNumber + 1)
		case <-n.getTimer(): // timer expired
			n.handleHeartbeatTimeout()
		case msg := <-n.viewChangeChannel:
			n.handleViewChange(msg)
		case msg := <-n.newViewChannel:
			n.handleNewView(msg)
		}
	}
}

// does appropriate actions after receivin a client request
// i.e. send out preprepares and stuff
func (n *PBFTNode) handleClientRequest(request *ClientRequest) {
	if n.down {
		return
	}
	if n.viewChange.inProgress {
		return
	}
	// don't re-process already processed requests
	if _, ok := n.requests[request.Id]; ok {
		return // return with answer?
	}
	n.requests[request.Id] = requestInfo{
		id:        request.Id,
		committed: false,
		request:   request,
	}
	if n.isPrimary() {
		if request == nil {
			return
		}
		n.issuedSequenceNumber = n.issuedSequenceNumber + 1
		n.Log("ISSUED SEQNUM %d for %d", n.issuedSequenceNumber, request.Id)
		id := SlotId{
			ViewNumber: n.viewNumber,
			SeqNumber:  n.issuedSequenceNumber,
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
			prepares:   make(map[NodeId]Prepare),
			commits:    make(map[NodeId]*Commit),
			prepared:   false,
			committed:  false,
		}
		go n.broadcast("PBFTNode.PrePrepare", &fullMessage, 0)
	} else {
		// forward to all ma frandz if im not da leader
		// TODO: they should probably just forward it to the leader
		go n.broadcast("PBFTNode.ClientRequest", request, 0)

		// start execution timer...
		go func(n *PBFTNode) {
			<-time.After(TIMEOUT)
			if !n.requests[request.Id].committed {
				n.requestTimeoutChannel <- true
			}
		}(n)
	}
}

func (n *PBFTNode) handlePrePrepare(preprepare *PrePrepareFull) {
	if n.down {
		return
	}
	if n.viewChange.inProgress {
		return
	}
	// A backup accepts a pre-prepare message provided:
	// -  the signatures in the request and the pre-prepare message are
	//    correct and d is the digest for message m
	// -  it is in view v
	// -  it has not accepted a pre-prepare message for view v and seq
	//    num n containing a different digest
	// -  the sequence number in the preprepare message is between the
	//    low water-mark h and high water-mark H
	// TODO: Drop invalid Preprepares...
	// verification should include:
	//     1. only the leader of the specified view can send preprepares
	preprepareMessage := preprepare.PrePrepareMessage
	n.timeoutTimer.Reset(n.getTimeout())
	if preprepareMessage == (PrePrepare{}) {
		//NO-OP heartbeat... don't bother processing
		return
	}
	newSeqNum := preprepare.PrePrepareMessage.Number.SeqNumber
	if newSeqNum > n.issuedSequenceNumber {
		n.issuedSequenceNumber = newSeqNum
	}
	prepare := Prepare{
		Number:  preprepareMessage.Number,
		Message: preprepare.Request,
		Node:    n.id,
	}
	matchingSlot := n.ensureMapping(preprepareMessage.Number)
	if matchingSlot.preprepare != nil {
		//TODO: Only throw here if digest is different
		n.Error("Received more than one pre-prepare for slot id %+v", preprepareMessage.Number)
		return
	}
	matchingSlot.request = &preprepare.Request
	matchingSlot.preprepare = preprepare
	n.log[preprepareMessage.Number] = matchingSlot
	go n.broadcast("PBFTNode.Prepare", &prepare, 0)
}

func (n *PBFTNode) handlePrepare(message *Prepare) {
	if n.viewChange.inProgress {
		return
	}
	if n.down {
		return
	}
	// If we've already committed this message... don't bother
	if message.Number.BeforeOrEqual(SlotId{
		ViewNumber: n.viewNumber, SeqNumber: n.sequenceNumber}) {
		return
	}
	// TODO: validate prepare message
	slot := n.ensureMapping(message.Number)
	//TODO: check that the prepare matches the pre-prepare message if it exists
	slot.prepares[message.Node] = *message
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
		go n.broadcast("PBFTNode.Commit", &commit, 0)
	}
}

func (n *PBFTNode) handleCommit(message *Commit) {
	if n.down {
		return
	}
	if n.viewChange.inProgress {
		return
	}
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
		info := n.requests[message.Message.Id] //.committed = true
		n.requests[message.Message.Id] = requestInfo{
			id:        info.id,
			committed: true,
			request:   info.request,
		}
		if message.Number.SeqNumber > n.sequenceNumber {
			n.sequenceNumber = message.Number.SeqNumber
		}
	}
	n.log[message.Number] = slot
	if nowCommitted {
		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel
		n.Log("COMMITTED %+v", message.Number)
		n.Committed() <- &Operation{Opcode: message.Message.Opcode, Op: message.Message.Op}
	}
}

type requestView struct {
	request ClientRequest
	view    int
}

// Section 4.4 in paper
// O is computed as follows:
// 1. The primary determines the sequence number min-s of the
//    last stable checkpoint in V and the highest sequence
//    number max-s in a prepare message in V.
// 2. The primary creates a new pre-prepare message for view
//    v+1 for each sequence number n between min-s and max-s.
//    There are two cases:
//    1. There is at least one set in the P component of some
//       view-change message in V with sequence number n.
//       Primary creates Pre-prepare with the request digest
//       in the pre-prepare message for n with the highest
//       view number in V.
//    2. There is no set in P of any view-change message in V
//       with sequence number n.
//       Primary creates Pre-prepare with a no-op message.
// Then the primary appends the messages in O to its log.
// Then enters new view.
func (n *PBFTNode) generatePrepreparesForNewView(view int) map[SlotId]PrePrepareFull {
	// 1. The primary determines the sequence number min-s of the
	//    latest stable checkpoint in V and the highest sequence
	//    number max-s in a prepare message in V.
	minS := n.lastCheckpoint.SeqNumber
	maxS := n.lastCheckpoint.SeqNumber
	seqNums := make(map[int]requestView)
	for _, viewChange := range n.viewChange.messages {
		// 1a. Get the latest stable checkpoint in V
		if viewChange.Checkpoint.SeqNumber > minS {
			minS = viewChange.Checkpoint.SeqNumber
		}
		for num, prepareproof := range viewChange.Proofs {
			// 1b. Get the highest sequence number in any prepare message.
			//     We also want to record the ClientRequest messages
			//     for all the sequence numbers (we choose the ones with
			//     the highest view numbers), for part (2).
			reqInfo := requestView{
				view:    num.ViewNumber,
				request: prepareproof.Preprepare.Request,
			}
			if prevReqInfo, ok := seqNums[num.SeqNumber]; ok {
				// if it exists, only replace if this one has a higher viewnum
				if reqInfo.view > prevReqInfo.view {
					seqNums[num.SeqNumber] = reqInfo
				}
			} else {
				// if it doesn't exist already
				seqNums[num.SeqNumber] = reqInfo
			}
			if maxS < num.SeqNumber {
				maxS = num.SeqNumber
			}
		}
	}
	n.Log("sending prepares for messages between %d and %d", minS, maxS)
	// 2. The primary creates a new pre-prepare message for view
	//    v+1 for each sequence number n between min-s and max-s.
	preprepares := make(map[SlotId]PrePrepareFull)
	for s := minS; s <= maxS; s++ {
		slotId := SlotId{
			ViewNumber: view,
			SeqNumber:  s,
		}
		var request ClientRequest
		//    There are two cases:
		if reqInfo, ok := seqNums[s]; ok {
			//    1. There is at least one set in the P component of some
			//       view-change message in V with sequence number n.
			//       Primary creates Pre-prepare with the request digest
			//       in the pre-prepare message for n with the highest
			//       view number in V.
			request = reqInfo.request
		} else {
			//    2. There is no such set.
			//       Primary creates Pre-prepare with a no-op message.
			request = ClientRequest{}
		}
		preprepares[slotId] = PrePrepareFull{
			PrePrepareMessage: PrePrepare{Number: slotId},
			Request:           request,
		}
	}
	return preprepares
}

func (n *PBFTNode) handleViewChange(message *ViewChange) {
	if n.down {
		return
	}
	// Paper: section 4.5.2
	// if a replica receives a set of f+1 valid view change messages
	// from other replicas for views higher than its current view,
	// it sends a view-change message for the next smallest view in
	// the set, even if its timer has not expired.
	var currentView int
	if n.viewChange.inProgress {
		currentView = n.viewChange.viewNumber
	} else {
		currentView = n.viewNumber
	}
	// 0. Update most recently seen viewchange message from this node
	n.viewChange.messages[message.Node] = *message
	// 1. If a replica receives a set of f+1 valid view change messages
	// from other replicas for views higher than its current view...
	if message.ViewNumber > currentView {
		higherThanCurrent := 0
		lowestNewView := message.ViewNumber
		for _, msg := range n.viewChange.messages {
			if msg.ViewNumber > currentView {
				higherThanCurrent += 1
				if msg.ViewNumber < lowestNewView {
					lowestNewView = msg.ViewNumber
				}
			}
		}
		if higherThanCurrent >= (len(n.peermap)/3)+1 {
			// 2. Broadcast view-change messages for the next smallest
			//    view in that set.
			n.startViewChange(lowestNewView)
			return
		}
	}
	// Paper: 4.4
	// When the primary p of view v + 1 receives 2f valid view-change messages
	// for view v + 1, it multicasts NEW-VIEW.
	// < And then it does a bunch of stuff I haven't read through yet >
	// Then it /enters/ view v+1: at this point it is able to accept messages for
	// view v + 1.
	// 0. If no new view change was started, and I'm the leader of this view change
	if n.viewChange.inProgress && n.cluster.LeaderFor(message.ViewNumber) == n.id {
		// 1. See if we got 2f view-change messages for this view!
		votes := 0
		for _, msg := range n.viewChange.messages {
			if msg.ViewNumber == message.ViewNumber {
				votes += 1
			}
		}
		// 2. If so, multicast new-view (heartbeat)
		if votes >= (2 * len(n.peermap) / 3) {
			newview := NewView{
				ViewNumber:  message.ViewNumber,
				ViewChanges: n.viewChange.messages,
				PrePrepares: n.generatePrepreparesForNewView(message.ViewNumber),
				Node:        n.id,
			}
			n.newView = &newview
			n.caughtUpMux.Lock()
			for p, _ := range n.peermap {
				n.caughtUp[p] = false
			}
			n.caughtUpMux.Unlock()
			n.enterNewView(message.ViewNumber)
			n.sendHeartbeat()
		}
	}
}

func (n *PBFTNode) handleNewView(message *NewView) {
	if n.down {
		return
	}
	// Paper: section 4.4
	// A backup accepts a new-view message for view v+1 if it is signed
	// properly, if the view-change messages it contains are valid for view v+1,
	// and *if the set O is correct* (TODO). It multicasts prepares for each
	// message in O, and enters view + 1
	var currentView int
	if n.viewChange.inProgress {
		if message.ViewNumber == n.viewChange.viewNumber {
			n.enterNewView(message.ViewNumber)
			return
		}
		currentView = n.viewChange.viewNumber
	} else {
		currentView = n.viewNumber
	}
	// TODO: validate everything
	if message.ViewNumber > currentView {
		n.enterNewView(message.ViewNumber)
	}
}

// ** TIMER/HEARTBEAT STUFF ** //

func (n *PBFTNode) getTimeout() time.Duration {
	// When primary times out, it send a heartbeat
	if n.isPrimary() {
		return TIMEOUT
	} else {
		// When non-primaries timeout with no heartbeat,
		// they start view change
		return TIMEOUT * time.Duration(len(n.peermap))
	}
}

func (n *PBFTNode) handleHeartbeatTimeout() {
	if n.down || n.viewChange.inProgress {
		return
	}
	if !n.isPrimary() {
		n.startViewChange(n.viewNumber + 1)
	} else {
		n.sendHeartbeat()
	}
}

func heartbeatMessage() PrePrepareFull {
	return PrePrepareFull{
		PrePrepareMessage: PrePrepare{},
		Request:           ClientRequest{},
	}
}

// TODO (sydli): Clean up the below. It's gross.
func (n *PBFTNode) sendHeartbeat() {
	if !n.isPrimary() {
		return
	}
	for id, hostname := range n.peermap {
		var rpcType string
		var msg interface{}
		var after func(NodeId, error)
		n.caughtUpMux.RLock()
		caughtUp := n.caughtUp[id]
		n.caughtUpMux.RUnlock()
		if !caughtUp {
			rpcType = "PBFTNode.NewView"
			msg = n.newView
			after = func(id NodeId, err error) {
				n.caughtUpMux.Lock()
				n.caughtUp[id] = err == nil
				n.caughtUpMux.Unlock()
			}
		} else {
			rpcType = "PBFTNode.PrePrepare"
			msg = heartbeatMessage()
			after = func(id NodeId, err error) {}
		}
		go func(id NodeId, hostname string, rpcType string, msg interface{}, after func(NodeId, error)) {
			err := sendRpc(n.id, id, hostname, rpcType, n.cluster.Endpoint, msg, nil, 1, time.Duration(100*time.Millisecond))
			after(id, err)
		}(id, hostname, rpcType, msg, after)
	}
}

func (n *PBFTNode) getTimer() <-chan time.Time {
	if n.isPrimary() {
		return n.heartbeatTicker.C
	} else {
		return n.timeoutTimer.C
	}
}

func (n *PBFTNode) stopTimers() {
	// TODO: properly close these guys (flush channels):
	// https://github.com/golang/go/issues/14383
	if n.timeoutTimer != nil {
		n.timeoutTimer.Stop()
	}
	if n.heartbeatTicker != nil {
		n.heartbeatTicker.Stop()
	}
}

func (n *PBFTNode) startTimers() {
	if n.isPrimary() {
		n.heartbeatTicker = time.NewTicker(n.getTimeout())
	} else {
		n.timeoutTimer = time.NewTimer(n.getTimeout())
	}
}

// ** VIEW CHANGES ** //

func (n *PBFTNode) generateProofsSinceCheckpoint() map[SlotId]PreparedProof {
	proofs := make(map[SlotId]PreparedProof)
	for id, slot := range n.log {
		if n.lastCheckpoint.Before(id) && n.isPrepared(slot) {
			proofs[id] = PreparedProof{
				Number:     id,
				Preprepare: *slot.preprepare,
				Prepares:   slot.prepares,
			}
		}
	}
	return proofs
}

// Section 4.4
// If the timer of backup i expires in view v, it starts
// a view change to move the system to view v + 1.
// It stops accepting messages (other than checkpoint,
// view-change, and new-view) and multicasts a view-change
// to all other replicas.
func (n *PBFTNode) startViewChange(view int) {
	if n.viewChange.viewNumber > view || n.viewChange.inProgress && n.viewChange.viewNumber == view {
		return
	}
	n.Log("START VIEW CHANGE FOR VIEW %d", view)
	n.viewChange.inProgress = true
	n.viewChange.viewNumber = view
	message := ViewChange{
		ViewNumber:      view,
		Checkpoint:      n.lastCheckpoint,
		CheckpointProof: n.checkpointProof,
		Proofs:          n.generateProofsSinceCheckpoint(),
		Node:            n.id,
	}
	// TODO (sydli): instead of stopping this timer, use it for exponential backoff
	n.stopTimers()
	go broadcast(n.id, n.peermap, "PBFTNode.ViewChange", n.cluster.Endpoint, &message, time.Duration(100*time.Millisecond))
}

func (n *PBFTNode) enterNewView(view int) {
	n.Log("ENTER NEW VIEW FOR VIEW %d", view)
	n.viewChange.inProgress = false
	n.viewNumber = view
	n.startTimers()
}

// ** Protocol **//

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Committed() chan *Operation {
	return n.committedChannel
}

func (n *PBFTNode) Propose(operation Operation) {
	request := new(ClientRequest)
	request.Opcode = operation.Opcode
	request.Timestamp = operation.Timestamp
	request.Id = operation.Timestamp.UnixNano()
	request.Op = operation.Op
	n.requestChannel <- request
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

func (n PBFTNode) ClientRequest(req *ClientRequest, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.requestChannel <- req
	return nil
}

func (n PBFTNode) PrePrepare(req *PrePrepareFull, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.preprepareChannel <- req
	return nil
}

func (n PBFTNode) Prepare(req *Prepare, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.prepareChannel <- req
	return nil
}

func (n PBFTNode) Commit(req *Commit, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.commitChannel <- req
	return nil
}

func (n PBFTNode) ViewChange(req *ViewChange, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.viewChangeChannel <- req
	return nil
}

func (n PBFTNode) NewView(req *NewView, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.newViewChannel <- req
	return nil
}

func (n PBFTNode) broadcast(rpcName string, message interface{}, timeout time.Duration) {
	broadcast(n.id, n.peermap, rpcName, n.cluster.Endpoint, message, timeout)
}

// ** RPC helpers ** //
func broadcast(fromId NodeId, peers map[NodeId]string, rpcName string, endpoint string, message interface{}, timeout time.Duration) {
	for i, p := range peers {
		go func(i NodeId, hostname string) {
			err := sendRpc(fromId, i, hostname, rpcName, endpoint, message, nil, 10, 0)
			if err != nil {
				plog.Info(err)
			}
		}(i, p)
	}
}

func sendRpc(fromId NodeId, peerId NodeId, hostName string, rpcName string, endpoint string, message interface{}, response interface{}, retries int, timeout time.Duration) error {
	plog.Infof("[Node %d] Sending RPC (%s) to Node %d", fromId, rpcName, peerId)
	return util.SendRpc(hostName, endpoint, rpcName, message, response, retries, 0)
}
