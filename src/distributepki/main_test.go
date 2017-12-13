package main

import (
	"distributepki/util"
	"fmt"
	"math/rand"
	"pbft"
	"testing"
	"time"
)

// Some simple static tests on cluster.json config

// ** GLOBAL STATE ** //
// ( just for tests :D )
var r *rand.Rand // Rand for this package.
var cluster pbft.ClusterConfig

func assertEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a == b {
		return
	}
	if len(message) == 0 {
		message = fmt.Sprintf("%v != %v", a, b)
	}
	t.Fatal(message)
}

func startCluster(cluster *pbft.ClusterConfig, shutdown chan struct{}) {
	initialKeyTable := LoadInitialKeys("keys.json", cluster)
	StartCluster(&initialKeyTable, cluster, shutdown, true)
}

func getNode(node int) *pbft.NodeConfig {
	for _, n := range cluster.Nodes {
		if n.Id == pbft.NodeId(node) {
			return &n
		}
	}
	return nil
}

func init() {
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
	cluster = LoadConfig("cluster.json")
}

// Returns a shutdown channel for the cluster.
func setup() *chan struct{} {
	shutdownSignal := make(chan struct{})
	return &shutdownSignal
}

func teardown(shutdown *chan struct{}) {
	fmt.Println("TEARDOWN!")
	close(*shutdown)
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

func sendDebugMessageToNode(nodeId int, message pbft.DebugMessage) {
	node := getNode(nodeId)
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

// Simulates a "client get": broadcast to entire cluster and wait for f + 1 responses.
func clientGet(cluster *pbft.ClusterConfig, alias string) string {
	finished := make(chan string)
	for i := 0; i < 4; i++ {
		go func(i int) {
			_, result := doGet(cluster, &(cluster.Nodes[i]), alias)
			finished <- result
		}(i)
	}

	var result string
	for i := 0; i < (4/3)+1; i++ {
		result = <-finished
	}
	return result
}

func concurrentPutHelper(t *testing.T, concurrent int) {
	npeers := len(cluster.Nodes)
	aliases := make([]string, 0)
	keys := make([]string, 0)
	for i := 0; i < concurrent; i++ {
		go func() {
			putAt := r.Intn(npeers)
			alias, key := generateRandomAliasKeyPair()
			putStatus := doPut(&cluster, getNode(putAt+1), alias, key)
			aliases = append(aliases, alias)
			keys = append(keys, key)
			assertEqual(t, putStatus, "200 OK", "")
		}()
	}
	<-time.After(time.Duration(int(time.Second) / 2 * concurrent))
	for i := 0; i < concurrent; i++ {
		getFrom := r.Intn(npeers)
		alias := aliases[i]
		key := keys[i]
		status, result := doGet(&cluster, getNode(getFrom+1), alias)
		assertEqual(t, status, "200 OK", "")
		assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
	}
}

func testPutHelper(t *testing.T, putAt int, getAt int) (string, string) {
	leader := getNode(putAt)
	alias, key := generateRandomAliasKeyPair()
	putStatus := doPut(&cluster, leader, alias, key)
	assertEqual(t, putStatus, "200 OK", "")
	// wait for the command to commit...
	// (eventually have a way to report success to client)
	<-time.After(time.Duration(time.Second / 4))
	status, result := doGet(&cluster, getNode(getAt), alias)
	assertEqual(t, status, "200 OK", "")
	assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
	return alias, key
}

// Wrapper for tests.
func TestMain(m *testing.M) {
	shutdownChannel := setup()
	go func() {
		defer teardown(shutdownChannel)
		// Wait for cluster to start up
		<-time.After(time.Second / 4)
		m.Run()
	}()
	startCluster(&cluster, *shutdownChannel)
}

// ** TESTS START HERE ** //

func TestNormalOperation(t *testing.T) {
	// assuming node 1 is leader...
	testPutHelper(t, 1, 1)
	testPutHelper(t, 1, 4)
	testPutHelper(t, 3, 4)
	testPutHelper(t, 4, 1)
}

func TestViewChangeAfterCommit(t *testing.T) {
	alias, key := testPutHelper(t, 3, 4)
	testPutHelper(t, 2, 5)
	sendDebugMessageToNode(1, pbft.DebugMessage{Op: pbft.DOWN})
	<-time.After(3 * time.Second)
	status, result := doGet(&cluster, getNode(2), alias)
	assertEqual(t, status, "200 OK", "")
	assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
}

func TestCommitDuringViewChange(t *testing.T) {
	// 1. Take down leader and wait for view change
	sendDebugMessageToNode(1, pbft.DebugMessage{Op: pbft.DOWN})
	<-time.After(3 * time.Second)
	// 2. Try to persist update
	alias, key := testPutHelper(t, 3, 4)
	testPutHelper(t, 2, 3)
	// 3. Bring node back up
	sendDebugMessageToNode(1, pbft.DebugMessage{Op: pbft.UP})
	<-time.After(3 * time.Second)
	// 4. Did the node catch up properly?
	status, result := doGet(&cluster, getNode(1), alias)
	assertEqual(t, status, "200 OK", "")
	assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
}

func TestBackupCatchesUp(t *testing.T) {
	// 1. Take down a backup node.
	sendDebugMessageToNode(3, pbft.DebugMessage{Op: pbft.DOWN})
	// 2. Commit a value without them.
	alias, key := testPutHelper(t, 2, 4)
	// 3. Bring node back.
	sendDebugMessageToNode(3, pbft.DebugMessage{Op: pbft.UP})
	<-time.After(time.Second)
	// 4. Do they catch up?
	status, result := doGet(&cluster, getNode(3), alias)
	assertEqual(t, status, "200 OK", "")
	assertEqual(t, result, fmt.Sprintf("\"%s\"", key), "")
}

func TestConcurrentWrites(t *testing.T) {
	concurrentPutHelper(t, 5)
}

// ** BENCHMARKING ** //

// cluster config for cluster of any size~
func getCluster(size int) pbft.ClusterConfig {
	thisCluster := pbft.ClusterConfig{
		Nodes:            make([]pbft.NodeConfig, 0),
		AuthorityKeyFile: cluster.AuthorityKeyFile,
		Endpoint:         cluster.Endpoint,
	}
	canonicalNode := cluster.Nodes[0]
	id := 1
	port := 20000
	clientPort := 9020
	for i := 0; i < size; i++ {
		thisCluster.Nodes = append(cluster.Nodes, pbft.NodeConfig{
			ClientPort:     clientPort,
			Port:           port,
			Id:             pbft.NodeId(id),
			Host:           "localhost",
			PrivateKeyFile: canonicalNode.PrivateKeyFile,
			PublicKeyFile:  canonicalNode.PublicKeyFile,
			PassPhraseFile: canonicalNode.PassPhraseFile,
			// ^ everyone has the same keys cuz testing
		})
		clientPort++
		port++
		id++
	}
	return thisCluster
}

func doBenchmark(b *testing.B, cluster pbft.ClusterConfig, benchmark func(*testing.B, *pbft.ClusterConfig)) {
	shutdownChannel := setup()
	go func() {
		defer teardown(shutdownChannel)
		<-time.After(time.Second / 4)
		benchmark(b, &cluster)
	}()
	startCluster(&cluster, *shutdownChannel)
}

func benchmarkGets(b *testing.B, cluster *pbft.ClusterConfig) {
	alias, key := generateRandomAliasKeyPair()
	doPut(cluster, &(cluster.Nodes[0]), alias, key)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientGet(cluster, alias)
	}
}

func benchmarkPuts(b *testing.B, cluster *pbft.ClusterConfig) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alias, key := generateRandomAliasKeyPair()
		doPut(cluster, &(cluster.Nodes[0]), alias, key)
	}
}

