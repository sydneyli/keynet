package pbft

import (
	"distributepki/util"

	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"net/rpc"
	"time"
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
	requestChannel    chan *string
	preprepareChannel chan *PrePrepareFull
	prepareChannel    chan *Prepare
	commitChannel     chan *Commit
	viewChangeChannel chan *ViewChange
	newViewChannel    chan *NewView
	timeoutChannel    chan bool

	// client library channels
	errorChannel     chan error
	committedChannel chan *string

	// local
	KeyRequest chan *KeyRequest // hostname

	log                map[SlotId]*Slot
	viewNumber         int
	sequenceNumber     int
	viewChange         *viewChangeInfo
	viewChangeMessages map[NodeId]*ViewChange
}

type viewChangeInfo struct {
	inProgress bool
	viewNumber int
}

type KeyRequest struct {
	Hostname string
	Reply    chan *string
}

// helpers

// ensure mapping from SlotId exists in PBFTNode
func (n *PBFTNode) ensureMapping(num SlotId) *Slot {
	slot, ok := n.log[num]
	if !ok {
		slot = &Slot{
			request:       nil,
			requestDigest: [sha256.Size]byte{},
			preprepare:    nil,
			prepares:      make(map[NodeId]*Prepare),
			commits:       make(map[NodeId]*Commit),
			prepared:      false,
			committed:     false,
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

func (n PBFTNode) isPrepared(slot *Slot) bool {
	// # Prepares received >= 2f = 2 * ((N - 1) / 3)
	return slot.preprepare != nil && len(slot.prepares) >= 2*(len(n.peermap)/3)
}

func (n PBFTNode) isCommitted(slot *Slot) bool {
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
	// 2. Create the node
	node := PBFTNode{
		id:                 host.Id,
		host:               host.Host,
		port:               host.Port,
		peermap:            peermap,
		hostToPeer:         hostToPeer,
		cluster:            cluster,
		debugChannel:       make(chan *DebugMessage),
		committedChannel:   make(chan *string),
		errorChannel:       make(chan error),
		requestChannel:     make(chan *string, 10), // some nice inherent rate limiting
		preprepareChannel:  make(chan *PrePrepareFull),
		prepareChannel:     make(chan *Prepare),
		commitChannel:      make(chan *Commit),
		viewChangeChannel:  make(chan *ViewChange),
		newViewChannel:     make(chan *NewView),
		timeoutChannel:     make(chan bool),
		log:                make(map[SlotId]*Slot),
		viewNumber:         0,
		sequenceNumber:     0,
		viewChange:         &viewChangeInfo{inProgress: false, viewNumber: 0},
		viewChangeMessages: make(map[NodeId]*ViewChange),
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
		case <-n.timeoutChannel: // one of my client requests timed out!
			n.startViewChange(n.viewNumber + 1)
		case msg := <-n.viewChangeChannel:
			n.handleViewChange(msg)
		case msg := <-n.newViewChannel:
			n.handleNewView(msg)
		}
	}
}

// does appropriate actions after receivin a client request
// i.e. send out preprepares and stuff
func (n *PBFTNode) handleClientRequest(request *string) {
	if n.viewChange.inProgress {
		return
	}

	if n.isPrimary() {
		if request == nil {
			return
		}
		n.sequenceNumber = n.sequenceNumber + 1
		id := SlotId{
			ViewNumber: n.viewNumber,
			SeqNumber:  n.sequenceNumber,
		}

		n.Log("Received new request - View Number: %d, Sequence Number: %d", n.viewNumber, n.sequenceNumber)

		requestDigest, err := util.GenerateDigest(*request)
		if err != nil {
			n.Log(err.Error())
			return
		}

		fullMessage := PrePrepareFull{
			PrePrepareMessage: PrePrepare{
				Number:        id,
				RequestDigest: requestDigest,
				Digest:        [sha256.Size]byte{},
			},
			Request: *request,
		}
		fullMessage.PrePrepareMessage.SetDigest()

		n.log[id] = &Slot{
			request:       request,
			requestDigest: requestDigest,
			preprepare:    &fullMessage.PrePrepareMessage,
			prepares:      make(map[NodeId]*Prepare),
			commits:       make(map[NodeId]*Commit),
			prepared:      false,
			committed:     false,
		}
		go n.broadcast("PBFTNode.PrePrepare", &fullMessage)

		// start execution timer...
		go func(n *PBFTNode) {
			<-time.After(2 * time.Second)
			if !n.log[id].committed {
				n.timeoutChannel <- true
			}
		}(n)
	} else {
		// forward to all ma frandz if im not da leader
		go n.broadcast("PBFTNode.ClientRequest", request)

		// TODO: send just to primary?
		// primaryId, primaryHostname := n.getPrimary()
		// n.Log("primary id: %d", primaryId)
		// go sendRpc(n.id, primaryId, primaryHostname, "PBFTNode.ClientRequest", n.cluster.Endpoint, request, nil, 5)

		// TODO: do some sort of timeout for a view change
	}
}

// TODO: verification should include:
//     1. only the leader of the specified view can send preprepares
// TODO: handle high, low water marks (Paper 4.2)
func (n *PBFTNode) handlePrePrepare(preprepare *PrePrepareFull) {
	if n.viewChange.inProgress {
		return
	}

	preprepareMessage := preprepare.PrePrepareMessage
	if !preprepareMessage.DigestValid() {
		n.Log("Error: PrePrepare digest does not match data")
		return
	}

	requestDigest, err := util.GenerateDigest(preprepare.Request)
	if err != nil {
		n.Log(err.Error())
		return
	}

	if preprepareMessage.RequestDigest != requestDigest {
		n.Log("Error: PrePrepare request digest does not match the request data")
		return
	}

	slot := n.ensureMapping(preprepareMessage.Number)

	// XXX: assumes that it's correct to NOT resend Prepares if we've already seen or sent the PrePrepare (i.e. request != nil)
	if slot.request != nil {
		if slot.requestDigest != preprepareMessage.RequestDigest {
			plog.Errorf("Received pre-prepare for slot id %+v with mismatched digest.", preprepareMessage.Number)
		}
		return
	}

	slot.request = &preprepare.Request
	slot.requestDigest = preprepareMessage.RequestDigest
	slot.preprepare = &preprepareMessage

	// TODO: iterate through potentially existing Prepare and Commit entries and check hashes, throw away non-matching with warning

	prepare := Prepare{
		Number:        preprepareMessage.Number,
		RequestDigest: preprepareMessage.RequestDigest,
		Node:          n.id,
		Digest:        [sha256.Size]byte{},
	}
	prepare.SetDigest()

	slot.prepares[n.id] = &prepare
	go n.broadcast("PBFTNode.Prepare", &prepare)
}

func (n *PBFTNode) handlePrepare(message *Prepare) {
	if n.viewChange.inProgress {
		return
	}

	if !message.DigestValid() {
		n.Log("Error: Prepare digest does not match data")
		return
	}

	slot := n.ensureMapping(message.Number)
	if slot.request != nil && slot.requestDigest != message.RequestDigest {
		plog.Errorf("Received prepare for slot id %+v with mismatched digest.", message.Number)
	}
	slot.prepares[message.Node] = message

	if !slot.prepared && n.isPrepared(slot) {
		n.Log("PREPARED %+v", message.Number)
		slot.prepared = true

		commit := Commit{
			Number:        message.Number,
			RequestDigest: message.RequestDigest,
			Node:          n.id,
			Digest:        [sha256.Size]byte{},
		}
		commit.SetDigest()
		slot.commits[n.id] = &commit

		go n.broadcast("PBFTNode.Commit", &commit)
	}
}

func (n *PBFTNode) handleCommit(message *Commit) {
	if n.viewChange.inProgress {
		return
	}

	if !message.DigestValid() {
		n.Log("Error: Commit digest does not match data")
		return
	}

	slot := n.ensureMapping(message.Number)
	if slot.request != nil && slot.requestDigest != message.RequestDigest {
		plog.Errorf("Received commit for slot id %+v with mismatched digest.", message.Number)
	}
	slot.commits[message.Node] = message

	if !slot.committed && n.isCommitted(slot) {
		n.Log("COMMITTED %+v", message.Number)
		slot.committed = true

		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel
		// TODO: fix this
		n.Committed() <- slot.request
	}
}

func (n *PBFTNode) handleViewChange(message *ViewChange) {
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
	n.viewChangeMessages[message.Node] = message
	// 1. If a replica receives a set of f+1 valid view change messages
	// from other replicas for views higher than its current view...
	if message.ViewNumber > currentView {
		higherThanCurrent := 0
		lowestNewView := message.ViewNumber
		for _, msg := range n.viewChangeMessages {
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
		n.Log("I'm the leader for view %d", message.ViewNumber)
		// 1. See if we got 2f view-change messages for this view!
		votes := 0
		for _, msg := range n.viewChangeMessages {
			if msg.ViewNumber == message.ViewNumber {
				votes += 1
			}
		}
		// 2. If so, multicast new-view
		if votes >= (2 * len(n.peermap) / 3) {
			newview := NewView{
				ViewNumber: message.ViewNumber,
				Node:       n.id,
			}
			go n.broadcast("PBFTNode.NewView", &newview)
			n.enterNewView(message.ViewNumber)
		}
	}
}

func (n *PBFTNode) handleNewView(message *NewView) {
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
// (done) if a replica gets f+1 view change messages for a higher view, then send view-change
//         with smallest view of the message set

func (n *PBFTNode) startViewChange(view int) {
	if n.viewChange.inProgress && n.viewChange.viewNumber >= view {
		return
	}
	n.Log("START VIEW CHANGE FOR VIEW %d", view)
	n.viewChange.inProgress = true
	n.viewChange.viewNumber = view
	message := ViewChange{
		ViewNumber: view,
		Node:       n.id,
	}
	go n.broadcast("PBFTNode.ViewChange", &message)
}

func (n *PBFTNode) enterNewView(view int) {
	n.Log("ENTER NEW VIEW FOR VIEW %d", view)
	n.viewChange.inProgress = false
	n.viewNumber = view
	n.sequenceNumber = 0
}

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Committed() chan *string {
	return n.committedChannel
}

func (n *PBFTNode) Propose(operation *string) {
	n.requestChannel <- operation
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

func (n PBFTNode) ClientRequest(req *string, res *Ack) error {
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

func (n PBFTNode) ViewChange(req *ViewChange, res *Ack) error {
	n.viewChangeChannel <- req
	return nil
}

func (n PBFTNode) NewView(req *NewView, res *Ack) error {
	n.newViewChannel <- req
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
