package main

import (
	"distributepki/keystore"
	"distributepki/util"
	"encoding/json"
	"flag"
	"io/ioutil"
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
	id := flag.Int("id", 1, "Node ID to start")
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
		initialKeys = append(initialKeys, pbft.KeyPair{Key: n.Key, Alias: util.GetHostname(n.Host, n.Port)})
	}

	initialKeyTable := make(map[string]string)
	for _, kp := range initialKeys {
		initialKeyTable[string(kp.Alias)] = string(kp.Key)
		log.Infof("    %v => %v", kp.Alias, kp.Key)
	}

	log.Infof("Starting node %d (%s)...", *id, util.GetHostname(thisNode.Host, thisNode.Port))

	ready := make(chan *pbft.PBFTNode)
	go pbft.StartNode(thisNode, config, ready)
	node := <-ready
	if node != nil {
		log.Info("PBFT node started successfully!")
	} else {
		log.Fatal("PBFT node/cluster failed to start.")
	}

	keyNode := NewKeyNode(
		node,
		keystore.NewKeystore(keystore.NewKVStore(node, initialKeyTable)),
	)
	if thisNode.Id == config.Primary.Id {
		keyNode.StartRPC(config.Primary.RpcPort)
	}

	<-node.Failure()
}
