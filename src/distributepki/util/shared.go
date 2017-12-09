package util

import (
	"github.com/coreos/pkg/capnslog"

	"crypto/sha256"
	"errors"
	"net/rpc"
	"strconv"
	"time"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "pbft")
)

func GetHostname(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}

func SendRpc(hostName string, endpoint string, rpcFunction string, message interface{}, response interface{}, rpcRetries int, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 1000 * time.Millisecond
	}
	rpcClient, err := rpc.DialHTTPPath("tcp", hostName, endpoint)
	for nRetries := 0; err != nil && rpcRetries < nRetries; nRetries++ {
		rpcClient, err = rpc.DialHTTPPath("tcp", hostName, endpoint)
	}
	if err != nil {
		return err
	}
	remoteCall := rpcClient.Go(rpcFunction, message, response, nil)
	defer rpcClient.Close()
	select {
	case result := <-remoteCall.Done:
		if result.Error != nil {
			return result.Error
		}
	case <-time.After(timeout):
		return errors.New("Timed out")
	}
	return nil

}

func GenerateDigest(s string) ([sha256.Size]byte, error) {
	return sha256.Sum256([]byte(s)), nil
}
