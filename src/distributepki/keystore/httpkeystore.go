package keystore

import (
	"distributepki/common"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"strconv"
)

// Handler for a http based keystore
type HttpKeystoreAPI struct {
	rpcClient *rpc.Client
}

func (h *HttpKeystoreAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathParam := string(r.RequestURI)[1:]
	switch {
	case r.Method == "PUT":

		alias := Alias(pathParam)
		val, err := ioutil.ReadAll(r.Body)
		if err != nil {
			plog.Printf("Failed to read on PUT (%v)\n", err)
			http.Error(w, "Failed on PUT", http.StatusBadRequest)
			return
		}

		args := &CreateRequest{
			Alias: alias,
			Key:   Key(val),
		}
		var response Ack

		err = h.rpcClient.Call("Keystore.CreateKeyRemote", args, &response)
		if err != nil || !response.Success {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusNoContent)

	case r.Method == "GET":

		args := &LookupRequest{
			Alias: Alias(pathParam),
		}
		var response LookupAck

		err := h.rpcClient.Call("Keystore.LookupKeyRemote", args, &response)
		if err != nil || !response.Success {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			w.Write([]byte(response.Key))
		}

	default:
		w.Header().Set("Allow", "PUT")
		w.Header().Add("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func ServeKeystoreHttpApi(primaryHost string, primaryRpcPort int, port int) {
	plog.Info("API server going live at port " + strconv.Itoa(port) + "...")

	client, err := rpc.DialHTTPPath("tcp", common.GetHostname(primaryHost, primaryRpcPort), "/public")
	for err != nil {
		client, err = rpc.DialHTTPPath("tcp", common.GetHostname(primaryHost, primaryRpcPort), "/public")
	}

	plog.Info("API server connected to primary")
	srv := http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: &HttpKeystoreAPI{
			rpcClient: client,
		},
	}

	if err := srv.ListenAndServe(); err != nil {
		plog.Fatal(err)
	}
}
