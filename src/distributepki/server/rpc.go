package server

import (
	"distributepki/keystore"
	"net"
	"time"
)

type Create struct {
	Alias     keystore.Alias
	Key       keystore.Key
	Timestamp time.Time
	Client    net.Addr
	Signature string
}

type Update struct {
	Alias     keystore.Alias
	Update    keystore.KeyUpdate
	Timestamp time.Time
	Client    net.Addr
	Signature string
}

type Lookup struct {
	Alias     keystore.Alias
	Timestamp time.Time
	Client    net.Addr
	Signature string
}

type Ack struct {
	Success bool
}
