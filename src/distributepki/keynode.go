package main

import (
	"bytes"
	"distributepki/keystore"
	"distributepki/server"
	"distributepki/util"
	"encoding/gob"
	"encoding/json"
	"io/ioutil"
	// "net"
	"net/http"
	// "net/rpc"
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

const OP_CREATE = 0x01
const OP_UPDATE = 0x02
const OP_LOOKUP = 0x03

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
	kn.CreateKey(&server.Create{alias, keystore.Key("testkey"), time.Now(), nil, "sig"}, nil)
	<-time.NewTimer(time.Second * 2).C
	if found, key := kn.LookupKey(&server.Lookup{alias, time.Now(), nil, "sig"}, nil); found {
		plog.Infof("Lookup got key: %v for alias %v", key, alias)
	} else {
		plog.Infof("Lookup key failed for alias %v", alias)
	}
}

// is there a better way to bind this variable to the inner fn...?
func handlerWithContext(kn *KeyNode) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			var response keystore.Key
			alias := keystore.Alias(r.URL.Query().Get("name"))
			log.Print("HIIIII")
			log.Print(alias)
			if found, key := kn.LookupKey(&server.Lookup{alias, time.Now(), nil, "sig"}, nil); found {
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
				http.Error(w, "oops", http.StatusInternalServerError)
				return
			}
			key := keystore.Key(string(keybytes[:]))
			log.Print("HIIIII")
			log.Print(alias)
			log.Print(key)
			kn.CreateKey(&server.Create{alias, key, time.Now(), nil, "sig"}, nil)
			response := "sent"
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

	// server := rpc.NewServer()
	// server.Register(kn)
	// server.HandleHTTP("/public", "/debug/public")
	// l, e := net.Listen("tcp", util.GetHostname("", rpcPort))
	// if e != nil {
	// 	plog.Fatal("listen error:", e)
	// }
	// go http.Serve(l, nil)
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
	// go keyNode.testPropose()
	return &keyNode
}

func (kn *KeyNode) handleUpdates() {
	for {
		commit := <-kn.consensusNode.Committed()
		kn.handleCommit(commit)
	}
}

func (kn *KeyNode) handleCommit(operation *pbft.Operation) {
	decoder := gob.NewDecoder(bytes.NewReader([]byte(operation.Op)))
	switch operation.Opcode {
	case OP_CREATE:
		var create server.Create
		decoder.Decode(&create)
		plog.Infof("Commit create to keystore: %v", create)
		kn.store.CreateKey(create.Alias, create.Key)
	case OP_UPDATE:
		var update server.Update
		decoder.Decode(&update)
		plog.Infof("Commit update to keystore: %v", update)
		// TODO: Update keystore
	}
}

func (kn *KeyNode) CreateKey(args *server.Create, reply *server.Ack) error {

	plog.Info("Create Key: %+v", args)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		plog.Fatal(err)
	}

	kn.consensusNode.Propose(pbft.Operation{Opcode: OP_CREATE, Op: buf.String(), Timestamp: time.Now()})

	// go kn.handleUpdates()
	// TODO err := ks.CreateKey(args.Alias, args.Key, buf.String())
	// reply.Success = err == nil
	// return err
	// reply.Success = true
	return nil
}

func (kn *KeyNode) UpdateKey(args *server.Update, reply *server.Ack) error {

	plog.Info("Update Key: %+v", args)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		plog.Fatal(err)
	}

	kn.consensusNode.Propose(pbft.Operation{Opcode: OP_UPDATE, Op: buf.String(), Timestamp: time.Now()})

	// TODO err := ks.UpdateKey(args.Alias, args.Update, buf.String())
	// reply.Success = err == nil
	// return err
	// reply.Success = true
	return nil
}

func (kn *KeyNode) LookupKey(args *server.Lookup, reply *server.Ack) (bool, keystore.Key) {

	plog.Info("Lookup Key: %+v", args)

	// var buf bytes.Buffer
	// if err := gob.NewEncoder(&buf).Encode(args); err != nil {
	// 	plog.Fatal(err)
	// }

	// kn.consensusNode.Propose(pbft.Operation{Opcode: OP_LOOKUP, Op: buf.String()})

	//TODO err := ks.LookupKey(args.Alias, buf.String())
	// reply.Success = err == nil
	// return err
	// reply.Success = true
	return kn.store.LookupKey(args.Alias)
}
