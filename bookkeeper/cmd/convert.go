// Copyright (c) 2019 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package cmd

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	// MultisendABI defines the ABI of multisend contract
	MultisendABI = `[
		{
			"constant": false,
			"inputs": [
				{
					"name": "recipients",
					"type": "address[]"
				},
				{
					"name": "amounts",
					"type": "uint256[]"
				},
				{
					"name": "payload",
					"type": "string"
				}
			],
			"name": "sendCoin",
			"outputs": [],
			"payable": true,
			"stateMutability": "payable",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "_limit",
					"type": "uint256"
				}
			],
			"name": "setLimit",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [],
			"name": "withdraw",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [],
			"name": "owner",
			"outputs": [
				{
					"name": "",
					"type": "address"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "_minTips",
					"type": "uint256"
				}
			],
			"name": "setMinTips",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [],
			"name": "limit",
			"outputs": [
				{
					"name": "",
					"type": "uint256"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "token",
					"type": "address"
				},
				{
					"name": "recipients",
					"type": "address[]"
				},
				{
					"name": "amounts",
					"type": "uint256[]"
				},
				{
					"name": "payload",
					"type": "string"
				}
			],
			"name": "sendToken",
			"outputs": [],
			"payable": true,
			"stateMutability": "payable",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [],
			"name": "minTips",
			"outputs": [
				{
					"name": "",
					"type": "uint256"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "newOwner",
					"type": "address"
				}
			],
			"name": "transferOwnership",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"name": "_minTips",
					"type": "uint256"
				},
				{
					"name": "_limit",
					"type": "uint256"
				}
			],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "constructor"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"name": "from",
					"type": "address"
				},
				{
					"indexed": true,
					"name": "to",
					"type": "address"
				},
				{
					"indexed": false,
					"name": "value",
					"type": "uint256"
				}
			],
			"name": "Transfer",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": false,
					"name": "_token",
					"type": "address"
				},
				{
					"indexed": false,
					"name": "_totalAmount",
					"type": "uint256"
				},
				{
					"indexed": false,
					"name": "_tips",
					"type": "uint256"
				},
				{
					"indexed": false,
					"name": "_payload",
					"type": "string"
				}
			],
			"name": "Receipt",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": false,
					"name": "_owner",
					"type": "address"
				},
				{
					"indexed": false,
					"name": "_balance",
					"type": "uint256"
				}
			],
			"name": "Withdraw",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"name": "previousOwner",
					"type": "address"
				},
				{
					"indexed": true,
					"name": "newOwner",
					"type": "address"
				}
			],
			"name": "OwnershipTransferred",
			"type": "event"
		}]`
	// sendCoin is the api name to call
	sendCoin = "sendCoin"
)

// ConvertCmd converts csv to bytecode
var ConvertCmd = &cobra.Command{
	Use:   "convert csv",
	Short: "Convert csv to bytecode",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		total, bytecode, err := convertToBytecode(args[0], inputUnit)
		if err == nil {
			totalInFloat := new(big.Float).SetInt(total)
			fmt.Printf("Total Amount: %f IOTX or %d Rau\n", totalInFloat.Quo(totalInFloat, OneIOTX), total)
			fmt.Printf("Byte Code: %s\n", hex.EncodeToString(bytecode))
		}
		return err
	},
}

var (
	outputFile string
	inputUnit  string
	format     string
)

func init() {
	ConvertCmd.Flags().StringVar(&inputUnit, "input-unit", "Rau", "output file")
}

func convertToBytecode(csvFile string, unit string) (totalAmount *big.Int, bytecode []byte, err error) {
	switch strings.ToLower(unit) {
	case "rau":
		unit = "Rau"
	case "iotx":
		unit = "IOTX"
	default:
		err = errors.Errorf("invalid unit type %s", unit)
		return
	}
	f, err := os.Open(csvFile)
	if err != nil {
		return
	}
	defer f.Close()
	totalAmount = big.NewInt(0)
	var addrs []common.Address
	var amounts []*big.Int
	reader := csv.NewReader(f)
	var record []string
	for {
		record, err = reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
		addrs = append(addrs, common.HexToAddress(record[0]))
		amount, ok := new(big.Float).SetString(record[1])
		if !ok {
			err = errors.Errorf("failed to parse record %s", record[1])
			return
		}
		if unit == "IOTX" {
			amount = amount.Mul(amount, OneIOTX)
		}
		amountInInt, _ := amount.Int(nil)
		totalAmount = totalAmount.Add(totalAmount, amountInInt)
		amounts = append(amounts, amountInInt)
	}
	if len(amounts) == 0 {
		err = errors.Errorf("no records in csv file %s", csvFile)
		return
	}
	multisendABI, err := abi.JSON(strings.NewReader(MultisendABI))
	if err != nil {
		log.Fatalf("invalid abi %s\n", MultisendABI)
	}
	bytecode, err = multisendABI.Pack(sendCoin, addrs, amounts, "")
	return
}
