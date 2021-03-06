package keystore

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "Keystore")
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
	keys *map[Alias]Key
	mux  sync.RWMutex
}

func NewKeystore(initial *map[string]string) *Keystore {
	syncMap := make(map[Alias]Key)
	for k, v := range *initial {
		// TODO: figure out a better way to do this
		syncMap[Alias(k)] = Key(v)
	}
	return &Keystore{keys: &syncMap}
}

func (ks *Keystore) CreateKey(alias Alias, key Key) error {
	plog.Infof("Storing key for alias %v", alias)
	ks.mux.Lock()
	defer ks.mux.Unlock()
	(*ks.keys)[alias] = key
	return nil
}

func (ks *Keystore) UpdateKey(alias Alias, key Key) error {
	plog.Infof("Updating key for alias %v", alias)
	ks.mux.Lock()
	defer ks.mux.Unlock()
	(*ks.keys)[alias] = key
	return nil
}

func (ks *Keystore) LookupKey(alias Alias) (bool, Key) {
	/*
		plog.Infof("Keystore Query for Alias: %v", alias)
		ks.store.Propose("Lookup", clientMessage)
	*/
	plog.Info("Load ", alias)
	ks.mux.RLock()
	defer ks.mux.RUnlock()
	if key, ok := (*ks.keys)[alias]; ok {
		return true, key
	}
	return false, Key("")

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

func (s *Keystore) GetSnapshot() ([]byte, error) {
	// s.mu.Lock()
	// defer s.mu.Unlock()
	return json.Marshal(*s.keys)
}

func (ks *Keystore) ApplySnapshot(snapshot *[]byte) error {
	// s.mu.Lock()
	// defer s.mu.Unlock()
	ks.mux.Lock()
	defer ks.mux.Unlock()
	var keys map[Alias]Key
	if err := json.Unmarshal(*snapshot, &keys); err != nil {
		return err
	}
	ks.keys = &keys
	return nil
}
