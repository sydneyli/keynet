package pbft

import (
	"distributepki/common"
	"net"
	"net/http"
	"net/rpc"
)

type PBFTNode struct {
	id               int
	host             string
	port             string
	startup          chan bool
	errorChannel     chan error
	committedChannel chan *string
}

type ReadyMsg int
type ReadyResp bool

func StartNode(host NodeConfig, cluster ClusterConfig, ready chan<- common.ConsensusNode) {

	h, p, err := net.SplitHostPort(host.Hostname)
	if err != nil {
		log.Fatal("HostPort split error:", err)
	}

	node := PBFTNode{
		id:               host.Id,
		host:             h,
		port:             p,
		startup:          make(chan bool, len(cluster.Nodes)-1),
		errorChannel:     make(chan error),
		committedChannel: make(chan *string),
	}
	rpc.Register(&node)
	rpc.HandleHTTP()

	listener, e := net.Listen("tcp", ":"+node.port)
	if e != nil {
		log.Fatal("Listen error:", e)
	}
	go http.Serve(listener, nil)

	if host.Primary {
		for i := 0; i < len(cluster.Nodes)-1; i++ {
			<-node.startup
		}
	} else {
		node.signalReady(cluster)
	}
	ready <- node
}

func (n PBFTNode) ConfigChange(interface{}) {
	// XXX: Do nothing, cluster configuration changes not supported
}

func (n PBFTNode) Failure() chan error {
	return n.errorChannel
}

func (n PBFTNode) Propose(string) {
	// TODO: implement
}

func (n PBFTNode) Committed() chan *string {
	return n.committedChannel
}

func (n PBFTNode) GetCheckpoint() (interface{}, error) {
	// TODO: implement
	return nil, nil
}

func (n PBFTNode) signalReady(cluster ClusterConfig) {

	var primary NodeConfig
	for _, n := range cluster.Nodes {
		if n.Primary {
			primary = n
			break
		}
	}

	rpcClient, err := rpc.DialHTTP("tcp", primary.Hostname)
	for err != nil {
		rpcClient, err = rpc.DialHTTP("tcp", primary.Hostname)
	}

	message := ReadyMsg(n.id)
	remoteCall := rpcClient.Go("PBFTNode.Ready", &message, new(ReadyResp), nil)
	<-remoteCall.Done
}

func (n *PBFTNode) Ready(req *ReadyMsg, res *ReadyResp) error {
	*res = ReadyResp(true)
	n.startup <- true
	return nil
}
