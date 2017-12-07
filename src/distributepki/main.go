package main

import (
	"bufio"
	"distributepki/keystore"
	"distributepki/util"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"pbft"
	"strconv"
	"strings"
	"time"

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
	cluster := flag.Bool("cluster", false, "Bootstrap entire cluster")
	debug := flag.Bool("debug", false, "with cluster flag, enables debugging. without cluster flag, starts debugging repl")
	id := flag.Int("id", 1, "Node ID to start")
	keystoreFile := flag.String("keys", "keys.json", "Initial keys in store")

	flag.Parse()

	log.Infof("Reading cluster configuration from %s...", *configFile)
	configData, err := ioutil.ReadFile(*configFile)
	logFatal(err)

	var config pbft.ClusterConfig
	err = json.Unmarshal(configData, &config)
	logFatal(err)

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
		log.Debugf("    %v => %v", kp.Alias, kp.Key)
	}

	if *cluster {
		StartCluster(&initialKeyTable, &config, *debug)
	} else if *debug {
		StartRepl(&config)
	} else {
		StartNode(pbft.NodeId(*id), &initialKeyTable, &config)
	}
}

func StartCluster(initialKeyTable *map[string]string, cluster *pbft.ClusterConfig, debug bool) {
	var nodeProcesses []*exec.Cmd
	for _, n := range cluster.Nodes {
		id := n.Id
		if debug {
			// TODO (sydli): Take debug flag into account
		}
		cmd := exec.Command("./distributepki", "-id", fmt.Sprintf("%d", id))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = "."
		err := cmd.Start()
		if err != nil {
			log.Fatal(err)
			continue
		}
		nodeProcesses = append(nodeProcesses, cmd)
	}
	// If we get Ctrl+C, kill all subprocesses
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	for i, cmd := range nodeProcesses {
		log.Infof("Kill process %d", i)
		cmd.Process.Kill()
	}
}

// TODO (sydli): the below needs a massive cleanup
func StartRepl(cluster *pbft.ClusterConfig) {
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("> ")
		cmdString, _ := reader.ReadString('\n')
		cmdList := strings.Fields(cmdString)
		if len(cmdList) == 0 {
			continue
		}
		// args := cmdList[1:]
		response := new(pbft.Ack)
		var message pbft.DebugMessage
		var id pbft.NodeId
		switch cmd := cmdList[0]; cmd {
		case "exit":
			return
		case "commit":
			message = pbft.DebugMessage{
				Op: pbft.PUT,
				Request: pbft.ClientRequest{
					Opcode:    OP_CREATE,
					Op:        "bingo",
					Id:        time.Now().UnixNano(),
					Timestamp: time.Now(),
					Client:    nil,
				},
			}
			id = cluster.Primary.Id
		case "up":
			if i, err := strconv.Atoi(cmdList[1]); err == nil {
				id = pbft.NodeId(i)
			} else {
				fmt.Println("Please specify which node you want to bring up!")
				return
			}
			message = pbft.DebugMessage{
				Op: pbft.UP,
			}
		case "down":
			if i, err := strconv.Atoi(cmdList[1]); err == nil {
				id = pbft.NodeId(i)
			} else {
				fmt.Println("Please specify which node you want to take down!")
				return
			}
			message = pbft.DebugMessage{
				Op: pbft.DOWN,
			}
		}
		var to pbft.NodeConfig
		for _, n := range cluster.Nodes {
			if n.Id == id {
				to = n
				break
			}
		}
		err := util.SendRpc(
			util.GetHostname(to.Host, to.Port),
			cluster.Endpoint, //TODO: listen on a different endpoint for debugging
			"PBFTNode.Debug",
			&message,
			response,
			10,
			0,
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(response)
	}
}

func StartNode(id pbft.NodeId, initialKeyTable *map[string]string, cluster *pbft.ClusterConfig) {
	var thisNode pbft.NodeConfig
	for _, n := range cluster.Nodes {
		if n.Id == id {
			thisNode = n
		}
	}

	store := keystore.NewKeystore(initialKeyTable)

	log.Infof("Starting node %d (%s)...", id, util.GetHostname(thisNode.Host, thisNode.Port))
	node := SpawnKeyNode(thisNode, cluster, store)
	if node == nil {
		log.Fatalf("Node %d failed to start.", id)
		return
	}
	log.Infof("Node %d started successfully!", id)

	if thisNode.Id == cluster.Primary.Id {
		node.StartRPC(cluster.Primary.RpcPort)
	}

	<-node.consensusNode.Failure()
}
