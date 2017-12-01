package keystore

import (
	"distributepki/common"
	"fmt"
	"time"

	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "keystore")
)

type Alias string
type Signature string
type Key string

type KeyUpdate struct {
	alias      Alias
	key        Key
	expiration time.Time
}

type AliasNotFoundError string

func (e AliasNotFoundError) Error() string {
	return fmt.Sprintf("Alias '%v' not found.", string(e))
}

type AliasAlreadyExists string

func (e AliasAlreadyExists) Error() string {
	return fmt.Sprintf("Alias '%v' already exists.", string(e))
}

type SignatureMismatch struct {
	update KeyUpdate
	oldKey Key
}

func (e SignatureMismatch) Error() string {
	return "Signature Mismatch"
}

// Public Key Store backed by DistributedStore
type Keystore struct {
	store common.DistributedStore
}

func NewKeystore(s common.DistributedStore) Keystore {
	return Keystore{store: s}
}

func (ks *Keystore) CreateKey(alias Alias, key Key) error {
	if _, ok := ks.store.Get(string(alias)); ok {
		return AliasAlreadyExists(alias)
	}
	// TODO: make sure that key is valid key format here
	plog.Infof("Create Key: %v for Alias: %v", key, alias)
	ks.store.Put(string(alias), string(key))
	return nil
}

func (ks *Keystore) UpdateKey(alias Alias, update KeyUpdate) error {
	var oldKey Key
	if val, ok := ks.store.Get(string(alias)); !ok {
		return AliasNotFoundError(alias)
	} else {
		oldKey = Key(val)
	}

	if !verifyKeyUpdate(update, oldKey) {
		return SignatureMismatch{update, oldKey}
	}

	plog.Infof("Update Alias: %v set Key: %v ", alias, update.key)
	ks.store.Put(string(alias), string(update.key))
	return nil
}

func (ks *Keystore) LookupKey(alias Alias) (Key, error) {
	if v, ok := ks.store.Get(string(alias)); ok {
		key := Key(v)
		plog.Infof("Keystore Lookup for Alias: %v returned %v", alias, key)
		return key, nil
	} else {
		return Key(""), AliasNotFoundError(alias)
	}
}

func verifyKeyUpdate(update KeyUpdate, oldKey Key) bool {
	// TODO: check signature here using old key
	return true
}
