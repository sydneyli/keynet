package pbft

import (
	"github.com/coreos/pkg/capnslog"

	"crypto/sha256"
	"encoding/binary"
	"golang.org/x/crypto/openpgp"
	"os"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

type NodeId uint
type EntityFingerprint [20]byte

type ClusterConfig struct {
	Nodes            []NodeConfig
	AuthorityKeyFile string
	Endpoint         string
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
	ClientPort     int
	Host           string
	Id             NodeId
	Port           int
	PrivateKeyFile string
	PublicKeyFile  string
	PassPhraseFile string
}

type EndpointConfig struct {
	DebugEndpoint string
	Endpoint      string
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
	request       *string
	requestDigest [sha256.Size]byte
	preprepare    *SignedPrePrepare
	prepares      map[NodeId]SignedPrepare
	commits       map[NodeId]*SignedCommit
	prepared      bool
	committed     bool
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

func ReadPgpKeyFile(path string) (openpgp.EntityList, error) {
	keyfile, e := os.Open(path)
	if e != nil {
		var empty openpgp.EntityList
		return empty, e
	}

	list, readErr := openpgp.ReadArmoredKeyRing(keyfile)
	if readErr != nil {
		var empty openpgp.EntityList
		return empty, readErr
	}
	return list, nil
}
