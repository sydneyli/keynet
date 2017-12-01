// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keystore

import (
	"bytes"
	"distributepki/common"
	"encoding/gob"
	"encoding/json"
	"log"
	"sync"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/snap"
)

// a key-value store backed by raft
type Kvstore struct {
	mu            sync.RWMutex
	kvStore       map[string]string // current committed key-value pairs
	consensusNode common.ConsensusNode
}

type kv struct {
	Key string
	Val string
}

func NewKVStore(node common.ConsensusNode, initialStore map[string]string) *Kvstore {
	if initialStore == nil {
		initialStore = make(map[string]string)
	}
	s := &Kvstore{kvStore: initialStore, consensusNode: node}

	// XXX: this is necessary for Raft, but not PBFT right now, fix it to work with both
	// replay log into key-value map
	// s.readCommits(node)
	// read commits from cluster into kvStore map until error
	// go s.readCommits(node)
	return s
}

func (s *Kvstore) Get(key string) (string, bool) {
	s.mu.RLock()
	v, ok := s.kvStore[key]
	s.mu.RUnlock()
	return v, ok
}

func (s *Kvstore) Put(k, v string) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(kv{k, v}); err != nil {
		log.Fatal(err)
	}
	s.consensusNode.Propose(buf.String())
}

func (s *Kvstore) readCommits(node common.ConsensusNode) {
	for data := range node.Committed() {
		if data == nil {
			// done replaying log; new data incoming
			// OR signaled to load snapshot
			// snapshot, err := s.snapshotter.Load()
			snapshot, err := node.GetCheckpoint()
			if err == snap.ErrNoSnapshot {
				return
			}
			if err != nil && err != snap.ErrNoSnapshot {
				log.Panic(err)
			}
			r_snapshot, ok := snapshot.(*raftpb.Snapshot)
			if !ok {
				log.Panic("Incorrectly-typed snapshot")
			}
			log.Printf("loading snapshot at term %d and index %d", r_snapshot.Metadata.Term, r_snapshot.Metadata.Index)
			if err := s.recoverFromSnapshot(r_snapshot.Data); err != nil {
				log.Panic(err)
			}
			continue
		}

		var dataKv kv
		dec := gob.NewDecoder(bytes.NewBufferString(*data))
		if err := dec.Decode(&dataKv); err != nil {
			log.Fatalf("raftexample: could not decode message (%v)", err)
		}
		s.mu.Lock()
		s.kvStore[dataKv.Key] = dataKv.Val
		s.mu.Unlock()
	}
	if err, ok := <-node.Failure(); ok {
		log.Fatal(err)
	}
}

func (s *Kvstore) MakeCheckpoint() (interface{}, error) {
	return s.getSnapshot()
}

func (s *Kvstore) getSnapshot() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(s.kvStore)
}

func (s *Kvstore) recoverFromSnapshot(snapshot []byte) error {
	var store map[string]string
	if err := json.Unmarshal(snapshot, &store); err != nil {
		return err
	}
	s.mu.Lock()
	s.kvStore = store
	s.mu.Unlock()
	return nil
}
