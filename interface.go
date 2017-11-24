package main

type DistributedStore interface {
	Put(key, value string)
	Get(key string) (string, bool)

	MakeCheckpoint() (interface{}, error)
}

type ConsensusNode interface {
	ConfigChange(interface{})
	Failure() chan error

	Propose(string)
	Committed() chan *string

	GetCheckpoint() (interface{}, error)
}
