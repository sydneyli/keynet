package util

import (
	"net/rpc"
	"strconv"
)

func GetHostname(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}

const rpcRetries int = 10

func SendRpc(hostName string, endpoint string, rpcFunction string, message interface{}, response interface{}) error {
	rpcClient, err := rpc.DialHTTPPath("tcp", hostName, endpoint)
	for nRetries := 0; err != nil && rpcRetries < nRetries; nRetries++ {
		rpcClient, err = rpc.DialHTTPPath("tcp", hostName, endpoint)
	}
	if err != nil {
		return err
	}
	remoteCall := rpcClient.Go(rpcFunction, message, response, nil)
	result := <-remoteCall.Done
	if result.Error != nil {
		return result.Error
	}
	return nil

}
