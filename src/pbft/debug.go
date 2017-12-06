package pbft

type DebugOp int

const (
	PUT DebugOp = iota
	DOWN
	UP
)

type DebugMessage struct {
	Op      DebugOp
	Request ClientRequest
}

func (n *PBFTNode) Debug(req *DebugMessage, res *Ack) error {
	res.Success = true
	n.debugChannel <- req
	return nil
}

func (n PBFTNode) blockUntilUp() {
	for {
		msg := <-n.debugChannel
		if msg.Op == UP {
			n.Log("BACK UP")
			return
		}
	}
}

func (n PBFTNode) handleDebug(debug *DebugMessage) {
	switch op := debug.Op; op {
	case PUT:
		n.Log("PUT %+v", debug.Request)
		n.handleClientRequest(&debug.Request)
	case DOWN:
		n.Log("DOWN")
		n.blockUntilUp()
	case UP:
	}
}
