package pbft

import (
	//"fmt"
	//"net"
	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

type ClusterConfig struct {
	Primary PrimaryConfig
	Nodes   []NodeConfig
}

type PrimaryConfig struct {
	Id      int
	RpcPort int
}

type NodeConfig struct {
	Id   int
	Host string
	Port int
	Key  string
}

type KeyPair struct {
	Alias string
	Key   string
}
