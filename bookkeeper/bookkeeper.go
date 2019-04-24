// Copyright (c) 2019 IoTeX
// This program is free software: you can redistribute it and/or modify it under the terms of the
// GNU General Public License as published by the Free Software Foundation, either version 3 of
// the License, or (at your option) any later version.
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See
// the GNU General Public License for more details.
// You should have received a copy of the GNU General Public License along with this program. If
// not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/protobuf/proto"
	. "github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v2"

	"github.com/iotexproject/iotex-address/address"
	"github.com/iotexproject/iotex-core/action/protocol/rewarding/rewardingpb"
	"github.com/iotexproject/iotex-core/protogen/iotexapi"
	"github.com/iotexproject/iotex-election/committee"
)

// Bucket of votes
type Bucket struct {
	owner  string
	amount *big.Int
}

// hard code
const (
	MultisendABI  = `[{"constant":false,"inputs":[{"name":"recipients","type":"address[]"},{"name":"amounts","type":"uint256[]"},{"name":"payload","type":"string"}],"name":"multiSend","outputs":[],"payable":true,"stateMutability":"payable","type":"function"},{"anonymous":false,"inputs":[{"indexed":false,"name":"recipient","type":"address"},{"indexed":false,"name":"amount","type":"uint256"}],"name":"Transfer","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"refund","type":"uint256"}],"name":"Refund","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"payload","type":"string"}],"name":"Payload","type":"event"}]`
	MultisendFunc = "multiSend"
	Disclaim      = "This Bookkeeper is a REFERENCE IMPLEMENTATION of reward distribution tool provided by IOTEX FOUNDATION. IOTEX FOUNDATION disclaims all responsibility for any damages or losses (including, without limitation, financial loss, damages for loss in business projects, loss of profits or other consequential losses) arising in contract, tort or otherwise from the use of or inability to use the Bookkeeper, or from any action or decision taken as a result of using this Bookkeeper."
)

// Hard code
var bpHexMap map[string]string
var abiJSON string
var abiFunc string

func init() {
	// init zap
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	zapCfg.Level.SetLevel(zap.WarnLevel)
	l, err := zapCfg.Build()
	if err != nil {
		log.Fatalln("Failed to init zap global logger, no zap log will be shown till zap is properly initialized: ", err)
	}
	zap.ReplaceGlobals(l)
}

