package pbft

import (
	"distributepki/util"

	"crypto/sha256"
	"errors"
	"golang.org/x/crypto/openpgp"
	"io/ioutil"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"sync"
	"time"
)

type PBFTNode struct {
	//////
	// Immutable config data
	//////
	id            NodeId
	host          string
	port          int
	primary       bool
	peermap       map[NodeId]string // id => hostname
	hostToPeer    map[string]NodeId // hostname => id
	cluster       ClusterConfig
	entity        *openpgp.Entity
	peerEntityMap map[EntityFingerprint]NodeId
	peerEntities  openpgp.EntityList

	// MAIN MESSAGE CHANNELS.
	// Main execution loop selects from these.
	debugChannel           chan *DebugMessage
	requestChannel         chan *string
	recvSnapshotChannel    chan snapshot
	preprepareChannel      chan *FullPrePrepare
	prepareChannel         chan *SignedPrepare
	checkpointChannel      chan *SignedCheckpoint
	checkpointProofChannel chan *SignedCheckpointProof
	commitChannel          chan *SignedCommit
	viewChangeChannel      chan *SignedViewChange
	newViewChannel         chan *SignedNewView
	requestTimeoutChannel  chan bool

	// CLIENT CHANNELS.
	// We write to these when we learn the
	// result of a client request.
	errorChannel           chan error
	requestSnapshotChannel chan SlotId
	committedChannel       chan *string
	snapshottedChannel     chan *[]byte

	// Requests: did they finish yet?
	requests map[[sha256.Size]byte]requestInfo

	// local key grabbing hack
	KeyRequest chan *KeyRequest // hostname

	//////
	// The below are all mutable, but writes should ALWAYS
	// happen on the main routine unless otherwise indicated.
	//////

	// LEDGER (and where we are in it)
	// Sequence numbers have to start at 1 for a subtle reason.
	// a node with seqnum 0 hasn't yet "committed" to the
	// current view.
	log                  map[SlotId]*Slot
	viewNumber           int
	sequenceNumber       int
	issuedSequenceNumber int

	// VIEW CHANGE STATE. We are in the middle of a viewchange
	// if viewChange.inProgress.
	viewChange *viewChangeInfo

	// CHECKPOINT STATE.
	lastCheckpoint     CheckpointProof
	pendingCheckpoints map[SlotId]CheckpointProof

	// TIMEOUTS. The heartbeat ticker allows the primary to
	// continually send timeouts; replicas use the timeout
	// timer to determine if the leader has been active.
	heartbeatTicker *time.Ticker
	timeoutTimer    *time.Timer

	// LEADER STATE (to catch up stragglers)
	// if some nodes aren't caught up, newView is broadcasted
	// to them instead of regular PrePrepare heartbeats.
	// note: caughtUp is written to in a goroutine when peers reply
	// to heartbeats, so we lock it.
	// are all my peers caught up? map peer => current seqnum
	caughtUp    map[NodeId]int
	caughtUpMux sync.RWMutex
	newView     *NewView // view message to continually broadcast

	// Debug states
	down bool
	slow bool
}

type snapshot struct {
	number SlotId
	state  []byte
}

// // last stable checkpoint & proof
// // unstable checkpoints & building proofs
// type checkpointInfo struct {
// 	number SlotId
// 	// checkpoint Checkpoint
// 	proof map[NodeId]Checkpoint
// }

type requestInfo struct {
	committed bool
	// also info about the reply that we sent
}

// Information associated with current view change.
type viewChangeInfo struct {
	messages   map[NodeId]SignedViewChange
	inProgress bool
	viewNumber int
}

type KeyRequest struct {
	Hostname string
	Reply    chan *string
}

// Heartbeat ticker
const TIMEOUT time.Duration = time.Duration(1 * time.Second)

// How many sequence numbers to wait before checkpointing
const CHECKPOINT uint = 3

