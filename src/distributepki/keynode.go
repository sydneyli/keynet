package main

import (
	"bytes"
	"distributepki/keystore"
	"distributepki/server"
	"distributepki/util"
	"encoding/gob"
	"net"
	"net/http"
	"net/rpc"
	"pbft"
)

type KeyNode struct {
	consensusNode *pbft.PBFTNode
	store         *keystore.Keystore
}

const OP_CREATE = 0x01
const OP_UPDATE = 0x02
const OP_LOOKUP = 0x03

func NewKeyNode(node *pbft.PBFTNode, store *keystore.Keystore) KeyNode {
	n := KeyNode{
		consensusNode: node,
		store:         store,
	}

	return n
}

func (kn *KeyNode) StartRPC(rpcPort int) {
	server := rpc.NewServer()
	server.Register(kn)
	server.HandleHTTP("/public", "/debug/public")
	l, e := net.Listen("tcp", util.GetHostname("", rpcPort))
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

func (kn *KeyNode) CreateKey(args *server.Create, reply *server.Ack) error {

	log.Info("Create Key: %+v", args)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		log.Fatal(err)
	}

	kn.consensusNode.Propose(OP_CREATE, buf.String())

	// TODO err := ks.CreateKey(args.Alias, args.Key, buf.String())
	// reply.Success = err == nil
	// return err
	reply.Success = true
	return nil
}

func (kn *KeyNode) UpdateKey(args *server.Update, reply *server.Ack) error {

	log.Info("Update Key: %+v", args)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		log.Fatal(err)
	}

	kn.consensusNode.Propose(OP_UPDATE, buf.String())

	// TODO err := ks.UpdateKey(args.Alias, args.Update, buf.String())
	// reply.Success = err == nil
	// return err
	reply.Success = true
	return nil
}

func (kn *KeyNode) LookupKey(args *server.Lookup, reply *server.Ack) error {

	log.Info("Lookup Key: %+v", args)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(args); err != nil {
		log.Fatal(err)
	}

	kn.consensusNode.Propose(OP_LOOKUP, buf.String())

	//TODO err := ks.LookupKey(args.Alias, buf.String())
	// reply.Success = err == nil
	// return err
	reply.Success = true
	return nil
}
