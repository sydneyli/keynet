package pbft

type DebugOp int

const (
	SLOW DebugOp = iota
	PUT
	GET
	DOWN
	UP
)

type DebugMessage struct {
	Op DebugOp
}
