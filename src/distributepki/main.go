package main

import (
	"bufio"
	"distributepki/keystore"
	"distributepki/util"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/rpc"
	"os"
	"os/exec"
	"os/signal"
	"pbft"
	"strings"

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
		log.Infof("    %v => %v", kp.Alias, kp.Key)
	}

	if *cluster {
		StartCluster(&initialKeyTable, &config, *debug)
	} else if *debug {
		StartRepl(&config)
	} else {
		StartNode(*id, &initialKeyTable, &config)
	}
}

func StartCluster(initialKeyTable *map[string]string, cluster *pbft.ClusterConfig, debug bool) {
	var nodeProcesses []*exec.Cmd
	for _, n := range cluster.Nodes {
		id := n.Id
		if debug {
			// TODO: append "debug" flag to cmd
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

func rpcTo(machineId int, hostName string, rpcName string, message interface{}, response interface{}, retries int) error {
	log.Infof("[REPL] Sending rpc to %d", machineId)
	rpcClient, err := rpc.DialHTTPPath("tcp", hostName, "/pbft")
	for nRetries := 0; err != nil && retries < nRetries; nRetries++ {
		rpcClient, err = rpc.DialHTTPPath("tcp", hostName, "/pbft")
	}
	if err != nil {
		return err
	}
	remoteCall := rpcClient.Go(rpcName, message, response, nil)
	result := <-remoteCall.Done
	if result.Error != nil {
		return result.Error
	}
	return nil

}

func StartRepl(cluster *pbft.ClusterConfig) {
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("> ")
		cmdString, _ := reader.ReadString('\n')
		cmdList := strings.Fields(cmdString)
		if len(cmdList) == 0 {
			continue
		}
		args := cmdList[1:]
		response := new(pbft.Ack)
		var primary pbft.NodeConfig
		for _, n := range cluster.Nodes {
			if n.Id == cluster.Primary.Id {
				primary = n
				break
			}
		}

		switch cmd := cmdList[0]; cmd {
		case "exit":
			return
		case "get":
			message := pbft.DebugMessage{
				Op: pbft.GET,
			}
			rpcTo(
				primary.Id,
				util.GetHostname(primary.Host, primary.Port),
				"PBFTNode.Debug",
				&message,
				response,
				10)
			fmt.Println(response)
		case "put":
			fmt.Println(args)
		case "slow":
			fmt.Println(args)
		case "down":
			fmt.Println(args)
		}
	}
}

func StartNode(id int, initialKeyTable *map[string]string, cluster *pbft.ClusterConfig) {
	var thisNode pbft.NodeConfig
	for _, n := range cluster.Nodes {
		if n.Id == id {
			thisNode = n
		}
	}
	log.Infof("Starting node %d (%s)...", id, util.GetHostname(thisNode.Host, thisNode.Port))

	ready := make(chan *pbft.PBFTNode)
	go pbft.StartNode(thisNode, *cluster, ready)
	node := <-ready
	if node != nil {
		log.Info("PBFT node started successfully!")
	} else {
		log.Fatal("PBFT node/cluster failed to start.")
	}

	keyNode := NewKeyNode(
		node,
		keystore.NewKeystore(keystore.NewKVStore(node, *initialKeyTable)),
	)
	if thisNode.Id == cluster.Primary.Id {
		keyNode.StartRPC(cluster.Primary.RpcPort)
	}

	<-node.Failure()
}
