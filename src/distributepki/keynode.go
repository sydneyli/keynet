package main

import (
	"bytes"
	"crypto/sha256"
	"distributepki/clientapi"
	"distributepki/keystore"
	"distributepki/util"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"pbft"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	"golang.org/x/crypto/openpgp"
)

type KeyRequest struct {
	op     *clientapi.KeyOperation
	writer *http.ResponseWriter
}

type KeyNode struct {
	consensusNode   *pbft.PBFTNode
	store           *keystore.Keystore
	pendingRequests *sync.Map
	logger          *capnslog.PackageLogger
}

var keyring openpgp.EntityList

func SpawnKeyNode(config pbft.NodeConfig, cluster *pbft.ClusterConfig, store *keystore.Keystore) *KeyNode {
	// Hook in mock authority~
	var err error
	keyring, err = util.ReadPgpKeyFile(cluster.AuthorityKeyFile)
	if err != nil {
		return nil
	}
	node := pbft.StartNode(config, *cluster)
	if node == nil {
		return nil
	}

	keyNode := KeyNode{
		consensusNode:   node,
		store:           store,
		pendingRequests: &sync.Map{},
		logger:          capnslog.NewPackageLogger("github.com/sydli/distributePKI", fmt.Sprintf("Keynode [Node %v]", node.Id())),
	}

	go keyNode.handleUpdates()
	go keyNode.serveKeyRequests()
	return &keyNode
}

func (kn *KeyNode) serveKeyRequests() {
	for request := range kn.consensusNode.KeyRequest {
		s := "testkey"
		request.Reply <- &s
		// TODO: finish implementing
		// if v, ok := kn.store.Get(request.Hostname); ok {
		// 	request.Reply <- v
		// } else {
		// 	request.Reply <- nil
		// }
	}
}

// is there a better way to bind this variable to the inner fn...?
func handlerWithContext(kn *KeyNode) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if kn.consensusNode.Down() {
			kn.logger.Info("I'm down")
			return
		}

		switch r.Method {

		// TODO: have client send signed KeyOperations directly, rather than generating them here
		case "GET":
			var response keystore.Key
			alias := keystore.Alias(r.URL.Query().Get("name"))
			op := clientapi.KeyOperation{
				OpCode: clientapi.OP_LOOKUP,
				Op:     clientapi.Lookup{alias, nil},
			}
			op.SetDigest()

			if found, key := kn.LookupKey(&op, nil); found {
				response = key
			} else {
				http.Error(w, "Key not found", http.StatusNotFound)
				return
			}
			jsonBody, err := json.Marshal(response)
			if err != nil {
				http.Error(w, "Error converting results to json",
					http.StatusInternalServerError)
				return
			}
			w.Write(jsonBody)
		case "POST":
			bodyData, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading body.", http.StatusInternalServerError)
				return
			}

			createJSON := clientapi.CreateJSON{}
			err = json.Unmarshal(bodyData, &createJSON)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var create clientapi.Create
			create, err = createJSON.ToCreate()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			op := clientapi.KeyOperation{
				OpCode: clientapi.OP_CREATE,
				Op:     create,
			}
			op.SetDigest()
			if error := kn.CreateKey(&op, nil); error == nil {
				kn.waitForCommit(&op, &w)
			} else {
				http.Error(w, error.Error(), http.StatusBadRequest)
				return
			}
		case "PUT":
			bodyData, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading key.", http.StatusInternalServerError)
				return
			}

			updateJSON := clientapi.UpdateJSON{}
			err = json.Unmarshal(bodyData, &updateJSON)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var update clientapi.Update
			update, err = updateJSON.ToUpdate()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			op := clientapi.KeyOperation{
				OpCode: clientapi.OP_UPDATE,
				Op:     update,
			}
			op.SetDigest()
			if error := kn.UpdateKey(&op, nil); error == nil {
				kn.waitForCommit(&op, &w)
			} else {
				http.Error(w, error.Error(), http.StatusBadRequest)
				return
			}
		}
	}
}

func (kn *KeyNode) waitForCommit(op *clientapi.KeyOperation, w *http.ResponseWriter) {
	responseChan := make(chan string)
	kn.pendingRequests.Store(op.Digest, responseChan)
	kn.logger.Infof("Store pending request with digest: %v", op.Digest)

	var error string
	select {
	case error = <-responseChan:
	case <-time.After(time.Second * 10):
		error = "Timeout on wait for Commit"
	}

	if error == "" {
		writeJSON("", w)
	} else {
		http.Error(*w, error, http.StatusInternalServerError)
	}
	kn.pendingRequests.Delete(op.Digest)
}

func writeJSON(msg string, w *http.ResponseWriter) {
	if jsonBody, err := json.Marshal(msg); err == nil {
		(*w).Write(jsonBody)
	} else {
		http.Error(*w, "Error converting results to json",
			http.StatusInternalServerError)
	}
}

func (kn *KeyNode) respondPendingRequest(digest [sha256.Size]byte, response string) {
	if request, ok := kn.pendingRequests.Load(digest); !ok {
		kn.logger.Debugf("No corresponding http request for operation digest %v", digest)
		return
	} else {
		request.(chan string) <- response
	}
}

func (kn *KeyNode) StartClientServer(httpPort int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlerWithContext(kn))
	log.Fatal(http.ListenAndServe(util.GetHostname("", httpPort), mux))
}

func (kn *KeyNode) handleUpdates() {
	for {
		select {
		case commit := <-kn.consensusNode.Committed():
			kn.handleCommit(commit)
		case request := <-kn.consensusNode.SnapshotRequested():
			kn.handleSnapshotRequest(request)
		case snapshot := <-kn.consensusNode.Snapshotted():
			kn.handleSnapshot(snapshot)
		}
	}
}

