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
	"distributepki/common"
	"distributepki/keystore"
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
	kvport := flag.Int("port", 9121, "HTTP server port")
	//join := flag.Bool("join", false, "join an existing cluster")
	configFile := flag.String("config", "cluster.json", "PBFT configuration file")
	keystoreFile := flag.String("keys", "keys.json", "Initial keys in store")
	flag.Parse()

	log.Infof("Reading cluster configuration from %s...", *configFile)
	configData, err := ioutil.ReadFile(*configFile)
	logFatal(err)

	var config pbft.ClusterConfig
	err = json.Unmarshal(configData, &config)
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

	log.Infof("Reading initial keys from %s...", *keystoreFile)

	keyData, err := ioutil.ReadFile(*keystoreFile)
	logFatal(err)

	var initialKeys []pbft.KeyPair
	err = json.Unmarshal(keyData, &initialKeys)
	logFatal(err)

	for _, n := range config.Nodes {
		initialKeys = append(initialKeys, pbft.KeyPair{Key: n.Key, Alias: n.Hostname})
	}

	initialKeyTable := make(map[string]string)
	for _, kp := range initialKeys {
		initialKeyTable[string(kp.Alias)] = string(kp.Key)
		log.Infof("    %v => %v", kp.Alias, kp.Key)
	}

	log.Infof("Starting node %d (%s)...", *id, thisNode.Hostname)

	ready := make(chan common.ConsensusNode)
	go pbft.StartNode(thisNode, config, ready)

	node := <-ready
	if node != nil {
		log.Info("Node started successfully!")
	} else {
		log.Fatal("Node/cluster failed to start.")
	}

	/* // Unneeded for now
	var checkpointFn = func() ([]byte, error) {
		checkpoint, err := kvs.MakeCheckpoint()
		byte_checkpoint, ok := checkpoint.([]byte)
		if !ok {
			log.Panic("Checkpoint not a []byte array")
		}
		return byte_checkpoint, err
	}
	*/

	log.Info("hsdfelldasf")
	store := keystore.NewKeystore(keystore.NewKVStore(node, initialKeyTable))
	log.Info("helldasf")
	keystore.ServeKeystoreHttpApi(store, node, *kvport)
	log.Info("hellsdfa")
}
