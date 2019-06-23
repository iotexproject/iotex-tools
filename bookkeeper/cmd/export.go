// Copyright (c) 2019 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/protobuf/proto"
	"github.com/iotexproject/iotex-address/address"
	"github.com/iotexproject/iotex-core/action/protocol/rewarding/rewardingpb"
	"github.com/iotexproject/iotex-core/protogen/iotexapi"
	"github.com/iotexproject/iotex-election/committee"
	"github.com/iotexproject/iotex-tools/util"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	configPath          string
	endpoint            string
	start               uint64
	to                  uint64
	percentage          uint
	withFoundationBonus bool
	unit                string
	useIOAddr           bool
)

// Bucket of votes
type Bucket struct {
	owner  string
	amount *big.Int
}

// ExportCmd exports reward result into csv
var ExportCmd = &cobra.Command{
	Use:   "export bp-name",
	Short: "Export reward result in csv",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return export(configPath, args[0], start, to, endpoint, unit, percentage, withFoundationBonus, useIOAddr)
	},
}

func init() {
	ExportCmd.Flags().StringVar(&configPath, "config", "committee.yaml", "config file")
	ExportCmd.Flags().Uint64Var(&start, "start", 0, "start epoch number")
	ExportCmd.Flags().Uint64Var(&to, "to", 0, "to epoch number")
	ExportCmd.Flags().StringVarP(&endpoint, "endpoint", "e", "api.iotex.one:443", "iotex endpoint")
	ExportCmd.Flags().UintVarP(&percentage, "percentage", "p", 100, "percentage")
	ExportCmd.Flags().BoolVarP(&withFoundationBonus, "with-foundation-bonus", "w", false, "epoch bonus with foundation bonus")
	ExportCmd.Flags().StringVarP(&unit, "unit", "u", "Rau", "unit of amount")
	ExportCmd.Flags().BoolVarP(&useIOAddr, "in-io-address", "i", false, "output address in iotex format")
}

func export(configPath string, bp string, startEpoch uint64, toEpoch uint64, endpoint string, unit string, distPercentage uint, withFoundationBonus bool, useIOAddr bool) error {
	committee, err := util.NewCommitteeWithConfigFile(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to create committee %+v")
	}
	if len(bp) == 0 {
		return errors.New("bp name is invalid")
	}
	delegateName, err := decodeDelegateName(bp)
	if err != nil {
		return errors.Errorf("failed to parse bp name %s", bp)
	}
	if startEpoch == 0 || toEpoch == 0 {
		return errors.Errorf("invalid epoch number from %d and to %d", startEpoch, toEpoch)
	}
	if distPercentage == 0 {
		return errors.Errorf("invalid distribution percentage %d", distPercentage)
	}
	switch strings.ToLower(unit) {
	case "rau":
		unit = "Rau"
	case "iotx":
		unit = "IOTX"
	default:
		return errors.Errorf("invalid amount unit %s", unit)
	}

	if distPercentage > 100 {
		fmt.Println(aurora.Brown("\nWarning: percentage " + strconv.Itoa(int(distPercentage)) + `% is larger than 100%`))
	}
	if toEpoch-startEpoch >= 24 {
		fmt.Println(aurora.Brown("\nWarning: fetch more than 24 epoches' voters may cost much time"))
	}

	fmt.Printf(
		"\nStart calculating distribution for %s (%s) from epoch %d to epoch %d\n",
		string(delegateName),
		hex.EncodeToString(delegateName),
		startEpoch,
		toEpoch,
	)
	distributions := make(map[string]*big.Int)
	for epochNum := startEpoch; epochNum <= toEpoch; epochNum++ {
		fmt.Printf("processing epoch %d\n", epochNum)
		gravityChainHeight, err := gravityChainHeight(endpoint, epochNum)
		if err != nil {
			return errors.Wrapf(err, "failed to get gravity chain height for epoch %d", epochNum)
		}
		fmt.Printf("\tgravity chain height %d\n", gravityChainHeight)
		rewardAddress, totalVotes, buckets, err := readEthereum(gravityChainHeight, delegateName, committee)
		if err != nil {
			return errors.Wrapf(err, "failed to fetch data from ethereum for epoch %d", epochNum)
		}
		if len(rewardAddress) == 0 {
			fmt.Println("no reward address specified")
			continue
		}
		fmt.Printf("\treward address is %s\n", rewardAddress)
		reward, err := getReward(endpoint, epochNum, rewardAddress, withFoundationBonus)
		if err != nil {
			return errors.Wrapf(err, "failed to fetch reward for epoch %d", epochNum)
		}
		fmt.Printf("\treward: %d\n", reward)
		if reward.Sign() == 0 {
			continue
		}
		reward = new(big.Int).Div(new(big.Int).Mul(reward, new(big.Int).SetUint64(uint64(distPercentage))), big.NewInt(100))
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
	fmt.Printf("The output amount unit is in %s.\n", unit)
	filename := fmt.Sprintf("%s_epoch_%d_to_%d_in_%s.csv", delegateName, startEpoch, toEpoch, unit)
	filename = strings.Replace(strings.Trim(filename, "\x00"), "\x00", "#", -1)
	writeCSV(
		filename,
		useIOAddr,
		distributions,
		unit,
	)
	fmt.Printf("csv format data has been written to %s\n", filename)
	return nil
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
	committee committee.Committee,
) (rewardAddress string, totalVotes *big.Int, buckets []Bucket, err error) {
	totalVotes = big.NewInt(0)
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

// OneIOTX is the amount of one IOTX in Rau
var OneIOTX = new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

func writeCSV(filename string, useIOAddr bool, distributions map[string]*big.Int, unit string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	type Owner struct {
		Addr   common.Address
		Reward *big.Float
	}
	var owners []Owner
	for owner, reward := range distributions {
		rewardInFloat := new(big.Float).SetInt(reward)
		switch unit {
		case "Rau":
			// do nothing
		case "IOTX":
			rewardInFloat = new(big.Float).Quo(rewardInFloat, OneIOTX)
		default:
			return errors.Errorf("unit %s is not supported unit", unit)
		}
		owners = append(owners, Owner{common.HexToAddress(owner), rewardInFloat})
	}
	sort.Slice(owners, func(i, j int) bool {
		return owners[i].Reward.Cmp(owners[j].Reward) >= 0
	})
	for _, owner := range owners {
		var addr string
		if useIOAddr {
			ioaddr, err := address.FromBytes(owner.Addr.Bytes())
			if err != nil {
				return err
			}
			addr = ioaddr.String()
		} else {
			addr = owner.Addr.String()
		}
		if err := writer.Write([]string{addr, owner.Reward.String()}); err != nil {
			return err
		}
	}
	return nil
}