func main() {
	var configPath string
	var startEpoch uint64
	var toEpoch uint64
	var bp string
	var endpoint string
	var distPercentage uint64
	var rewardAddress string
	var withFoundationBonus bool
	var byteCodeMode bool
	var useIOAddr bool
	fmt.Printf("\n%s\n%s\n", Bold(Red("Attention")), Red(Disclaim))
	flag.StringVar(&configPath, "config", "committee.yaml", "path of server config file")
	flag.Uint64Var(&startEpoch, "start", 0, "iotex epoch start")
	flag.Uint64Var(&toEpoch, "to", 0, "iotex epoch to")
	flag.StringVar(&bp, "bp", "", "bp name")
	flag.StringVar(&endpoint, "endpoint", "api.iotex.one:443", "set endpoint")
	flag.Uint64Var(&distPercentage, "percentage", 100, "distribution percentage of epoch reward")
	flag.StringVar(&rewardAddress, "reward-address", "", "choose reward address in certain epoch")
	flag.BoolVar(&withFoundationBonus, "with-foundation-bonus", false, "add foundation bonus in distribution")
	flag.BoolVar(&byteCodeMode, "bytecodeMode", false, "output in byte code mode")
	flag.BoolVar(&useIOAddr, "useIOAddr", false, "output io address in csv")
	flag.Parse()

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalln("failed to load config file")
	}
	var config committee.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalln("failed to unmarshal config")
	}
	if len(bp) == 0 {
		log.Fatalln("please set bp name by '--bp'")
	}
	delegateName, err := decodeDelegateName(bp)
	if err != nil {
		log.Fatalf("failed to parse bp name %s\n", bp)
	}
	if startEpoch == 0 || toEpoch == 0 {
		log.Fatalln("please set correct epoch number by using '--start' and '--to'")
	}
	if distPercentage == 0 {
		log.Fatalln("please set distribution percentage by '--percentage'")
	}
	multisendABI, err := abi.JSON(strings.NewReader(MultisendABI))
	if err != nil {
		log.Fatalf("invalid abi %s\n", MultisendABI)
	}
	if distPercentage > 100 {
		fmt.Println(Brown("\nWarning: percentage " + strconv.Itoa(int(distPercentage)) + `% is larger than 100%`))
	}
	if toEpoch-startEpoch >= 24 {
		fmt.Println(Brown("\nWarning: fetch more than 24 epoches' voters may cost much time"))
	}
	fmt.Println()

	distributions := make(map[string]*big.Int)
	for epochNum := startEpoch; epochNum <= toEpoch; epochNum++ {
		fmt.Printf("processing epoch %d\n", epochNum)
		gravityChainHeight, err := gravityChainHeight(endpoint, epochNum)
		if err != nil {
			log.Fatalf("Failed to get gravity chain height for epoch %d\n%+v", epochNum, err)
		}
		fmt.Printf("\tgravity chain height %d\n", gravityChainHeight)
		rewardAddress, totalVotes, buckets, err := readEthereum(gravityChainHeight, delegateName, config)
		if err != nil {
			log.Fatalf("Failed to fetch data from ethereum for epoch %d\n%+v", epochNum, err)
		}
		if len(rewardAddress) == 0 {
			fmt.Println("no reward address specified")
			continue
		}
		fmt.Printf("\treward address is %s\n", rewardAddress)
		reward, err := getReward(endpoint, epochNum, rewardAddress, withFoundationBonus)
		if err != nil {
			log.Fatalf("Failed to fetch reward for epoch %d\n%+v", epochNum, err)
		}
		fmt.Printf("\treward: %d\n", reward)
		if reward.Sign() == 0 {
			continue
		}
		reward = new(big.Int).Div(new(big.Int).Mul(reward, new(big.Int).SetUint64(distPercentage)), big.NewInt(100))
		for _, bucket := range buckets {
			if _, ok := distributions[bucket.owner]; !ok {
				distributions[bucket.owner] = big.NewInt(0)
			}
			distributions[bucket.owner].Add(
				distributions[bucket.owner],
				new(big.Int).Div(new(big.Int).Mul(bucket.amount, reward), totalVotes),
			)
		}
	}
	if !byteCodeMode {
		filename := Sprintf("epoch_%d_to_%d.csv", startEpoch, toEpoch)
		writeCSV(
			filename,
			useIOAddr,
			distributions,
		)
		fmt.Printf("byte code has been written to %s\n", filename)
	} else {
		filename := Sprintf("epoch_%d_to_%d.txt", startEpoch, toEpoch)
		writeByteCode(
			filename,
			multisendABI,
			distributions,
			fmt.Sprintf("reward from delegate %s for epoch %d to %d", string(delegateName), startEpoch, toEpoch),
		)
		fmt.Printf("csv format data has been written to %s\n", filename)
	}
}

