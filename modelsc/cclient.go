// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file is a derived work, based on ava-labs code.
//
// It is distributed under the same license conditions as the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********************************************************

package modelsc

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/lasthyphen/coreth/core/types"
	"github.com/lasthyphen/coreth/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Client struct {
	rpcClient *rpc.Client
	// ethClient ethclient.Client
	lock sync.Mutex
}

func NewClient(url string) (*Client, error) {
	rc, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}
	cl := &Client{}
	cl.rpcClient = rc
	// cl.ethClient = ethclient.NewClient(rc)
	return cl, nil
}

func (c *Client) Close() {
	c.rpcClient.Close()
}

type ExtendedReceipt struct {

	// Consensus fields: These fields are defined by the Yellow Paper
	PostState         []byte       `json:"root"`
	Status            uint64       `json:"status"`
	CumulativeGasUsed uint64       `json:"cumulativeGasUsed" gencodec:"required"`
	Bloom             types.Bloom  `json:"logsBloom"         gencodec:"required"`
	Logs              []*types.Log `json:"logs"              gencodec:"required"`
	Type              uint8        `json:"type,omitempty"`

	// Implementation fields: These fields are added by geth when processing a transaction.
	// They are stored in the chain database.
	TxHash           common.Hash     `json:"transactionHash" gencodec:"required"`
	ContractAddress  *common.Address `json:"contractAddress"`
	TransactionIndex uint            `json:"transactionIndex"`

	GasUsed uint64 `json:"gasUsed" gencodec:"required"`

	// Inclusion information: These fields provide information about the inclusion of the
	// transaction corresponding to this receipt.
	BlockHash   common.Hash `json:"blockHash,omitempty"`
	BlockNumber *big.Int    `json:"blockNumber,omitempty"`

	EffectiveGasPrice uint64 `json:"effectiveGasPrice"`

	Raw []byte `json:"-"`
}

// UnmarshalJSON unmarshals from JSON.
func (r *ExtendedReceipt) UnmarshalJSON(input []byte) error {
	type Receipt struct {
		Type              *hexutil.Uint64 `json:"type,omitempty"`
		PostState         *hexutil.Bytes  `json:"root"`
		Status            *hexutil.Uint64 `json:"status"`
		CumulativeGasUsed *hexutil.Uint64 `json:"cumulativeGasUsed" gencodec:"required"`
		Bloom             *types.Bloom    `json:"logsBloom"         gencodec:"required"`
		Logs              []*types.Log    `json:"logs"              gencodec:"required"`
		TxHash            *common.Hash    `json:"transactionHash" gencodec:"required"`
		ContractAddress   *common.Address `json:"contractAddress"`
		GasUsed           *hexutil.Uint64 `json:"gasUsed" gencodec:"required"`
		BlockHash         *common.Hash    `json:"blockHash,omitempty"`
		BlockNumber       *hexutil.Big    `json:"blockNumber,omitempty"`
		TransactionIndex  *hexutil.Uint   `json:"transactionIndex"`
		EffectiveGasPrice *hexutil.Uint64 `json:"effectiveGasPrice"`
	}
	var dec Receipt
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Type != nil {
		r.Type = uint8(*dec.Type)
	}
	if dec.PostState != nil {
		r.PostState = *dec.PostState
	}
	if dec.Status != nil {
		r.Status = uint64(*dec.Status)
	}
	if dec.CumulativeGasUsed == nil {
		return errors.New("missing required field 'cumulativeGasUsed' for Receipt")
	}
	r.CumulativeGasUsed = uint64(*dec.CumulativeGasUsed)
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Receipt")
	}
	r.Bloom = *dec.Bloom
	if dec.Logs == nil {
		return errors.New("missing required field 'logs' for Receipt")
	}
	r.Logs = dec.Logs
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionHash' for Receipt")
	}
	r.TxHash = *dec.TxHash
	r.ContractAddress = dec.ContractAddress
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Receipt")
	}
	r.GasUsed = uint64(*dec.GasUsed)
	if dec.BlockHash != nil {
		r.BlockHash = *dec.BlockHash
	}
	if dec.BlockNumber != nil {
		r.BlockNumber = (*big.Int)(dec.BlockNumber)
	}
	if dec.TransactionIndex != nil {
		r.TransactionIndex = uint(*dec.TransactionIndex)
	}
	if dec.EffectiveGasPrice != nil {
		r.EffectiveGasPrice = uint64(*dec.EffectiveGasPrice)
	}
	r.Raw = input
	return nil
}

func (c *Client) ReadReceipt(txHash string, rpcTimeout time.Duration) (*ExtendedReceipt, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	ctx, cancelCTX := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancelCTX()

	result := &ExtendedReceipt{}
	if err := c.rpcClient.CallContext(ctx, result, "eth_getTransactionReceipt", txHash); err != nil {
		return nil, err
	}

	return result, nil
}
