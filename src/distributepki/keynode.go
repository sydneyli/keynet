package main

import (
	"bytes"
	"distributepki/clientapi"
	"distributepki/keystore"
	"distributepki/util"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"pbft"
	"time"

	"github.com/coreos/pkg/capnslog"
)

var (
	plog = capnslog.NewPackageLogger("github.com/sydli/distributePKI", "keynode")
)

type KeyNode struct {
	consensusNode *pbft.PBFTNode
	store         *keystore.Keystore
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

func (kn *KeyNode) testPropose() {
	alias := keystore.Alias("testalias")
	<-time.NewTimer(time.Second * 2).C

	op := clientapi.KeyOperation{
		OpCode: clientapi.OP_CREATE,
		Op:     clientapi.Create{alias, keystore.Key("testkey"), time.Now(), nil},
	}
	op.SetDigest()

	kn.CreateKey(&op, nil)
	<-time.NewTimer(time.Second * 2).C

	lookup := clientapi.KeyOperation{
		OpCode: clientapi.OP_LOOKUP,
		Op:     clientapi.Lookup{alias, time.Now(), nil},
	}
	lookup.SetDigest()

	if found, key := kn.LookupKey(&lookup, nil); found {
		plog.Infof("Lookup got key: %v for alias %v", key, alias)
	} else {
		plog.Infof("Lookup key failed for alias %v", alias)
	}
}

// is there a better way to bind this variable to the inner fn...?
func handlerWithContext(kn *KeyNode) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {

		// TODO: have client send signed KeyOperations directly, rather than generating them here
		case "GET":
			var response keystore.Key
			alias := keystore.Alias(r.URL.Query().Get("name"))
			op := clientapi.KeyOperation{
				OpCode: clientapi.OP_LOOKUP,
				Op:     clientapi.Lookup{alias, time.Now(), nil},
			}
			op.SetDigest()

			plog.Infof("GET DIGEST: %+v", op.Digest)

			if found, key := kn.LookupKey(&op, nil); found {
				response = key
			} else {
				http.Error(w, "Key not found", http.StatusBadRequest)
			}
			jsonBody, err := json.Marshal(response)
			if err != nil {
				http.Error(w, "Error converting results to json",
					http.StatusInternalServerError)
			}
			w.Write(jsonBody)
		case "POST":
			alias := keystore.Alias(r.URL.Query().Get("name"))
			keybytes, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading key.", http.StatusInternalServerError)
				return
			}
			key := keystore.Key(string(keybytes[:]))
			op := clientapi.KeyOperation{
				OpCode: clientapi.OP_CREATE,
				Op:     clientapi.Create{alias, key, time.Now(), nil},
			}
			op.SetDigest()

			plog.Infof("POST DIGEST: %+v", op.Digest)

			kn.CreateKey(&op, nil)
			response := "Key creation submitted."
			jsonBody, err := json.Marshal(response)
			if err != nil {
				http.Error(w, "Error converting results to json",
					http.StatusInternalServerError)
			}
			w.Write(jsonBody)
		}
	}
}

func (kn *KeyNode) StartClientServer(rpcPort int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlerWithContext(kn))
	log.Fatal(http.ListenAndServe(util.GetHostname("", rpcPort), mux))
}

func SpawnKeyNode(config pbft.NodeConfig, cluster *pbft.ClusterConfig, store *keystore.Keystore) *KeyNode {
	node := pbft.StartNode(config, *cluster)
	if node == nil {
		return nil
	}

	keyNode := KeyNode{
		consensusNode: node,
		store:         store,
	}

	go keyNode.handleUpdates()
	go keyNode.serveKeyRequests()
	return &keyNode
}

func (kn *KeyNode) handleUpdates() {
	for {
		commit := <-kn.consensusNode.Committed()
		kn.handleCommit(commit)
	}
}

func (kn *KeyNode) handleCommit(operation *string) {

	var keyOp clientapi.KeyOperation
	err := gob.NewDecoder(bytes.NewReader([]byte(*operation))).Decode(&keyOp)
	if err != nil {
		plog.Error(err)
		return
	}

	plog.Infof("Commit operation: %+v", keyOp)

	// XXX: do we need to check the signature of the operation again here?
	// Or do we assume that since it's committed and we're a non-faulty
	// node, we can apply it.
	switch keyOp.OpCode {
	case clientapi.OP_CREATE:
		create, ok := keyOp.Op.(clientapi.Create)
		if !ok {
			plog.Error("Operation not a Create (handleCommit)")
			return
		}
		plog.Infof("Commit create to keystore: %v", create)
		kn.store.CreateKey(create.Alias, create.Key)
	case clientapi.OP_UPDATE:
		update, ok := keyOp.Op.(clientapi.Update)
		if !ok {
			plog.Error("Operation not a Update (handleCommit)")
			return
		}
		plog.Infof("Commit update to keystore: %v", update)
		// TODO: Update keystore
	}
}

func (kn *KeyNode) CreateKey(args *clientapi.KeyOperation, reply *clientapi.Ack) error {

	// TODO: verify operation signature
	if !args.DigestValid() {
		errMsg := "Operation digest is invalid (CreateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}

	if args.OpCode != clientapi.OP_CREATE {
		errMsg := "Incorrect opcode value (CreateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}

	create, ok := args.Op.(clientapi.Create)
	if !ok {
		errMsg := "Operation not a Create (CreateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}
	plog.Infof("Create Key: %+v", create)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		plog.Error(err)
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

	// TODO: verify operation signature
	if !args.DigestValid() {
		errMsg := "Operation digest is invalid (UpdateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}

	if args.OpCode != clientapi.OP_UPDATE {
		errMsg := "Incorrect opcode value (UpdateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}

	update, ok := args.Op.(clientapi.Update)
	if !ok {
		errMsg := "Operation not an Update (UpdateKey)"
		plog.Error(errMsg)
		return errors.New(errMsg)
	}
	plog.Infof("Update Key: %+v", update)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		plog.Error(err)
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
		plog.Error("Operation digest is invalid (LookupKey)")
		return false, keystore.Key("")
	}

	if args.OpCode != clientapi.OP_LOOKUP {
		plog.Error("Incorrect opcode value (LookupKey)")
		return false, keystore.Key("")
	}

	lookup, ok := args.Op.(clientapi.Lookup)
	if !ok {
		plog.Error("Operation not a Lookup (LookupKey)")
		return false, keystore.Key("")
	}
	plog.Infof("Lookup Key: %+v", lookup)
	return kn.store.LookupKey(lookup.Alias)
}
