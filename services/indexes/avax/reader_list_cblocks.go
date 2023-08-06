// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file is a derived work, based on ava-labs code.
//
// It is distributed under the same license conditions as the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********************************************************

package djtx

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services/indexes/params"
)

func (r *Reader) ListCBlocks(ctx context.Context, p *params.ListCBlocksParams) (*models.CBlockList, error) {
	fmtHex := func(n uint64) string { return "0x" + strconv.FormatUint(n, 16) }

	dbRunner, err := r.conns.DB().NewSession("list_cblocks", cfg.RequestTimeout)
	if err != nil {
		return nil, err
	}

	result := models.CBlockList{}
	err = dbRunner.Select("COUNT(block)").
		From(db.TableCvmBlocks).
		LoadOneContext(ctx, &result.BlockCount)
	if err != nil {
		return nil, err
	}

	err = dbRunner.Select("COUNT(hash)").
		From(db.TableCvmTransactionsTxdata).
		LoadOneContext(ctx, &result.TransactionCount)
	if err != nil {
		return nil, err
	}

	// Setp 1 get Block headers
	if p.ListParams.Limit > 0 {
		var blockList []*db.CvmBlocks

		_, err = dbRunner.Select(
			"evm_tx",
			"atomic_tx",
			"serialization",
		).
			From(db.TableCvmBlocks).
			OrderDesc("block").
			Limit(uint64(p.ListParams.Limit)).
			Offset(uint64(p.ListParams.Offset)).
			LoadContext(ctx, &blockList)
		if err != nil {
			return nil, err
		}

		result.Blocks = make([]*models.CBlockHeaderBase, len(blockList))
		for i, block := range blockList {
			err = json.Unmarshal(block.Serialization, &result.Blocks[i])
			if err != nil {
				return nil, err
			}
			result.Blocks[i].EvmTx = block.EvmTx
			result.Blocks[i].AtomicTx = block.AtomicTx
		}
	}

	// Setp 2 get Transactions
	if p.TxLimit > 0 {
		var txList []*struct {
			Serialization []byte
			CreatedAt     time.Time
			FromAddr      string
			Block         uint64
			Idx           uint64
			Status        uint16
			GasUsed       uint64
		}

		_, err = dbRunner.Select(
			db.TableCvmTransactionsTxdata+".serialization",
			db.TableCvmTransactionsTxdata+".created_at",
			"from_addr",
			"block",
			"idx",
			"status",
			"gas_used",
		).
			From(db.TableCvmTransactionsTxdata).
			LeftJoin(db.TableCvmTransactionsReceipts, db.TableCvmTransactionsTxdata+".hash = "+db.TableCvmTransactionsReceipts+".hash").
			OrderDesc("block").
			OrderAsc("idx").
			Limit(uint64(p.TxLimit)).
			Offset(uint64(p.TxOffset)).
			LoadContext(ctx, &txList)
		if err != nil {
			return nil, err
		}

		result.Transactions = make([]*models.CTransactionDataBase, len(txList))
		for i, tx := range txList {
			dest := &result.Transactions[i]
			err = json.Unmarshal(tx.Serialization, dest)
			if err != nil {
				return nil, err
			}
			(*dest).Block = fmtHex(tx.Block)
			(*dest).Index = fmtHex(tx.Idx)
			(*dest).CreatedAt = fmtHex(uint64(tx.CreatedAt.Unix()))
			(*dest).From = tx.FromAddr
			(*dest).Status = fmtHex(uint64(tx.Status))
			(*dest).GasUsed = fmtHex(tx.GasUsed)
		}
	}

	return &result, nil
}
