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
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/lasthyphen/coreth/core/types"
	"github.com/lasthyphen/coreth/ethclient"
	"github.com/lasthyphen/coreth/rpc"
)

var ErrNotFound = errors.New("block not found")

type Block struct {
	Header         types.Header        `json:"header"`
	Uncles         []types.Header      `json:"uncles"`
	TxsBytes       *[][]byte           `json:"txs,omitempty"`
	Version        uint32              `json:"version"`
	BlockExtraData []byte              `json:"blockExtraData"`
	Txs            []types.Transaction `json:"transactions,omitempty"`
}

func New(bl *types.Block) (*Block, error) {
	var cblock Block
	cblock.Version = bl.Version()
	cblock.BlockExtraData = bl.ExtData()
	if cblock.BlockExtraData != nil {
		if len(cblock.BlockExtraData) == 0 {
			cblock.BlockExtraData = nil
		}
	}
	var h = bl.Header()
	if h != nil {
		cblock.Header = *h
	}
	for _, u := range bl.Uncles() {
		if u == nil {
			continue
		}
		cblock.Uncles = append(cblock.Uncles, *u)
	}
	for _, t := range bl.Transactions() {
		cblock.Txs = append(cblock.Txs, *t)
	}
	return &cblock, nil
}

func Marshal(bl *types.Block) ([]byte, error) {
	b, err := New(bl)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("invalid block")
	}
	return json.Marshal(b)
}

func Unmarshal(data []byte) (*Block, error) {
	var block Block
	err := json.Unmarshal(data, &block)
	if err != nil {
		return nil, err
	}

	if block.TxsBytes != nil && len(*block.TxsBytes) != 0 {
		// convert the tx bytes into transactions.
		for _, t := range *block.TxsBytes {
			var tr types.Transaction
			err := tr.UnmarshalJSON(t)
			if err != nil {
				return nil, err
			}
			block.Txs = append(block.Txs, tr)
		}
	}
	return &block, err
}

type Client struct {
	rpcClient *rpc.Client
	ethClient ethclient.Client
	lock      sync.Mutex
}

func NewClient(url string) (*Client, error) {
	rc, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}
	cl := &Client{}
	cl.rpcClient = rc
	cl.ethClient = ethclient.NewClient(rc)
	return cl, nil
}

func (c *Client) Latest(rpcTimeout time.Duration) (*big.Int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	ctx, cancelCTX := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancelCTX()
	bl, err := c.ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	return big.NewInt(0).SetUint64(bl), nil
}

func (c *Client) Close() {
	c.rpcClient.Close()
}

type TransactionReceipt struct {
	Hash    string `json:"hash"`
	Status  uint16 `json:"status"`
	GasUsed uint64 `json:"gasUsed"`
	Receipt []byte `json:"receipt"`
}

type BlockContainer struct {
	Block    *types.Block
	Receipts []*TransactionReceipt
}

func (c *Client) ReadBlock(blockNumber *big.Int, rpcTimeout time.Duration) (*BlockContainer, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	ctx, cancelCTX := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancelCTX()

	bl, err := c.ethClient.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}

	txReceipts := make([]*TransactionReceipt, 0, len(bl.Transactions()))
	for _, tx := range bl.Transactions() {
		txh := tx.Hash().Hex()
		if !strings.HasPrefix(txh, "0x") {
			txh = "0x" + txh
		}

		var result types.Receipt
		err = c.rpcClient.CallContext(ctx, &result, "eth_getTransactionReceipt",
			txh)
		if err != nil {
			return nil, err
		}
		receiptBits, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		txReceipts = append(txReceipts, &TransactionReceipt{
			Hash:    txh,
			Status:  uint16(result.Status),
			GasUsed: result.GasUsed,
			Receipt: receiptBits,
		})
	}
	return &BlockContainer{Block: bl, Receipts: txReceipts}, nil
}
