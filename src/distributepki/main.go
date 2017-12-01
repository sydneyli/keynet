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

package main

import (
	//"distributepki/etcdraft"
	//"distributepki/keystore"
	//"distributepki/pbft"
	"flag"
	"pbft"
	//"fmt"
	//"strings"
	"encoding/json"
	"io/ioutil"

	"github.com/coreos/pkg/capnslog"
)

var (
	log = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "main")
)

func logFatal(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func main() {
	//cluster := flag.String("cluster", "http://127.0.0.1:9021", "comma separated cluster peers")
	id := flag.Int("id", 1, "Node ID to start")
	//kvport := flag.Int("port", 9121, "key-value server port")
	//join := flag.Bool("join", false, "join an existing cluster")
	config_file := flag.String("config", "cluster.json", "PBFT configuration file")
	keystore_file := flag.String("keys", "keys.json", "Initial keys in store")
	flag.Parse()

	log.Infof("Reading cluster configuration from %s...", *config_file)
	config_data, err := ioutil.ReadFile(*config_file)
	logFatal(err)

	var config pbft.ClusterConfig
	err = json.Unmarshal(config_data, &config)
	logFatal(err)

	var thisNode pbft.NodeConfig
	for _, n := range config.Nodes {
		log.Infof("    [Node %d] %s", n.Id, n.Hostname)
		if n.Primary {
			log.Infof("        **PRIMARY**")
		}
		if n.Id == *id {
			thisNode = n
		}
	}

	log.Infof("Reading initial keys from %s...", *keystore_file)

	key_data, err := ioutil.ReadFile(*keystore_file)
	logFatal(err)

	var initial_keys []pbft.KeyPair
	err = json.Unmarshal(key_data, &initial_keys)
	logFatal(err)

	for _, n := range config.Nodes {
		initial_keys = append(initial_keys, pbft.KeyPair{Key: n.Key, Alias: n.Hostname})
	}

	for _, kp := range initial_keys {
		log.Infof("    %v => %v", kp.Alias, kp.Key)
	}

	log.Infof("Starting node %d (%s)...", *id, thisNode.Hostname)

	ready := make(chan bool)
	go pbft.StartNode(thisNode, config, ready)

	if <-ready {
		log.Info("Node started successfully!")
	} else {
		log.Fatal("Node/cluster failed to start.")
	}

	for true {
	}

	/*
		// raft provides a commit stream for the proposals from the http api
		var kvs *keystore.Kvstore
		var checkpointFn = func() ([]byte, error) {
			checkpoint, err := kvs.MakeCheckpoint()
			byte_checkpoint, ok := checkpoint.([]byte)
			if !ok {
				log.Panic("Checkpoint not a []byte array")
			}
			return byte_checkpoint, err
		}
		raftNode, ready := etcdraft.NewRaftNode(
			*id,
			strings.Split(*cluster, ","),
			*join,
			checkpointFn)

		<-ready
		kvs = keystore.NewKVStore(raftNode)

		// the key-value http handler will propose updates to raft
		keystore.ServeKeystoreHttpApi(keystore.NewKeystore(kvs), raftNode, *kvport)
	*/
}
