package util

import (
	"net/rpc"
	"strconv"
	"time"
)

func GetHostname(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}

func SendRpc(hostName string, endpoint string, rpcFunction string, message interface{}, response interface{}, rpcRetries int, timeout time.Duration) error {

	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	rpcClient, err := rpc.DialHTTPPath("tcp", hostName, endpoint)
	for nRetries := 0; err != nil && rpcRetries < nRetries; nRetries++ {
		rpcClient, err = rpc.DialHTTPPath("tcp", hostName, endpoint)
	}
	if err != nil {
		return err
	}
	remoteCall := rpcClient.Go(rpcFunction, message, response, nil)
	select {
	case result := <-remoteCall.Done:
		if result.Error != nil {
			return result.Error
		}
	case <-time.After(timeout):
		rpcClient.Close()
	}
	return nil

}
