package pbft

import (
	"net"
	"net/http"
	"net/rpc"
)

type PBFTNode struct {
	id      int
	host    string
	port    string
	startup chan bool
}

type ReadyMsg int
type ReadyResp bool

func StartNode(host NodeConfig, cluster ClusterConfig, ready chan<- bool) {

	h, p, err := net.SplitHostPort(host.Hostname)
	if err != nil {
		log.Fatal("HostPort split error:", err)
	}

	node := PBFTNode{
		id:      host.Id,
		host:    h,
		port:    p,
		startup: make(chan bool, len(cluster.Nodes)-1),
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
	log.Info("waiting")
	ready <- true
	log.Info("done")
}

func (n *PBFTNode) signalReady(cluster ClusterConfig) {

	var primary NodeConfig
	for _, n := range cluster.Nodes {
		if n.Primary {
			primary = n
			break
		}
	}

	rpcClient, err := rpc.DialHTTP("tcp", primary.Hostname)
	for err != nil {
		log.Info(err)
		rpcClient, err = rpc.DialHTTP("tcp", primary.Hostname)
	}

	message := ReadyMsg(n.id)
	remoteCall := rpcClient.Go("PBFTNode.Ready", &message, new(ReadyResp), nil)
	log.Info("waiting for remote call")
	<-remoteCall.Done
}

func (n *PBFTNode) Ready(req *ReadyMsg, res *ReadyResp) error {
	log.Infof("Node %d ready to start", int(*req))
	*res = ReadyResp(true)
	n.startup <- true
	return nil
}
