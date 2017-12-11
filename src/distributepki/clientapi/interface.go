package clientapi

import (
	"bytes"
	"crypto/sha256"
	"distributepki/keystore"
	"encoding/base64"
	"encoding/json"
	"net"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"golang.org/x/crypto/openpgp"
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
	Timestamp int64
	Client    net.Addr
	Signature keystore.Signature // Signature of authority
}

type keyMapping struct {
	Alias     string
	Key       string
	Timestamp int64
}

func (c Create) SignatureValid(authorities openpgp.EntityList) bool {
	nosig := keyMapping{
		Alias:     string(c.Alias),
		Key:       string(c.Key),
		Timestamp: c.Timestamp,
	}
	signature, _ := base64.StdEncoding.DecodeString(string(c.Signature))
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(nosig); err != nil {
		plog.Error("Couldn't encode as json!")
		return false
	}
	if _, err := sig.Write(signature); err != nil {
		plog.Error("Not even a valid signature...")
		return false
	}
	_, err := openpgp.CheckDetachedSignature(authorities, &buf, &sig)
	if err != nil {
		plog.Error("Not signed by correct auth")
		return false
	}
	return true
}

type Update struct {
	Alias     keystore.Alias
	Key       keystore.Key
	Timestamp int64
	Client    net.Addr
	Signature keystore.Signature
}

// XXX: This doesn't validate correctly, for some reason...
func (u Update) SignatureValid(previousKey keystore.Key) bool {
	keyring, readErr := openpgp.ReadArmoredKeyRing(strings.NewReader(string(previousKey)))
	if readErr != nil {
		plog.Error("Couldn't read previous key!")
		return false
	}
	nosig := keyMapping{
		Alias:     string(u.Alias),
		Key:       string(u.Key),
		Timestamp: u.Timestamp,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(nosig); err != nil {
		plog.Error("Couldn't encode as json!")
		return false
	}
	_, err := openpgp.CheckArmoredDetachedSignature(keyring, &buf, strings.NewReader(string(u.Signature)))
	if err != nil {
		plog.Error(err)
		plog.Error("Not signed by correct auth")
		return false
	}
	return true
}

type Lookup struct {
	Alias  keystore.Alias
	Client net.Addr
}

type Ack struct {
	Success bool
}

type CreateJSON struct {
	Alias     string
	Key       string
	Timestamp int64
	Signature string
}

func (createJSON *CreateJSON) ToCreate() (Create, error) {
	return Create{
		Alias:     keystore.Alias(createJSON.Alias),
		Key:       keystore.Key(createJSON.Key),
		Timestamp: createJSON.Timestamp,
		Client:    nil,
		Signature: keystore.Signature(createJSON.Signature),
	}, nil
}

type UpdateJSON struct {
	Alias     string
	Key       string
	Timestamp int64
	Signature string
}

func (updateJSON *UpdateJSON) ToUpdate() (Update, error) {
	return Update{
		Alias:     keystore.Alias(updateJSON.Alias),
		Key:       keystore.Key(updateJSON.Key),
		Timestamp: updateJSON.Timestamp,
		Signature: keystore.Signature(updateJSON.Signature),
	}, nil
}