func BenchmarkLocalGets(b *testing.B) {
	thisCluster := getCluster(5)
	doBenchmark(b, thisCluster, benchmarkGets)
}

func BenchmarkLocalPuts(b *testing.B) {
	thisCluster := getCluster(5)
	doBenchmark(b, thisCluster, benchmarkPuts)
}

func BenchmarkRemoteGets(b *testing.B) {
	remoteCluster := LoadConfig("prod_cluster.json")
	benchmarkGets(b, &remoteCluster)
}

func BenchmarkRemotePuts(b *testing.B) {
	remoteCluster := LoadConfig("prod_cluster.json")
	benchmarkPuts(b, &remoteCluster)
}

func BenchmarkRemotePuts(b *testing.B) {
	remoteCluster := LoadConfig("prod_cluster.json")
	benchmarkPuts(b, &remoteCluster)
}

//
func benchmarkWithIntermittentFailures(b *testing.B, cluster *pbft.ClusterConfig, benchmark func(*testing.B, *pbft.ClusterConfig)) {
	done := make(chan struct{})
	go func(done chan struct{}, cluster pbft.ClusterConfig) {
		ticker := time.NewTicker(time.Millisecond * 2000)
		down := 1

		select {
		case <-done:
		case <-ticker.C:
			sendDebugMessageToNode(down, pbft.DebugMessage{Op: pbft.UP})
			down = int(cluster.Nodes[rand.Intn(len(cluster.Nodes))].Id)
			sendDebugMessageToNode(down, pbft.DebugMessage{Op: pbft.DOWN})
		}
	}(done, *cluster)
	benchmark(b, cluster)
	defer func() { done <- struct{}{} }()
}

func BenchmarkPutDuringIntermittentFailures(b *testing.B) {
	thisCluster := getCluster(5)
	doBenchmark(b, thisCluster, func(*testing.B, *pbft.ClusterConfig) {
		benchmarkWithIntermittentFailures(b, cluster, benchmarkPuts)
	})
}

func BenchmarkGetDuringIntermittentFailures(b *testing.B) {
	thisCluster := getCluster(5)
	doBenchmark(b, thisCluster, func(*testing.B, *pbft.ClusterConfig) {
		benchmarkWithIntermittentFailures(b, cluster, benchmarkGets)
	})
}
