package keystore

import (
	"fmt"
	"sync"
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

type Keystore struct {
	keys *sync.Map
}

func NewKeystore(initial *map[string]string) *Keystore {
	syncMap := sync.Map{}
	for k, v := range *initial {
		// TODO: figure out a better way to do this
		syncMap.Store(k, v)
	}
	return &Keystore{keys: &syncMap}
}

func (ks *Keystore) CreateKey(alias Alias, key Key, clientMessage string) error {
	/*
		if _, ok := ks.store.Get(string(alias)); ok {
			return AliasAlreadyExists(alias)
		}
		// TODO: make sure that key is valid key format here
		plog.Infof("Create Key: %v for Alias: %v", key, alias)
		ks.store.Propose("Create", clientMessage)
	*/
	return nil
}

func (ks *Keystore) UpdateKey(alias Alias, update KeyUpdate, clientMessage string) error {
	/*
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
		ks.store.Propose("Update", clientMessage)
	*/
	return nil
}

func (ks *Keystore) LookupKey(alias Alias, clientMessage string) error {
	/*
		plog.Infof("Keystore Query for Alias: %v", alias)
		ks.store.Propose("Lookup", clientMessage)
	*/
	return nil

	// TODO: change once read-only transactions implemented
	/*
		if v, ok := ks.store.Get(string(alias)); ok {
			key := Key(v)
			plog.Infof("Keystore Lookup for Alias: %v returned %v", alias, key)
			return key, nil
		} else {
			return Key(""), AliasNotFoundError(alias)
		}
	*/
}

func verifyKeyUpdate(update KeyUpdate, oldKey Key) bool {
	// TODO: check signature here using old key
	return true
}
