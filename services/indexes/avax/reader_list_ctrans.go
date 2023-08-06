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
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/lasthyphen/coreth/core/types"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services/indexes/params"
	"github.com/lasthyphen/mages/utils"
	"github.com/gocraft/dbr/v2"
)

func (r *Reader) ListCTransactions(ctx context.Context, p *params.ListCTransactionsParams) (*models.CTransactionList, error) {
	toCTransactionData := func(t *types.Transaction) *models.CTransactionData {
		res := &models.CTransactionData{}
		res.Type = int(t.Type())
		res.Hash = t.Hash().Hex()
		if !strings.HasPrefix(res.Hash, "0x") {
			res.Hash = "0x" + res.Hash
		}
		res.Nonce = t.Nonce()
		if t.GasPrice() != nil {
			str := t.GasPrice().String()
			res.GasPrice = &str
		}
		res.GasLimit = t.Gas()
		if t.GasFeeCap() != nil {
			str := t.GasFeeCap().String()
			res.GasFeeCap = &str
		}
		if t.GasTipCap() != nil {
			str := t.GasTipCap().String()
			res.GasTipCap = &str
		}
		if t.To() != nil {
			str := utils.CommonAddressHexRepair(t.To())
			res.ToAddr = str
		}
		if t.Value() != nil {
			str := t.Value().String()
			res.Amount = &str
		}
		if len(t.Data()) != 0 {
			hexdata := "0x" + hex.EncodeToString(t.Data())
			res.Payload = &hexdata
		}
		v, s, r := t.RawSignatureValues()
		if v != nil {
			str := v.String()
			res.V = &str
		}
		if s != nil {
			str := s.String()
			res.S = &str
		}
		if r != nil {
			str := r.String()
			res.R = &str
		}
		return res
	}

	dbRunner, err := r.conns.DB().NewSession("list_ctransactions", cfg.RequestTimeout)
	if err != nil {
		return nil, err
	}

	var dataList []*db.CvmTransactionsTxdata

	sq := dbRunner.Select(
		"hash",
		"block",
		"idx",
		"from_addr",
		"to_addr",
		"nonce",
		"serialization",
		"created_at",
	).From(db.TableCvmTransactionsTxdata)

	r.listCTransFilter(p, dbRunner, sq)
	if len(p.Hashes) > 0 {
		sq.
			Where(db.TableCvmTransactionsTxdata+".hash in ?", p.Hashes)
	}

	_, err = p.Apply(sq).
		OrderDesc("created_at").
		LoadContext(ctx, &dataList)
	if err != nil {
		return nil, err
	}

	trItemsByHash := make(map[string]*models.CTransactionData)

	trItems := make([]*models.CTransactionData, 0, len(dataList))
	hashes := make([]string, 0, len(dataList))

	for _, txdata := range dataList {
		var tr types.Transaction
		err := tr.UnmarshalJSON(txdata.Serialization)
		if err != nil {
			return nil, err
		}
		ctr := toCTransactionData(&tr)
		ctr.Block = txdata.Block
		ctr.CreatedAt = txdata.CreatedAt
		ctr.FromAddr = txdata.FromAddr
		trItems = append(trItems, ctr)

		trItemsByHash[ctr.Hash] = ctr
		hashes = append(hashes, ctr.Hash)
	}

	err = r.getReceipts(ctx, dbRunner, hashes, trItemsByHash)
	if err != nil {
		return nil, err
	}

	listParamsOriginal := p.ListParams

	return &models.CTransactionList{
		Transactions: trItems,
		StartTime:    listParamsOriginal.StartTime,
		EndTime:      listParamsOriginal.EndTime,
	}, nil
}

func (r *Reader) listCTransFilter(p *params.ListCTransactionsParams, dbRunner *dbr.Session, sq *dbr.SelectStmt) {
	createdatefilter := func(b *dbr.SelectStmt) *dbr.SelectStmt {
		if p.ListParams.StartTimeProvided && !p.ListParams.StartTime.IsZero() {
			b.Where(db.TableCvmTransactionsTxdata+".created_at >= ?", p.ListParams.StartTime)
		}
		if p.ListParams.EndTimeProvided && !p.ListParams.EndTime.IsZero() {
			b.Where(db.TableCvmTransactionsTxdata+".created_at < ?", p.ListParams.EndTime)
		}
		return b
	}

	blockfilter := func(b *dbr.SelectStmt) *dbr.SelectStmt {
		if p.BlockStart == nil && p.BlockEnd == nil {
			return b
		}
		if p.BlockStart != nil {
			b.Where(db.TableCvmTransactionsTxdata + ".block >= " + p.BlockStart.String())
		}
		if p.BlockEnd != nil {
			b.Where(db.TableCvmTransactionsTxdata + ".block <= " + p.BlockEnd.String())
		}
		return b
	}

	if len(p.CAddressesTo) > 0 {
		subq := createdatefilter(
			blockfilter(dbRunner.Select(db.TableCvmTransactionsTxdata+".hash").From(db.TableCvmTransactionsTxdata).
				Where(db.TableCvmTransactionsTxdata+".to_addr in ?", p.CAddressesTo)),
		)
		sq.
			Where("hash in ?",
				dbRunner.Select("hash").From(subq.As("to_sq")),
			)
	}

	if len(p.CAddressesFrom) > 0 {
		subq := createdatefilter(
			blockfilter(dbRunner.Select(db.TableCvmTransactionsTxdata+".hash").From(db.TableCvmTransactionsTxdata).
				Where(db.TableCvmTransactionsTxdata+".from_addr in ?", p.CAddressesFrom)),
		)
		sq.
			Where("hash in ?",
				dbRunner.Select("hash").From(subq.As("from_sq")),
			)
	}

	if len(p.CAddresses) > 0 {
		subqfrom := createdatefilter(
			blockfilter(dbRunner.Select(db.TableCvmTransactionsTxdata+".hash").From(db.TableCvmTransactionsTxdata).
				Where(".from_addr in ?", p.CAddresses)),
		)
		subqto := createdatefilter(
			blockfilter(dbRunner.Select(db.TableCvmTransactionsTxdata+".hash").From(db.TableCvmTransactionsTxdata).
				Where("to_addr in ?", p.CAddresses)),
		)
		sq.
			Where("hash in ?",
				dbRunner.Select("hash").From(dbr.Union(subqfrom, subqto).As("to_from_sq")),
			)
	}

	blockfilter(sq)
}

func (r *Reader) getReceipts(ctx context.Context, dbRunner *dbr.Session, hashes []string, trItemsByHash map[string]*models.CTransactionData) error {
	if len(hashes) == 0 {
		return nil
	}
	var err error
	var txTransactionReceiptServices []*db.CvmTransactionsReceipt
	_, err = dbRunner.Select(
		"serialization",
	).From(db.TableCvmTransactionsReceipts).
		Where("hash in ?", hashes).
		LoadContext(ctx, &txTransactionReceiptServices)
	if err != nil {
		return err
	}

	for _, txTransactionReceiptService := range txTransactionReceiptServices {
		txTransactionReceiptModel := &types.Receipt{}
		err = json.Unmarshal(txTransactionReceiptService.Serialization, txTransactionReceiptModel)
		if err != nil {
			return err
		}
		trItemsByHash[txTransactionReceiptService.Hash].Receipt = txTransactionReceiptModel
	}
	return nil
}