func getReward(endpoint string, epoch uint64, rewardAddress string, withFoundationBonus bool) (*big.Int, error) {
	lastBlock := epoch * 24 * 15 // numDelegate: 24, subEpoch: 15
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	cli := iotexapi.NewAPIServiceClient(conn)
	blockRequest := &iotexapi.GetBlockMetasRequest{
		Lookup: &iotexapi.GetBlockMetasRequest_ByIndex{
			ByIndex: &iotexapi.GetBlockMetasByIndexRequest{
				Start: lastBlock,
				Count: 1,
			},
		},
	}
	ctx := context.Background()
	blockResponse, err := cli.GetBlockMetas(ctx, blockRequest)
	if err != nil {
		return nil, err
	}
	if len(blockResponse.BlkMetas) == 0 {
		return nil, errors.Errorf("failed to get last block in epoch %d", epoch)
	}
	actionsRequest := &iotexapi.GetActionsRequest{
		Lookup: &iotexapi.GetActionsRequest_ByBlk{
			ByBlk: &iotexapi.GetActionsByBlockRequest{
				BlkHash: blockResponse.BlkMetas[0].Hash,
				Start:   uint64(blockResponse.BlkMetas[0].NumActions) - 1,
				Count:   1,
			},
		},
	}
	actionsResponse, err := cli.GetActions(ctx, actionsRequest)
	if err != nil {
		return nil, err
	}
	if len(actionsResponse.ActionInfo) == 0 {
		return nil, errors.Errorf("failed to get last action in epoch %d", epoch)
	}
	if actionsResponse.ActionInfo[0].Action.Core.GetGrantReward() == nil {
		return nil, errors.New("Not grantReward action")
	}
	receiptRequest := &iotexapi.GetReceiptByActionRequest{
		ActionHash: actionsResponse.ActionInfo[0].ActHash,
	}
	receiptResponse, err := cli.GetReceiptByAction(ctx, receiptRequest)
	if err != nil {
		return nil, err
	}
	eReward := big.NewInt(0)
	fReward := big.NewInt(0)
	for _, receiptLog := range receiptResponse.ReceiptInfo.Receipt.Logs {
		var rewardLog rewardingpb.RewardLog
		var ok bool
		if err := proto.Unmarshal(receiptLog.Data, &rewardLog); err != nil {
			return nil, err
		}
		if strings.Compare(rewardLog.Addr, rewardAddress) != 0 {
			continue
		}
		if rewardLog.Type == rewardingpb.RewardLog_EPOCH_REWARD {
			eReward, ok = new(big.Int).SetString(rewardLog.Amount, 10)
			if !ok {
				return nil, errors.Errorf("Failed to parse epoch reward %s", rewardLog.Amount)
			}
		} else if rewardLog.Type == rewardingpb.RewardLog_FOUNDATION_BONUS && withFoundationBonus {
			fReward, ok = new(big.Int).SetString(rewardLog.Amount, 10)
			if !ok {
				return nil, errors.Errorf("Failed to parse foundation reward %s", rewardLog.Amount)
			}
		}
	}
	return new(big.Int).Add(eReward, fReward), nil
}

func gravityChainHeight(endpoint string, epochNum uint64) (uint64, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	cli := iotexapi.NewAPIServiceClient(conn)
	request := iotexapi.GetEpochMetaRequest{EpochNumber: epochNum}
	response, err := cli.GetEpochMeta(context.Background(), &request)
	if err != nil {
		return 0, err
	}
	return response.EpochData.GravityChainStartHeight, nil
}

func readEthereum(
	height uint64,
	delegateName []byte,
	config committee.Config,
) (rewardAddress string, totalVotes *big.Int, buckets []Bucket, err error) {
	totalVotes = big.NewInt(0)
	committee, err := committee.NewCommittee(nil, config)
	if err != nil {
		return
	}
	result, err := committee.FetchResultByHeight(height)
	if err != nil {
		return
	}
	for _, delegate := range result.Delegates() {
		if bytes.Equal(delegate.Name(), delegateName) {
			rewardAddress = string(delegate.RewardAddress())
			break
		}
	}
	if len(rewardAddress) == 0 {
		return
	}
	for _, vote := range result.VotesByDelegate(delegateName) {
		amount := vote.WeightedAmount()
		buckets = append(buckets, Bucket{
			owner:  hex.EncodeToString(vote.Voter()),
			amount: amount,
		})
		totalVotes.Add(totalVotes, amount)
	}
	return
}

func decodeDelegateName(rawName string) ([]byte, error) {
	if len(rawName) == 24 {
		return hex.DecodeString(rawName)
	}
	zeroBytes := []byte{}
	for i := 0; i < 12-len(rawName); i++ {
		zeroBytes = append(zeroBytes, byte(0))
	}
	return append(zeroBytes, []byte(rawName)...), nil
}

func writeByteCode(filename string, abi abi.ABI, distributions map[string]*big.Int, msg string) error {
	recipients := make([]common.Address, 0)
	amounts := make([]*big.Int, 0)
	for voter, reward := range distributions {
		recipients = append(recipients, common.HexToAddress(voter))
		amounts = append(amounts, reward)
	}
	bytes, err := abi.Pack(MultisendFunc, recipients, amounts, msg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, bytes, 066)
}

func writeCSV(filename string, useIOAddr bool, distributions map[string]*big.Int) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	for owner, reward := range distributions {
		recipient := common.HexToAddress(owner)
		if useIOAddr {
			ioaddr, err := address.FromBytes(recipient.Bytes())
			if err != nil {
				return err
			}
			if err := writer.Write([]string{ioaddr.String(), reward.String()}); err != nil {
				return err
			}
		} else {
			if err := writer.Write([]string{recipient.String(), reward.String()}); err != nil {
				return err
			}
		}
	}
	return nil
}
