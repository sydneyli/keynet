package pbft

import (
	"github.com/coreos/pkg/capnslog"

	"crypto/sha256"
	"encoding/binary"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

type NodeId uint

type ClusterConfig struct {
	Nodes    []NodeConfig
	Endpoint string
}

func hash(data []byte) uint32 {
	h := sha256.Sum256(data)
	return binary.LittleEndian.Uint32(h[:])
}

// Deterministic leader calculation
func (c ClusterConfig) LeaderFor(viewNumber int) NodeId {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(viewNumber))
	return c.Nodes[hash(buf)%uint32(len(c.Nodes))].Id
}

type NodeConfig struct {
	Id         NodeId
	Host       string
	Port       int
	ClientPort int
	Key        string
}

type EndpointConfig struct {
	Endpoint      string
	DebugEndpoint string
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

func (slot SlotId) BeforeOrEqual(other SlotId) bool {
	if slot.ViewNumber == other.ViewNumber {
		return slot.SeqNumber <= other.SeqNumber
	}
	return slot.ViewNumber <= other.ViewNumber
}
