package pbft

import (
	"distributepki/util"
	"net"
	"net/http"
	"net/rpc"
)

type LogId struct {
	viewNumber int
	seqNumber  int
}

type LogEntry struct {
	request    *ClientRequest
	preprepare PrePrepare
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
	preprepareChannel chan bool
	committedChannel  chan *string

	log        map[LogId]LogEntry
	viewNumber int
	seqNumber  int
}

type ReadyMsg int
type ReadyResp bool

func StartNode(host NodeConfig, cluster ClusterConfig, ready chan<- *PBFTNode) {

	peers := make([]string, len(cluster.Nodes)-1)
	for _, p := range cluster.Nodes {
		if p.Id != host.Id {
			peers = append(peers, util.GetHostname(p.Host, p.Port))
		}
	}

	node := PBFTNode{
		id:                host.Id,
		host:              host.Host,
		port:              host.Port,
		primary:           cluster.Primary.Id == host.Id,
		peers:             peers,
		startup:           make(chan bool, len(cluster.Nodes)-1),
		errorChannel:      make(chan error),
		requestChannel:    make(chan *ClientRequest, 10), // some nice inherent rate limiting
		preprepareChannel: make(chan bool),
		committedChannel:  make(chan *string),
		log:               make(map[LogId]LogEntry),
		viewNumber:        0,
		seqNumber:         0,
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
		go node.handleRequests()
	} else {
		node.signalReady(cluster)
		go node.handlePrePrepares()
	}
	ready <- &node
}

func (n PBFTNode) ConfigChange(interface{}) {
	// XXX: Do nothing, cluster configuration changes not supported
}

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Propose(opcode int, s string) {

	if !n.primary {
		return // no proposals for non-primary nodes
	}

	request := new(ClientRequest)
	request.opcode = opcode
	request.op = s

	n.requestChannel <- request
}

func (n PBFTNode) Committed() chan *string {
	return n.committedChannel
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

// ** Remote Calls ** //
func (n PBFTNode) handleRequests() {
	requestC := n.requestChannel
	for {
		request := <-requestC
		if request == nil {
			break
		}

		id := LogId{
			viewNumber: n.viewNumber,
			seqNumber:  n.seqNumber,
		}
		n.seqNumber += 1

		pp := PrePrepare{
			viewNumber: id.viewNumber,
			seqNumber:  id.seqNumber,
		}
		fullMessage := PrePrepareFull{
			message: *request,
			pp:      pp,
		}

		n.log[id] = LogEntry{
			request:    request,
			preprepare: pp,
		}

		responses := make([]interface{}, len(n.peers))
		for i := 0; i < len(n.peers); i++ {
			responses[i] = Ack{}
		}
		log.Infof("Sending PrePrepare messages for %+v", id)
		bcastRPC(n.peers, "PBFTNode.PrePrepare", &fullMessage, responses, 10)

		for i, r := range responses {
			log.Infof("PrePrepare response %d: %+v", i, r)
		}
	}
}

func (n PBFTNode) handlePrePrepares() {
	incoming := n.preprepareChannel
	for {
		pp := <-incoming
		log.Infof("PrePrepare detected %b", pp)
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
	res.success = true
	n.preprepareChannel <- true
	return nil
}

// ** RPC ** //

func bcastRPC(peers []string, rpcName string, message interface{}, response []interface{}, retries int) {
	for i, p := range peers {
		sendRPC(p, rpcName, message, response[i], retries)
	}
}

func sendRPC(hostName string, rpcName string, message interface{}, response interface{}, retries int) error {

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
