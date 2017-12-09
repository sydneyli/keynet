package pbft

import (
	"errors"
)

// ** CHECKPOINTING ** //
func (n *PBFTNode) checkpointed(checkpoint CheckpointProof) {
	if checkpoint.Number.Before(n.lastCheckpoint.Number) {
		return
	}
	n.lastCheckpoint = checkpoint
	//n.Log("CHECKPOINT: %+v", n.lastCheckpoint)
	//flush pending checkpoints
	var stable []SlotId
	for slot, _ := range n.pendingCheckpoints {
		if slot.BeforeOrEqual(checkpoint.Number) {
			stable = append(stable, slot)
		}
	}
	for _, slot := range stable {
		delete(n.pendingCheckpoints, slot)
	}
	//flush the log
	var stableLog []SlotId
	for slot, _ := range n.log {
		if slot.BeforeOrEqual(checkpoint.Number) {
			stableLog = append(stableLog, slot)
		}
	}
	for _, slot := range stable {
		delete(n.log, slot)
	}
	// if checkpoint's is before my current seq... probably wanna apply it~
	// n.Log("%d, %d", n.sequenceNumber, checkpoint.Number.SeqNumber)
	if n.sequenceNumber < checkpoint.Number.SeqNumber {
		n.Snapshotted() <- &checkpoint.Snapshot
		n.sequenceNumber = checkpoint.Number.SeqNumber
	}
}

func (n *PBFTNode) isStable(checkpoint *Checkpoint) bool {
	info := n.pendingCheckpoints[checkpoint.Number]
	return checkpoint.Number.BeforeOrEqual(n.lastCheckpoint.Number) ||
		len(info.Proof) >= 2*(len(n.peermap)/3)+1
}

func (n *PBFTNode) handleCheckpointProof(proof *CheckpointProof) {
	if n.timeoutTimer != nil {
		n.timeoutTimer.Reset(n.getTimeout())
	}
	n.checkpointed(*proof)
	// if lastCHeckpoint
	// if n.isStable(checkpoint) {
	// 	return
	// }
	// if _, ok := n.pendingCheckpoints[checkpoint.Number]; !ok {
	// 	n.pendingCheckpoints[checkpoint.Number] = checkpointInfo{
	// 		number: checkpoint.Number,
	// 		proof:  make(map[NodeId]Checkpoint),
	// 	}
	// }
	// n.pendingCheckpoints[checkpoint.Number].proof[checkpoint.Node] = *checkpoint
	// if n.isStable(checkpoint) {
	// 	n.checkpointed(checkpoint.Number)
	// }
}

func (n *PBFTNode) handleCheckpoint(checkpoint *Checkpoint) {
	if n.isStable(checkpoint) {
		return
	}
	if _, ok := n.pendingCheckpoints[checkpoint.Number]; !ok {
		n.pendingCheckpoints[checkpoint.Number] = CheckpointProof{
			Number:   checkpoint.Number,
			Snapshot: checkpoint.Snapshot,
			Proof:    make(map[NodeId]Checkpoint),
		}
	}
	n.pendingCheckpoints[checkpoint.Number].Proof[checkpoint.Node] = *checkpoint
	if n.isStable(checkpoint) {
		n.checkpointed(n.pendingCheckpoints[checkpoint.Number])
	}
}

func (n *PBFTNode) handleRecvSnapshot(snap *snapshot) {
	checkpoint := Checkpoint{
		Number:   snap.number,
		Snapshot: snap.state,
		Node:     n.id,
	}
	n.handleCheckpoint(&checkpoint)
	// n.pendingCheckpoints[snap.number].Proof[n.id] = checkpoint
	go n.broadcast("PBFTNode.Checkpoint", &checkpoint, 0)
}

func (n *PBFTNode) tryCheckpoint() {
	if n.sequenceNumber%int(CHECKPOINT) != 0 {
		// no chekpointing!
		return
	}
	slot := SlotId{
		ViewNumber: n.viewNumber,
		SeqNumber:  n.sequenceNumber,
	}
	n.SnapshotRequested() <- slot
}

func (n PBFTNode) Snapshotted() chan *[]byte {
	return n.snapshottedChannel
}

func (n PBFTNode) SnapshotRequested() chan SlotId {
	return n.requestSnapshotChannel
}

func (n *PBFTNode) SnapshotReply(number SlotId, state []byte) {
	n.recvSnapshotChannel <- snapshot{
		number: number,
		state:  state,
	}
}

func (n PBFTNode) Checkpoint(req *Checkpoint, res *Ack) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.checkpointChannel <- req
	return nil
}

func (n PBFTNode) CheckpointProof(req *CheckpointProof, res *PPResponse) error {
	if n.down {
		return errors.New("I'm down")
	}
	n.checkpointProofChannel <- req
	res.SeqNumber = n.sequenceNumber
	return nil
}
