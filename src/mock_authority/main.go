package main

import (
	"golang.org/x/crypto/openpgp"
	// 	"golang.org/x/crypto/openpgp/armor"
	// 	"golang.org/x/crypto/openpgp/packet"

	// "crypto/rsa"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type KeyMapping struct {
	Alias     string
	Key       string
	Timestamp uint64
}

type keyMappingSigned struct {
	Alias     string
	Key       string
	Timestamp uint64
	Signature []byte
}

func ReadPgpKeyFile(path string) (openpgp.EntityList, error) {
	keyfile, e := os.Open(path)
	if e != nil {
		var empty openpgp.EntityList
		return empty, e
	}

	list, readErr := openpgp.ReadArmoredKeyRing(keyfile)
	if readErr != nil {
		var empty openpgp.EntityList
		return empty, readErr
	}
	return list, nil
}

var thisEntity *openpgp.Entity

func handler(w http.ResponseWriter, r *http.Request) {
	// 1. decode from json
	var toSign KeyMapping
	if r.Body == nil {
		fmt.Print("EFT 1")
		http.Error(w, "Please send a request body", 400)
		return
	}
	err := json.NewDecoder(r.Body).Decode(&toSign)
	if err != nil {
		fmt.Printf("yoooo\n")
		res, _ := ioutil.ReadAll(r.Body)
		fmt.Printf("lmao %s\n", string(res))
		http.Error(w, err.Error(), 400)
		return
	}
	fmt.Printf(" hiii %+v", toSign)
	// 2. verify key mapping (mock server, so we dont do this)
	// 3. sign key mapping
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(toSign); err != nil {
		return
	}

	if thisEntity == nil {
		return
	}

	err = openpgp.DetachSign(&sig, thisEntity, &buf, nil)
	if err != nil {
		return
	}
	signed := keyMappingSigned{
		Alias:     toSign.Alias,
		Key:       toSign.Key,
		Timestamp: toSign.Timestamp,
		Signature: sig.Bytes(),
	}
	if err := json.NewEncoder(w).Encode(signed); err != nil {
		return
	}
}

func main() {
	// 1. read in all the key info
	hostEntityList, err := ReadPgpKeyFile("keys/private.key")
	if err != nil {
		fmt.Printf("reading private key: %s\n", err.Error())
	} else if len(hostEntityList) != 1 {
		fmt.Errorf("reading private key: expected only 1 host PGP entity, got %d", len(hostEntityList))
	}
	thisEntity = hostEntityList[0]
	fmt.Printf("%d host primary key: %+v\n", len(hostEntityList), thisEntity.PrimaryKey)

	phrase, err := ioutil.ReadFile("keys/passphrase.txt")
	if err != nil {
		fmt.Printf("reading passphrase: %s\n", err.Error())
	}
	passphrase := strings.TrimSpace(string(phrase))

	err = thisEntity.PrivateKey.Decrypt([]byte(passphrase))
	if err != nil {
		fmt.Printf("decrypting private key: %s\n", err.Error())
	}

	// 2. spin up server
	http.HandleFunc("/sign", handler)
	http.ListenAndServe(":8080", nil)
}
