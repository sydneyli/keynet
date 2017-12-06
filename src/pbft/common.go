package pbft

import (
	"github.com/coreos/pkg/capnslog"

	"bytes"
	"encoding/binary"
	"hash/fnv"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

type NodeId uint

type ClusterConfig struct {
	Primary PrimaryConfig
	Nodes   []NodeConfig
}

func hash(data []byte) uint {
	h := fnv.New32a()
	h.Write(data)
	return uint(h.Sum32())
}

// Deterministic leader calculation
func (c ClusterConfig) LeaderFor(viewNumber int) NodeId {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, viewNumber)
	return c.Nodes[hash(buf.Bytes())%uint(len(c.Nodes))].Id
}

type PrimaryConfig struct {
	Id      NodeId
	RpcPort int
}

type NodeConfig struct {
	Id   NodeId
	Host string
	Port int
	Key  string
}

type KeyPair struct {
	Alias string
	Key   string
}

type SlotId struct {
	ViewNumber int
	SeqNumber  int
}

type Slot struct {
	request    *ClientRequest
	preprepare *PrePrepareFull
	prepares   map[NodeId]*Prepare
	commits    map[NodeId]*Commit
	prepared   bool
	committed  bool
}

func (slot SlotId) Before(other SlotId) bool {
	if slot.ViewNumber == other.ViewNumber {
		return slot.SeqNumber < other.SeqNumber
	}
	return slot.ViewNumber < other.ViewNumber
}
