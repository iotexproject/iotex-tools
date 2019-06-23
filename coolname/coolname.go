package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/iotexproject/go-pkgs/crypto"
	"github.com/iotexproject/iotex-address/address"
)

type generatedAccount struct {
	Address    string `json:"address"`
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

var prefixDesired = "io1raullen"

func search(i int) {
	fmt.Printf("searching thread %d...\n", i)
	for {
		private, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr, err := address.FromBytes(private.PublicKey().Hash())
		if err != nil {
			panic(err)
		}
		newAccount := generatedAccount{
			Address:    addr.String(),
			PrivateKey: fmt.Sprintf("%x", private.Bytes()),
			PublicKey:  fmt.Sprintf("%x", private.PublicKey().Bytes()),
		}
		if strings.HasPrefix(newAccount.Address, prefixDesired) {
			fmt.Println(newAccount.Address, "\t", newAccount.PrivateKey)
		}
	}
}

func main() {
	for i := 0; i < 16; i++ {
		go search(i)
	}
	time.Sleep(24 * time.Hour)
}