func (kn *KeyNode) handleSnapshotRequest(slot pbft.SlotId) {
	snapshot, err := kn.store.GetSnapshot()
	if err != nil {
		kn.logger.Errorf("oh no, couldnt snapshot")
	}
	kn.consensusNode.SnapshotReply(slot, snapshot)
}

func (kn *KeyNode) handleSnapshot(snapshot *[]byte) {
	kn.store.ApplySnapshot(snapshot)
	// var keyOp clientapi.KeyOperation
	// err := gob.NewDecoder(bytes.NewReader([]byte(*operation))).Decode(&keyOp)
	// if err != nil {
	// 	kn.logger.Error(err)
	// 	return
	// }

	// kn.logger.Infof("Commit operation: %+v", keyOp)

	// // XXX: do we need to check the signature of the operation again here?
	// // Or do we assume that since it's committed and we're a non-faulty
	// // node, we can apply it.
	// switch keyOp.OpCode {
	// case clientapi.OP_CREATE:
	// 	create, ok := keyOp.Op.(clientapi.Create)
	// 	if !ok {
	// 		kn.logger.Error("Operation not a Create (handleCommit)")
	// 		return
	// 	}
	// 	kn.logger.Infof("Commit create to keystore: %v", create)
	// 	kn.store.CreateKey(create.Alias, create.Key)
	// case clientapi.OP_UPDATE:
	// 	update, ok := keyOp.Op.(clientapi.Update)
	// 	if !ok {
	// 		kn.logger.Error("Operation not a Update (handleCommit)")
	// 		return
	// 	}
	// 	kn.logger.Infof("Commit update to keystore: %v", update)
	// 	// TODO: Update keystore
	// }
}

func (kn *KeyNode) handleCommit(operation *string) {
	if operation == nil {
		return
	}

	var keyOp clientapi.KeyOperation
	err := gob.NewDecoder(bytes.NewReader([]byte(*operation))).Decode(&keyOp)
	if err != nil {
		kn.logger.Error(err)
		kn.respondPendingRequest(keyOp.Digest, err.Error())
		return
	}

	switch keyOp.OpCode {
	case clientapi.OP_CREATE:
		create, ok := keyOp.Op.(clientapi.Create)
		if !ok {
			error := "Operation not a Create (handleCommit)"
			kn.logger.Error(error)
			kn.respondPendingRequest(keyOp.Digest, error)
			return
		}
		kn.logger.Info("Commiting create to keystore")
		kn.store.CreateKey(create.Alias, create.Key)

	case clientapi.OP_UPDATE:
		update, ok := keyOp.Op.(clientapi.Update)
		if !ok {
			error := "Operation not a Update (handleCommit)"
			kn.logger.Error(error)
			kn.respondPendingRequest(keyOp.Digest, error)
			return
		}
		kn.logger.Info("Commiting update to keystore")
		kn.store.UpdateKey(update.Alias, update.Key)
	}

	kn.respondPendingRequest(keyOp.Digest, "")
}

func (kn *KeyNode) CreateKey(args *clientapi.KeyOperation, reply *clientapi.Ack) error {

	if !args.DigestValid() {
		errMsg := "Operation digest is invalid (CreateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	if args.OpCode != clientapi.OP_CREATE {
		errMsg := "Incorrect opcode value (CreateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	create, ok := args.Op.(clientapi.Create)
	if !ok {
		errMsg := "Operation not a Create (CreateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	if !create.SignatureValid(keyring) {
		errMsg := "Signature is invalid (CreateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	if ok, _ := kn.store.LookupKey((args.Op.(clientapi.Create)).Alias); ok {
		errMsg := "Key already exists for user (CreateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	kn.logger.Infof("Create Key: %+v", create)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		kn.logger.Error(err)
		return err
	}

	str := buf.String()
	kn.consensusNode.Propose(&str)

	if reply != nil {
		reply.Success = true
	}
	return nil
}

func (kn *KeyNode) UpdateKey(args *clientapi.KeyOperation, reply *clientapi.Ack) error {

	if !args.DigestValid() {
		errMsg := "Operation digest is invalid (UpdateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	if args.OpCode != clientapi.OP_UPDATE {
		errMsg := "Incorrect opcode value (UpdateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	update, ok := args.Op.(clientapi.Update)
	if !ok {
		errMsg := "Operation not an Update (UpdateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	ok, key := kn.store.LookupKey((args.Op.(clientapi.Update)).Alias)
	if !ok {
		errMsg := "Key does not exist for user (UpdateKey)"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}
	if !update.SignatureValid(key) {
		errMsg := "Update message not signed by client!"
		kn.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	kn.logger.Infof("Update Key: %+v", update)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		kn.logger.Error(err)
		return err
	}

	str := buf.String()
	kn.consensusNode.Propose(&str)

	if reply != nil {
		reply.Success = true
	}
	return nil
}

func (kn *KeyNode) LookupKey(args *clientapi.KeyOperation, reply *clientapi.Ack) (bool, keystore.Key) {
	// TODO: verify operation signature
	if !args.DigestValid() {
		kn.logger.Error("Operation digest is invalid (LookupKey)")
		return false, keystore.Key("")
	}

	if args.OpCode != clientapi.OP_LOOKUP {
		kn.logger.Error("Incorrect opcode value (LookupKey)")
		return false, keystore.Key("")
	}

	lookup, ok := args.Op.(clientapi.Lookup)
	if !ok {
		kn.logger.Error("Operation not a Lookup (LookupKey)")
		return false, keystore.Key("")
	}
	kn.logger.Infof("Lookup Key: %+v", lookup)
	return kn.store.LookupKey(lookup.Alias)
}
