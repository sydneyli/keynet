package main

import (
	"bufio"
	"bytes"
	"distributepki/clientapi"
	"distributepki/keystore"
	"distributepki/util"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"pbft"
	"strconv"
	"strings"
	"time"
)

func sendDebugMessage(cluster *pbft.ClusterConfig, node *pbft.NodeConfig, msg pbft.DebugMessage) {
	err := util.SendRpc(
		util.GetHostname(node.Host, node.Port),
		cluster.Endpoint, // TODO: listen on a different endpoint for debugging
		"PBFTNode.Debug",
		&msg,
		nil,
		10,
		0,
	)
	if err != nil {
		log.Fatal(err)
	}
}

func extractNode(cluster *pbft.ClusterConfig, arg string) (*pbft.NodeConfig, error) {
	if i, err := strconv.Atoi(arg); err == nil {
		for _, n := range cluster.Nodes {
			if n.Id == pbft.NodeId(i) {
				return &n, nil
			}
		}
	} else {
		fmt.Println("Please specify which node you want to bring up!")
		return nil, err
	}
	return nil, errors.New("Couldn't find node with that id")
}

// directly send message to pbft node
func sendPbft(cluster *pbft.ClusterConfig, args []string, message pbft.DebugMessage) {
	if len(args) > 0 {
		if node, err := extractNode(cluster, args[0]); err == nil {
			sendDebugMessage(cluster, node, message)
			return
		}
	}
	fmt.Println("Please specify which node you want to send a debug message!")
}

// TODO (sydli): clean up below code (get better at go)
func doPut(cluster *pbft.ClusterConfig, node *pbft.NodeConfig, alias string, key string) {
	req, err := http.NewRequest("POST", "http://"+util.GetHostname(node.Host, node.ClientPort), bytes.NewBuffer([]byte(key)))
	if err != nil {
		log.Print(err)
		return
	}
	q := req.URL.Query()
	q.Add("name", alias)
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Errored when sending request to the server")
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	resp_body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(resp.Status)
	fmt.Println(string(resp_body))
	return
}

func doGet(cluster *pbft.ClusterConfig, node *pbft.NodeConfig, alias string) {
	// resp, err := http.Get("http://example.com/")
	req, err := http.NewRequest("GET", "http://"+util.GetHostname(node.Host, node.ClientPort), nil)
	if err != nil {
		log.Print(err)
		return
	}
	q := req.URL.Query()
	q.Add("name", alias)
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Errored when sending request to the server")
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	resp_body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(resp.Status)
	fmt.Println(string(resp_body))
	return
}

// curl
func extractPutParams(cluster *pbft.ClusterConfig, args []string,
	next func(*pbft.ClusterConfig, *pbft.NodeConfig, string, string)) {
	if len(args) > 0 {
		if node, err := extractNode(cluster, args[0]); err == nil {
			if len(args) > 2 {
				next(cluster, node, args[1], args[2])
			} else {
				fmt.Println("also supply what keys to put!")
			}
		}
	} else {
		fmt.Println("Please specify which node you want to send a debug message!")
	}
}

// curl
func extractGetParams(cluster *pbft.ClusterConfig, args []string,
	next func(*pbft.ClusterConfig, *pbft.NodeConfig, string)) {
	if len(args) > 0 {
		if node, err := extractNode(cluster, args[0]); err == nil {
			if len(args) > 1 {
				next(cluster, node, args[1])
			} else {
				fmt.Println("also supply what key to get!")
			}
		}
	} else {
		fmt.Println("Please specify which node you want to send a debug message!")
	}
}

// TODO (sydli): the below needs a massive cleanup
func StartDebugRepl(cluster *pbft.ClusterConfig) {
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(">> ")
		cmdString, _ := reader.ReadString('\n')
		cmdList := strings.Fields(cmdString)
		if len(cmdList) == 0 {
			continue
		}
		switch cmd := cmdList[0]; cmd {
		case "exit":
			return
		case "get":
			extractGetParams(cluster, cmdList[1:], doGet)
		case "put":
			extractPutParams(cluster, cmdList[1:], doPut)
		case "commit":
			req := clientapi.KeyOperation{
				OpCode: clientapi.OP_CREATE,
				Op:     clientapi.Create{"bingo", keystore.Key(""), time.Now(), nil},
			}
			req.SetDigest()

			var buf bytes.Buffer
			if err := gob.NewEncoder(&buf).Encode(req); err != nil {
				plog.Error(err)
				return
			}

			sendPbft(cluster, cmdList[1:], pbft.DebugMessage{
				Op:      pbft.PUT,
				Request: buf.String(),
			})
		case "up":
			sendPbft(cluster, cmdList[1:], pbft.DebugMessage{Op: pbft.UP})
		case "down":
			sendPbft(cluster, cmdList[1:], pbft.DebugMessage{Op: pbft.DOWN})
		}
	}
}
