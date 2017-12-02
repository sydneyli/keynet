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
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"net/rpc"
	"pbft"

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
	configFile := flag.String("config", "cluster.json", "PBFT configuration file")

	pbftNode := flag.Bool("node", true, "[true] Start full PBFT node; [false] Start client")
	id := flag.Int("id", 1, "Node ID to start")
	keystoreFile := flag.String("keys", "keys.json", "Initial keys in store")
	clientport := flag.Int("clientport", 9121, "HTTP server port")

	flag.Parse()

	log.Infof("Reading cluster configuration from %s...", *configFile)
	configData, err := ioutil.ReadFile(*configFile)
	logFatal(err)

	var config pbft.ClusterConfig
	err = json.Unmarshal(configData, &config)
	logFatal(err)

	var store keystore.Keystore
	if *pbftNode {

		var thisNode pbft.NodeConfig
		for _, n := range config.Nodes {
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
			initialKeys = append(initialKeys, pbft.KeyPair{Key: n.Key, Alias: common.GetHostname(n.Host, n.Port)})
		}

		initialKeyTable := make(map[string]string)
		for _, kp := range initialKeys {
			initialKeyTable[string(kp.Alias)] = string(kp.Key)
			log.Infof("    %v => %v", kp.Alias, kp.Key)
		}

		log.Infof("Starting node %d (%s)...", *id, common.GetHostname(thisNode.Host, thisNode.Port))

		ready := make(chan common.ConsensusNode)
		go pbft.StartNode(thisNode, config, ready)

		node := <-ready
		if node != nil {
			log.Info("Node started successfully!")
		} else {
			log.Fatal("Node/cluster failed to start.")
		}
		store = keystore.NewKeystore(keystore.NewKVStore(node, initialKeyTable))

		if config.Primary.Id == thisNode.Id {
			rpc.Register(&store)
			server := rpc.NewServer()
			server.HandleHTTP("/public", "/dbg1")
			l, e := net.Listen("tcp", common.GetHostname("", config.Primary.RpcPort))
			if e != nil {
				log.Fatal("listen error:", e)
			}
			go http.Serve(l, nil)
		}

		<-node.Failure()

	} else {
		var primary pbft.NodeConfig
		for _, n := range config.Nodes {
			if n.Id == config.Primary.Id {
				primary = n
				break
			}
		}

		keystore.ServeKeystoreHttpApi(primary.Host, config.Primary.RpcPort, *clientport)
	}
}
