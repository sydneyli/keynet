package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func TestLmao(t *testing.T) {
	toSign := KeyMapping{
		Alias:     "test@test.com",
		Key:       "testkey",
		Timestamp: time.Now().UnixNano() / 1000000,
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(toSign); err != nil {
		fmt.Println("FAIL1")
		return
	}
	resp, err := http.Post("http://localhost:8080/sign", "application/json;charset=utf8", buf)
	if err != nil {
		fmt.Println("FAIL2")
		return
	}
	resp_body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(string(resp_body))
}
