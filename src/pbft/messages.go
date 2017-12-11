package pbft

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// REPLY:
// viewnum, timestamp, client addr, node addr, result
// (signed by node)
type ClientReply struct {
	result     string
	viewNumber int
	timestamp  time.Time
	client     net.Addr
	node       net.Addr
	digest     [sha256.Size]byte
}

// PRE-PREPARE:
// viewnum, seqnum, client message (digest)
// (signed by node)
type PrePrepare struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
}

type SignedPrePrepare struct {
	PrePrepareMessage PrePrepare
	Signature         []byte
}

type FullPrePrepare struct {
	SignedMessage SignedPrePrepare
	Request       string
}

// hehehe peepee
type PPResponse struct {
	SeqNumber int
}

type SignedPPResponse struct {
	Response  PPResponse
	Signature []byte
}

// PREPARE:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Prepare struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
	Node          NodeId
}

type SignedPrepare struct {
	PrepareMessage Prepare
	Signature      []byte
}

// COMMIT:
// viewnum, seqnum, client message (digest), node addr
// (signed by node i)
type Commit struct {
	Number        SlotId
	RequestDigest [sha256.Size]byte
	Node          NodeId
}

type SignedCommit struct {
	CommitMessage Commit
	Signature     []byte
}

// The checkpoint protocol is used to advance the low and high
// watermarks. Low watermark = sequence num of last checkpoint.
// High watermark = Low watermark + (checkpoint delta) * 2

// CHECKPOINT:
// seqnum, digest of state, node addr
// 2f + 1 of these is a proof for a particular seqnum's
// checkpoint
// (signed by node i)
type Checkpoint struct {
	Number   SlotId
	Snapshot []byte
	Node     NodeId
}

type SignedCheckpoint struct {
	CheckpointMessage Checkpoint
	Signature         []byte
}

type CheckpointProof struct {
	Number   SlotId
	Snapshot []byte
	Proof    map[NodeId]SignedCheckpoint
}

type CheckpointProofMessage struct {
	Proof CheckpointProof
	Node  NodeId
}

type SignedCheckpointProof struct {
	Message   CheckpointProofMessage
	Signature []byte
}

type PreparedProof struct {
	Number        SlotId
	Preprepare    SignedPrePrepare
	RequestDigest [sha256.Size]byte
	Prepares      map[NodeId]SignedPrepare
}

// VIEW CHANGE:
// n: last checkpoint sequence num
// C: Proof of checkpoint at n
// P: Proof of all PREPARED requests after n
//    1 Preprepare message (signed) and 2f+1 prepares per
// (signed by node i)
type ViewChange struct {
	ViewNumber      int                // v + 1
	Checkpoint      SlotId             // n
	CheckpointProof CheckpointProofMap // C
	Proofs          PreparedProofMap   // P
	Node            NodeId             // i
}

// random JSON serialization workarounds
type CheckpointProofMap map[NodeId]SignedCheckpoint
type PreparedProofMap map[SlotId]PreparedProof

func (m *CheckpointProofMap) UnmarshalJSON(b []byte) error {
	var strMap map[string]SignedCheckpoint
	if err := json.Unmarshal(b, &strMap); err != nil {
		return err
	}

	resultMap := make(CheckpointProofMap)
	for k, v := range strMap {
		uintK, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		resultMap[NodeId(uintK)] = v
	}

	*m = resultMap
	return nil
}

func (m CheckpointProofMap) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]SignedCheckpoint)
	for k, v := range m {
		strMap[string(k)] = v
	}

	return json.Marshal(strMap)
}

func (m *PreparedProofMap) UnmarshalJSON(b []byte) error {
	var strMap map[string]PreparedProof
	if err := json.Unmarshal(b, &strMap); err != nil {
		return err
	}

	resultMap := make(PreparedProofMap)
	for k, v := range strMap {
		s := strings.Split(k, ":")
		if len(s) != 2 {
			return errors.New("Unmarshalling JSON PreparedProofMap: invalid key")
		}

		view, err := strconv.Atoi(s[0])
		if err != nil {
			return err
		}

		seq, err := strconv.Atoi(s[1])

		resultMap[SlotId{
			ViewNumber: view,
			SeqNumber:  seq,
		}] = v
	}

	*m = resultMap
	return nil
}

func (m PreparedProofMap) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]PreparedProof)
	for k, v := range m {
		strMap[string(k.ViewNumber)+":"+string(k.SeqNumber)] = v
	}

	return json.Marshal(strMap)
}

type SignedViewChange struct {
	Message   ViewChange
	Signature []byte
}

// NEW VIEW
// viewnum, seqnum, client message (digest), node addr
// V: set of 2f valid view-change messages
// O: a set of pre-prepare messages
// (signed by node i)
type NewView struct {
	ViewNumber  int                  // v + 1
	ViewChanges NewViewViewChangeMap // V
	PrePrepares NewViewPrePrepareMap // O
	Node        NodeId               // i
}

type SignedNewView struct {
	Message   NewView
	Signature []byte
}

// more JSON trickery
type NewViewPrePrepareMap map[SlotId]FullPrePrepare
type NewViewViewChangeMap map[NodeId]SignedViewChange

func (m *NewViewPrePrepareMap) UnmarshalJSON(b []byte) error {
	var strMap map[string]FullPrePrepare
	if err := json.Unmarshal(b, &strMap); err != nil {
		return err
	}

	resultMap := make(NewViewPrePrepareMap)
	for k, v := range strMap {
		s := strings.Split(k, ":")
		if len(s) != 2 {
			return errors.New("Unmarshalling JSON NewViewPrePrepareMap: invalid key")
		}

		view, err := strconv.Atoi(s[0])
		if err != nil {
			return err
		}

		seq, err := strconv.Atoi(s[1])

		resultMap[SlotId{
			ViewNumber: view,
			SeqNumber:  seq,
		}] = v
	}

	*m = resultMap
	return nil
}

func (m NewViewPrePrepareMap) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]FullPrePrepare)
	for k, v := range m {
		strMap[string(k.ViewNumber)+":"+string(k.SeqNumber)] = v
	}

	return json.Marshal(strMap)
}

func (m *NewViewViewChangeMap) UnmarshalJSON(b []byte) error {
	var strMap map[string]SignedViewChange
	if err := json.Unmarshal(b, &strMap); err != nil {
		return err
	}

	resultMap := make(NewViewViewChangeMap)
	for k, v := range strMap {
		uintK, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		resultMap[NodeId(uintK)] = v
	}

	*m = resultMap
	return nil
}

func (m NewViewViewChangeMap) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]SignedViewChange)
	for k, v := range m {
		strMap[string(k)] = v
	}

	return json.Marshal(strMap)
}

type Ack struct {
}

type Message int
