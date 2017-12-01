package pbft

import (
	//"fmt"
	//"net"
	"github.com/coreos/pkg/capnslog"
)

var (
	log = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

type ClusterConfig struct {
	Nodes []NodeConfig
}

type NodeConfig struct {
	Id       int
	Hostname string
	Primary  bool
	Key      string
}

type KeyPair struct {
	Alias string
	Key   string
}
