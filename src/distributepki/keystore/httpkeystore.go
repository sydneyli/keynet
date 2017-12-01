package keystore

import (
	"distributepki/common"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/coreos/etcd/raft/raftpb"
)

// Handler for a http based keystore
type HttpKeystoreAPI struct {
	keystore      Keystore
	consensusNode common.ConsensusNode
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

		if error := h.keystore.CreateKey(alias, Key(val)); error != nil {
			http.Error(w, error.Error(), http.StatusBadRequest)
		}
		// Optimistic-- no waiting for ack from raft. Value is not yet
		// committed so a subsequent GET on the key may return old value
		w.WriteHeader(http.StatusNoContent)
	case r.Method == "GET":
		alias := Alias(pathParam)
		if key, error := h.keystore.LookupKey(alias); error == nil {
			w.Write([]byte(key))
		} else {
			http.Error(w, error.Error(), http.StatusNotFound)
		}
	case r.Method == "POST":
		url, err := ioutil.ReadAll(r.Body)
		if err != nil {
			plog.Printf("Failed to read on POST (%v)\n", err)
			http.Error(w, "Failed on POST", http.StatusBadRequest)
			return
		}

		nodeId, err := strconv.ParseUint(pathParam, 0, 64)
		if err != nil {
			plog.Printf("Failed to convert ID for conf change (%v)\n", err)
			http.Error(w, "Failed on POST", http.StatusBadRequest)
			return
		}

		cc := raftpb.ConfChange{
			Type:    raftpb.ConfChangeAddNode,
			NodeID:  nodeId,
			Context: url,
		}
		h.consensusNode.ConfigChange(cc)

		// As above, optimistic that raft will apply the conf change
		w.WriteHeader(http.StatusNoContent)
	case r.Method == "DELETE":
		nodeId, err := strconv.ParseUint(pathParam, 0, 64)
		if err != nil {
			plog.Printf("Failed to convert ID for conf change (%v)\n", err)
			http.Error(w, "Failed on DELETE", http.StatusBadRequest)
			return
		}

		cc := raftpb.ConfChange{
			Type:   raftpb.ConfChangeRemoveNode,
			NodeID: nodeId,
		}
		h.consensusNode.ConfigChange(cc)

		// As above, optimistic that raft will apply the conf change
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT")
		w.Header().Add("Allow", "GET")
		w.Header().Add("Allow", "POST")
		w.Header().Add("Allow", "DELETE")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func ServeKeystoreHttpApi(keystore Keystore, node common.ConsensusNode, port int) {
	plog.Info(":" + strconv.Itoa(port))
	srv := http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: &HttpKeystoreAPI{
			keystore:      keystore,
			consensusNode: node,
		},
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			plog.Fatal(err)
		}
	}()

	// exit when consensus cluster goes down
	if err, ok := <-node.Failure(); ok {
		plog.Fatal(err)
	}
}
