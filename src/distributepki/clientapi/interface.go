package clientapi

import (
	"bytes"
	"crypto/sha256"
	"distributepki/keystore"
	"encoding/json"
	"net"
	"time"

	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "clientapi")
)

const OP_CREATE = 0x01
const OP_UPDATE = 0x02
const OP_LOOKUP = 0x03

type KeyOperation struct {
	OpCode int
	Op     interface{}
	Digest [sha256.Size]byte
}

func (ko *KeyOperation) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ko); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (ko *KeyOperation) SetDigest() {
	ko.Digest = [sha256.Size]byte{}
	d, err := ko.generateDigest()
	if err != nil {
		plog.Fatal("Error setting KeyOperation digest: " + err.Error())
	} else {
		ko.Digest = d
	}
}

func (ko *KeyOperation) DigestValid() bool {
	currentDigest := ko.Digest
	ko.Digest = [sha256.Size]byte{}
	d, err := ko.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating KeyOperation digest for validity: " + err.Error())
		return false
	} else {
		ko.Digest = currentDigest
		return d == currentDigest
	}
}

type Create struct {
	Alias     keystore.Alias
	Key       keystore.Key
	Timestamp time.Time
	Client    net.Addr
}

type Update struct {
	Alias     keystore.Alias
	Key       keystore.Key
	Timestamp time.Time
	Client    net.Addr
	Signature keystore.Signature
}

type Lookup struct {
	Alias     keystore.Alias
	Timestamp time.Time
	Client    net.Addr
}

type Ack struct {
	Success bool
}