// Entry point for each PBFT node.
// NodeConfig: configuration for this node
// ClusterConfig: configuration for entire cluster
// ready channel: send item when node is up & running
func StartNode(host NodeConfig, cluster ClusterConfig) *PBFTNode {

	// 1. Read PGP private key
	hostEntityList, err := ReadPgpKeyFile(host.PrivateKeyFile)
	if err != nil {
		plog.Fatalf("StartNode(%d) reading private key: %s", host.Id, err.Error())
	} else if len(hostEntityList) != 1 {
		plog.Errorf("StartNode(%d) reading private key: expected only 1 host PGP entity, got %d", host.Id, len(hostEntityList))
	}
	hostEntity := hostEntityList[0]
	plog.Infof("node primary key fingerprint: %+v", hostEntity.PrimaryKey.Fingerprint)

	phrase, err := ioutil.ReadFile(host.PassPhraseFile)
	if err != nil {
		plog.Fatalf("StartNode(%d) reading passphrase: %s", host.Id, err.Error())
	}
	passphrase := strings.TrimSpace(string(phrase))

	err = hostEntity.PrivateKey.Decrypt([]byte(passphrase))
	if err != nil {
		plog.Fatalf("StartNode(%d) decrypting private key: %s", host.Id, err.Error())
	}

	// 2. Create id <=> peer hostname maps from list, read PGP public keys
	peermap := make(map[NodeId]string)
	hostToPeer := make(map[string]NodeId)
	peerEntityMap := make(map[EntityFingerprint]NodeId)
	peerEntities := make(openpgp.EntityList, 0)
	for _, p := range cluster.Nodes {
		if p.Id != host.Id {
			hostname := util.GetHostname(p.Host, p.Port)
			peermap[p.Id] = hostname
			hostToPeer[hostname] = p.Id

			list, err := ReadPgpKeyFile(p.PublicKeyFile)
			if err != nil {
				plog.Fatalf("StartNode(%d) reading node %d public key: %s", host.Id, p.Id, err.Error())
			} else if len(list) != 1 {
				plog.Errorf("StartNode(%d) reading node %d public key: expected only 1 PGP entity, got %d", host.Id, p.Id, len(list))
			}
			peerEntityMap[list[0].PrimaryKey.Fingerprint] = p.Id
			peerEntities = append(peerEntities, list[0])
		}
	}

	// 3. Create the node
	node := PBFTNode{
		id:                     host.Id,
		host:                   host.Host,
		port:                   host.Port,
		peermap:                peermap,
		hostToPeer:             hostToPeer,
		cluster:                cluster,
		entity:                 hostEntity,
		peerEntityMap:          peerEntityMap,
		peerEntities:           peerEntities,
		debugChannel:           make(chan *DebugMessage),
		committedChannel:       make(chan *string),
		requestSnapshotChannel: make(chan SlotId),
		errorChannel:           make(chan error),
		requestChannel:         make(chan *string, 10), // some nice inherent rate limiting
		recvSnapshotChannel:    make(chan snapshot),
		snapshottedChannel:     make(chan *[]byte),
		preprepareChannel:      make(chan *FullPrePrepare),
		prepareChannel:         make(chan *SignedPrepare),
		commitChannel:          make(chan *SignedCommit),
		checkpointChannel:      make(chan *SignedCheckpoint),
		checkpointProofChannel: make(chan *SignedCheckpointProof),
		viewChangeChannel:      make(chan *SignedViewChange),
		newViewChannel:         make(chan *SignedNewView),
		requestTimeoutChannel:  make(chan bool),
		requests:               make(map[[sha256.Size]byte]requestInfo),
		log:                    make(map[SlotId]*Slot),
		viewNumber:             0,
		sequenceNumber:         1,
		issuedSequenceNumber:   1,
		viewChange:             &viewChangeInfo{inProgress: false, viewNumber: 0, messages: make(map[NodeId]SignedViewChange)},
		lastCheckpoint: CheckpointProof{
			Number:   SlotId{ViewNumber: 0, SeqNumber: 0},
			Snapshot: make([]byte, 0),
			Proof:    make(map[NodeId]SignedCheckpoint)},
		pendingCheckpoints: make(map[SlotId]CheckpointProof),
		heartbeatTicker:    nil,
		timeoutTimer:       nil,
		caughtUp:           make(map[NodeId]int),
		newView:            &NewView{ViewNumber: 0, Node: host.Id},
		down:               false,
		slow:               false,
	}
	for p, _ := range node.peermap {
		node.caughtUp[p] = 1
	}

	// 4. Start RPC server
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

	// 5. Start exec loop
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
func (n *PBFTNode) ensureMapping(num SlotId) *Slot {
	slot, ok := n.log[num]
	if !ok {
		slot = &Slot{
			request:       nil,
			requestDigest: [sha256.Size]byte{},
			preprepare:    nil,
			prepares:      make(map[NodeId]SignedPrepare),
			commits:       make(map[NodeId]*SignedCommit),
			prepared:      false,
			committed:     false,
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

func (n PBFTNode) isPrepared(slot *Slot) bool {
	// # Prepares received >= 2f = 2 * ((N - 1) / 3)
	return slot.preprepare != nil && len(slot.prepares) >= 2*(len(n.peermap)/3)
}

func (n PBFTNode) isCommitted(slot *Slot) bool {
	// # Commits received >= 2f = 2 * ((N - 1) / 3)
	return len(slot.commits) >= 2*(len(n.peermap)/3)
}

// ** ALL THE MESSAGE HANDLERS ** //
// MAIN EXECUTION LOOP
func (n *PBFTNode) handleMessages() {
	for {
		select {
		// come from RPCS
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
		case msg := <-n.checkpointChannel:
			n.handleCheckpoint(msg)
		case msg := <-n.checkpointProofChannel:
			n.handleCheckpointProof(msg)
		case msg := <-n.viewChangeChannel:
			n.handleViewChange(msg)
		case msg := <-n.newViewChannel:
			n.handleNewView(msg)
		// from client
		case snapshot := <-n.recvSnapshotChannel:
			n.handleRecvSnapshot(&snapshot)
		// Come from internal timers
		case <-n.requestTimeoutChannel: // one of my client requests timed out!
			n.startViewChange(n.viewNumber + 1)
		case <-n.getTimer(): // timer expired
			n.handleHeartbeatTimeout()
		}
	}
}

// does appropriate actions after receivin a client request
// i.e. send out preprepares and stuff
func (n *PBFTNode) handleClientRequest(request *string) {
	if n.viewChange.inProgress {
		return
	}
	requestDigest, err := util.GenerateDigest(*request)
	if _, ok := n.requests[requestDigest]; ok {
		// we've already processed this client request
		return
	}
	n.requests[requestDigest] = requestInfo{committed: false}

	if n.isPrimary() {
		if request == nil {
			return
		}
		if err != nil {
			n.Log(err.Error())
			return
		}
		n.issuedSequenceNumber = n.issuedSequenceNumber + 1
		id := SlotId{
			ViewNumber: n.viewNumber,
			SeqNumber:  n.issuedSequenceNumber,
		}
		n.Log("Received new request - View Number: %d, Sequence Number: %d", n.viewNumber, n.issuedSequenceNumber)

		message := PrePrepare{
			Number:        id,
			RequestDigest: requestDigest,
		}
		signedMessage, err := message.Sign(n.entity)
		if err != nil {
			n.Log("Signing pre-prepare: " + err.Error())
			return
		}
		fullMessage := FullPrePrepare{
			SignedMessage: *signedMessage,
			Request:       *request,
		}

		n.log[id] = &Slot{
			request:       request,
			requestDigest: requestDigest,
			preprepare:    &fullMessage.SignedMessage,
			prepares:      make(map[NodeId]SignedPrepare),
			commits:       make(map[NodeId]*SignedCommit),
			prepared:      false,
			committed:     false,
		}
		go n.broadcast("PBFTNode.PrePrepare", &fullMessage, 0)
	} else {
		// forward to all ma frandz if im not da leader
		// TODO: they should probably just forward it to the leader
		go n.broadcast("PBFTNode.ClientRequest", request, 0)
		// primaryId, primaryHostname := n.getPrimary()
		// n.Log("primary id: %d", primaryId)
		// go sendRpc(n.id, primaryId, primaryHostname, "PBFTNode.ClientRequest", n.cluster.Endpoint, request, nil, 5)

		// TODO: do some sort of timeout for a view change
		/*
			// start execution timer...
			go func(n *PBFTNode) {
				<-time.After(TIMEOUT)
				if !n.requests[request.Id].committed {
					n.requestTimeoutChannel <- true
				}
			}(n)
		*/
	}
}

func (n *PBFTNode) handlePrePrepare(preprepare *FullPrePrepare) {
	if n.viewChange.inProgress || n.isPrimary() {
		return
	}

	sendingNode, err := preprepare.SignedMessage.SignatureValid(n.peerEntities, n.peerEntityMap)
	sameView := preprepare.SignedMessage.PrePrepareMessage.Number.ViewNumber == n.viewNumber
	if err != nil {
		n.Log("Validating PrePrepare signature: " + err.Error())
		return
	} else if primaryNode, _ := n.getPrimary(); sameView && sendingNode != primaryNode {
		n.Log("Error: received PrePrepare not signed by current primary")
		return
	}

	// reset heartbeat timer!
	n.timeoutTimer.Reset(n.getTimeout())
	preprepareMessage := preprepare.SignedMessage.PrePrepareMessage
	//NO-OP heartbeat... don't bother processing
	if preprepareMessage == (PrePrepare{}) {
		return
	}

	// A backup accepts a pre-prepare message provided 4 things:
	// 1. it is in view v
	if preprepareMessage.Number.ViewNumber != n.viewNumber {
		return
	}
	// 2. the sequence number in the preprepare message is between the
	//    low water-mark h and high water-mark H
	if preprepareMessage.Number.SeqNumber > n.lastCheckpoint.Number.SeqNumber+int(CHECKPOINT*3) ||
		preprepareMessage.Number.SeqNumber <= n.lastCheckpoint.Number.SeqNumber {
		return
	}
	// 3. the signatures in the request and the pre-prepare message are
	//    correct (message signature checked above) and d is the digest for message m
	// TODO: (jlwatson) check request signature. most likely a call into KeyNode
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
	// TODO (sydli): i think it's ok to re-prepare to catch up other nodes
	if slot.request != nil {
		// 4. it has not accepted a pre-prepare message for view v and seq
		//    num n containing a different digest
		if slot.requestDigest != preprepareMessage.RequestDigest {
			plog.Errorf("Received pre-prepare for slot id %+v with mismatched digest.", preprepareMessage.Number)
		}
		return
	}

	newSeqNum := preprepareMessage.Number.SeqNumber
	if newSeqNum > n.issuedSequenceNumber {
		n.issuedSequenceNumber = newSeqNum
	}

	slot.request = &preprepare.Request
	slot.requestDigest = preprepareMessage.RequestDigest
	slot.preprepare = &preprepare.SignedMessage

	// TODO: iterate through potentially existing Prepare and Commit entries and check hashes, throw away non-matching with warning

	prepare := Prepare{
		Number:        preprepareMessage.Number,
		RequestDigest: preprepareMessage.RequestDigest,
		Node:          n.id,
	}
	signedMessage, err := prepare.Sign(n.entity)
	if err != nil {
		n.Log("Signing prepare: " + err.Error())
		return
	}

	slot.prepares[n.id] = *signedMessage
	n.log[preprepareMessage.Number].preprepare = &preprepare.SignedMessage
	go n.broadcast("PBFTNode.Prepare", signedMessage, 0)
}

func (n *PBFTNode) handlePrepare(message *SignedPrepare) {
	if n.viewChange.inProgress {
		return
	}

	sender, err := message.SignatureValid(n.peerEntities, n.peerEntityMap)
	if err != nil {
		n.Log("Validating Prepare signature: " + err.Error())
		return
	} else if sender != message.PrepareMessage.Node {
		n.Log("Error: received Prepare not signed by correct sending node")
		return
	}

	prepare := message.PrepareMessage
	slot := n.ensureMapping(prepare.Number)
	if slot.request != nil && slot.requestDigest != prepare.RequestDigest {
		plog.Errorf("Received prepare for slot id %+v with mismatched digest.", prepare.Number)
	}
	slot.prepares[prepare.Node] = *message

	n.log[prepare.Number] = slot
	if n.isPrepared(slot) {
		n.Log("PREPARED %+v", prepare.Number)
		slot.prepared = true

		commit := Commit{
			Number:        prepare.Number,
			RequestDigest: prepare.RequestDigest,
			Node:          n.id,
		}
		signedMessage, err := commit.Sign(n.entity)
		if err != nil {
			n.Log("Signing commit: " + err.Error())
			return
		}

		slot.commits[n.id] = signedMessage
		go n.broadcast("PBFTNode.Commit", signedMessage, 0)
	}
}

func (n *PBFTNode) handleCommit(message *SignedCommit) {
	if n.viewChange.inProgress {
		return
	}

	sender, err := message.SignatureValid(n.peerEntities, n.peerEntityMap)
	if err != nil {
		n.Log("Validating Commit signature: " + err.Error())
		return
	} else if sender != message.CommitMessage.Node {
		n.Log("Error: received Commit not signed by correct sending node")
		return
	}

	commit := message.CommitMessage
	if commit.Number.Before(n.lastCheckpoint.Number) {
		return
	}

	slot := n.ensureMapping(commit.Number)
	if slot.request != nil && slot.requestDigest != commit.RequestDigest {
		plog.Errorf("Received commit for slot id %+v with mismatched digest.", commit.Number)
	}
	slot.commits[commit.Node] = message
	nowCommitted := !slot.committed && n.isCommitted(slot)
	if nowCommitted {
		n.Log("COMMITTED %+v", commit.Number)
		slot.committed = true
		// info := n.requests[commit.Message.Id] //.committed = true
		// n.requests[commit.Message.Id] = requestInfo{
		// 	id:        info.id,
		// 	committed: true,
		// 	request:   info.request,
		// }
		if commit.Number.SeqNumber > n.sequenceNumber {
			n.sequenceNumber = commit.Number.SeqNumber
		}
	}
	n.log[commit.Number] = slot
	if nowCommitted {
		// TODO: try to execute as many sequential queries as possible and
		// then reply to the clients via committedChannel. Figure out what
		// the highest committed operation sequence number was and apply
		// up to that
		// TODO: fix this
		n.Committed() <- slot.request
		n.tryCheckpoint()
	}
}

type requestView struct {
	request       string
	requestDigest [sha256.Size]byte
	view          int
}

// ** TIMER/HEARTBEAT STUFF ** //

func (n *PBFTNode) getTimeout() time.Duration {
	// When primary times out, it send a heartbeat
	if n.isPrimary() {
		return TIMEOUT
	} else {
		// When non-primaries timeout with no heartbeat,
		// they start view change
		return TIMEOUT * time.Duration(len(n.peermap)) / 2
	}
}

func (n *PBFTNode) handleHeartbeatTimeout() {
	if n.viewChange.inProgress {
		return
	}
	if n.down {
		return
	}
	if !n.isPrimary() {
		n.startViewChange(n.viewNumber + 1)
	} else {
		n.sendHeartbeat()
	}
}

// returns rpcName, message
func (n *PBFTNode) heartbeatMessage(peerSequence int) (string, interface{}) {
	if n.sequenceNumber == peerSequence {
		pp := PrePrepare{}
		signedMessage, err := pp.Sign(n.entity)
		if err != nil {
			plog.Fatal("Error signing empty heartbeat PrePrepare")
		}
		return "PBFTNode.PrePrepare", FullPrePrepare{
			SignedMessage: *signedMessage,
			Request:       "",
		}
	}
	if peerSequence < n.lastCheckpoint.Number.SeqNumber {
		message := CheckpointProofMessage{
			Proof: n.lastCheckpoint,
			Node:  n.id,
		}
		signedMessage, err := message.Sign(n.entity)
		if err != nil {
			plog.Fatal("Error signing checkpoint proof")
		}
		return "PBFTNode.CheckpointProof", signedMessage
	}

	slot := SlotId{
		ViewNumber: n.viewNumber,
		SeqNumber:  peerSequence + 1,
	}
	return "PBFTNode.PrePrepare", FullPrePrepare{
		SignedMessage: *n.log[slot].preprepare,
		Request:       *n.log[slot].request,
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
		var after func(NodeId, SignedPPResponse, error)
		// TOCTOU isn't too dangerous here since
		// it just means we send an unnecessary preprepare
		// that the peer has already received before
		n.caughtUpMux.RLock()
		caughtUp := n.caughtUp[id]
		n.caughtUpMux.RUnlock()
		if caughtUp == 0 {
			rpcType = "PBFTNode.NewView"
			newViewMessage := *n.newView
			signedMessage, err := newViewMessage.Sign(n.entity)
			if err != nil {
				n.Log("Signing NewView heartbeat: " + err.Error())
				return
			}
			msg = signedMessage
			after = func(id NodeId, response SignedPPResponse, err error) {
				if err == nil {
					n.caughtUpMux.Lock()
					n.caughtUp[id] = 1
					n.caughtUpMux.Unlock()
				}
			}
		} else {
			rpcType, msg = n.heartbeatMessage(caughtUp)
			after = func(id NodeId, response SignedPPResponse, err error) {
				if err == nil {
					respId, err := response.SignatureValid(n.peerEntities, n.peerEntityMap)
					if err != nil {
						n.Log("Error validating heartbeat signature: " + err.Error())
					} else if respId != id {
						n.Log("Error: heartbeat signed by wrong node")
					} else {
						n.caughtUpMux.Lock()
						n.caughtUp[id] = response.Response.SeqNumber
						n.caughtUpMux.Unlock()
					}
				}
			}
		}
		go func(id NodeId, hostname string, rpcType string, msg interface{}, after func(NodeId, SignedPPResponse, error)) {
			resp := SignedPPResponse{}
			err := sendRpc(n.id, id, hostname, rpcType, n.cluster.Endpoint, msg, &resp, 1, time.Duration(100*time.Millisecond))
			after(id, resp, err)
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

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Committed() chan *string {
	return n.committedChannel
}

func (n *PBFTNode) Propose(operation *string) {
	n.requestChannel <- operation
}

func (n PBFTNode) ClientRequest(req *string, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.requestChannel <- req
	return nil
}

func (n PBFTNode) PrePrepare(req *FullPrePrepare, res *SignedPPResponse) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.preprepareChannel <- req

	res.Response.SeqNumber = n.sequenceNumber
	sig, err := res.Response.GetSignature(n.entity)
	if err != nil {
		return err
	}
	res.Signature = sig

	return nil
}

func (n PBFTNode) Prepare(req *SignedPrepare, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.prepareChannel <- req
	return nil
}

func (n PBFTNode) Commit(req *SignedCommit, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.commitChannel <- req
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
	// plog.Infof("[Node %d] Sending RPC (%s) to Node %d", fromId, rpcName, peerId)
	return util.SendRpc(hostName, endpoint, rpcName, message, response, retries, 0)
}
