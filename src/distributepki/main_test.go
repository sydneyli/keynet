package main

import (
	"distributepki/util"
	"fmt"
	"math/rand"
	"pbft"
	"testing"
	"time"
)

func assertEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a == b {
		return
	}
	if len(message) == 0 {
		message = fmt.Sprintf("%v != %v", a, b)
	}
	t.Fatal(message)
}

func startCluster(cluster *pbft.ClusterConfig, shutdown chan bool) {
	initialKeyTable := LoadInitialKeys("keys.json", cluster)
	StartCluster(&initialKeyTable, cluster, shutdown, true)
}

func getNode(cluster *pbft.ClusterConfig, node int) *pbft.NodeConfig {
	for _, n := range cluster.Nodes {
		if n.Id == pbft.NodeId(node) {
			return &n
		}
	}
	return nil
}

var r *rand.Rand // Rand for this package.

func init() {
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func RandomString(strlen int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := ""
	for i := 0; i < strlen; i++ {
		index := r.Intn(len(chars))
		result += chars[index : index+1]
	}
	return result
}

func generateRandomAliasKeyPair() (string, string) {
	return RandomString(10), RandomString(10)
}

func testPutHelper(t *testing.T, cluster *pbft.ClusterConfig, putAt int, getAt int) (string, string) {
	leader := getNode(cluster, putAt)
	alias, key := generateRandomAliasKeyPair()
	putStatus := doPut(cluster, leader, alias, key)
	assertEqual(t, putStatus, "200 OK", "")
	// wait for the command to commit...
	// (eventually have a way to report success to client)
	<-time.After(time.Duration(1 * time.Second))
	status, result := doGet(cluster, getNode(cluster, getAt), alias)
	assertEqual(t, status, "200 OK", "")
	assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
	return alias, key
}

func TestNormalOperation(t *testing.T) {
	shutdownSignal := make(chan bool)
	cluster := LoadConfig("cluster.json")
	go func(cluster *pbft.ClusterConfig, shutdownSignal chan bool) {
		<-time.After(time.Second)
		defer func() { shutdownSignal <- true }()
		// assuming node 1 is leader...
		testPutHelper(t, cluster, 1, 1)
		testPutHelper(t, cluster, 1, 4)
		testPutHelper(t, cluster, 3, 4)
		testPutHelper(t, cluster, 4, 1)
	}(&cluster, shutdownSignal)
	startCluster(&cluster, shutdownSignal)
}

func sendDebugMessageToNode(cluster *pbft.ClusterConfig, nodeId int, message pbft.DebugMessage) {
	node := getNode(cluster, nodeId)
	err := util.SendRpc(
		util.GetHostname(node.Host, node.Port),
		cluster.Endpoint, // TODO: listen on a different endpoint for debugging
		"PBFTNode.Debug",
		&message,
		nil,
		10,
		0,
	)
	if err != nil {
		log.Fatal(err)
	}
}

func TestViewChangeAfterCommit(t *testing.T) {
	shutdownSignal := make(chan bool)
	cluster := LoadConfig("cluster.json")
	go func(cluster *pbft.ClusterConfig, shutdownSignal chan bool) {
		<-time.After(time.Second)
		defer func() { shutdownSignal <- true }()
		// assuming node 1 is leader...
		alias, key := testPutHelper(t, cluster, 3, 4)
		sendDebugMessageToNode(cluster, 1, pbft.DebugMessage{Op: pbft.DOWN})
		<-time.After(6 * time.Second)
		status, result := doGet(cluster, getNode(cluster, 2), alias)
		assertEqual(t, status, "200 OK", "")
		assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
	}(&cluster, shutdownSignal)
	startCluster(&cluster, shutdownSignal)
}

func TestSingleViewChange(t *testing.T) {
	shutdownSignal := make(chan bool)
	cluster := LoadConfig("cluster.json")
	go func(cluster *pbft.ClusterConfig, shutdownSignal chan bool) {
		<-time.After(time.Second)
		defer func() { shutdownSignal <- true }()
		// assuming node 1 is leader...
		sendDebugMessageToNode(cluster, 1, pbft.DebugMessage{Op: pbft.DOWN})
		<-time.After(6 * time.Second)
		testPutHelper(t, cluster, 3, 4)
	}(&cluster, shutdownSignal)
	startCluster(&cluster, shutdownSignal)
}
