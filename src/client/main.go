package main

import (
	"distributepki/keystore"
	"distributepki/server"
	"distributepki/util"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"pbft"
	"strconv"

	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "client")
)

func main() {

	configFile := flag.String("config", "cluster.json", "PBFT configuration file")
	port := flag.Int("port", 9121, "HTTP API port")
	flag.Parse()

	plog.Infof("Reading cluster configuration from %s...", *configFile)
	configData, err := ioutil.ReadFile(*configFile)
	if err != nil {
		plog.Fatal(err)
	}

	var config pbft.ClusterConfig
	err = json.Unmarshal(configData, &config)
	if err != nil {
		plog.Fatal(err)
	}

	var primary pbft.NodeConfig
	for _, n := range config.Nodes {
		if n.Id == config.Primary.Id {
			primary = n
			break
		}
	}

	serveHttpApi(primary.Host, config.Primary.ClientPort, *port)
}

func serveHttpApi(primaryHost string, primaryClientPort int, port int) {
	plog.Info("API server going live at port " + strconv.Itoa(port) + "...")

	c, err := rpc.DialHTTPPath("tcp", util.GetHostname(primaryHost, primaryClientPort), "/public")
	for err != nil {
		c, err = rpc.DialHTTPPath("tcp", util.GetHostname(primaryHost, primaryClientPort), "/public")
	}

	plog.Info("API server connected to primary")
	srv := http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: &client{
			rpcClient: c,
		},
	}

	if err := srv.ListenAndServe(); err != nil {
		plog.Fatal(err)
	}
}

type client struct {
	rpcClient *rpc.Client
}

// TODO: actually wait for responses rather than just sending things off
func (h *client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathParam := string(r.RequestURI)[1:]
	switch {
	case r.Method == "PUT":

		alias := keystore.Alias(pathParam)
		val, err := ioutil.ReadAll(r.Body)
		if err != nil {
			plog.Printf("Failed to read on PUT (%v)\n", err)
			http.Error(w, "Failed on PUT", http.StatusBadRequest)
			return
		}

		// TODO: do digest/signing for client <-> keynode level
		args := &server.Create{
			Alias: alias,
			Key:   keystore.Key(val),
		}
		var response server.Ack

		err = h.rpcClient.Call("KeyNode.CreateKey", args, &response)
		if err != nil || !response.Success {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusNoContent)

	case r.Method == "GET":

		args := &server.Lookup{
			Alias: keystore.Alias(pathParam),
		}
		var response server.Ack

		err := h.rpcClient.Call("KeyNode.LookupKey", args, &response)
		if err != nil || !response.Success {
			http.Error(w, err.Error(), http.StatusNotFound)
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "PUT")
		w.Header().Add("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
