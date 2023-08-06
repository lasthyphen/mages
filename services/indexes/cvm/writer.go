// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file is a derived work, based on ava-labs code whose
// original notices appear below.
//
// It is distributed under the same license conditions as the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********************************************************
// (c) 2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cvm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lasthyphen/coreth/core/types"
	"github.com/lasthyphen/coreth/plugin/evm"
	"github.com/lasthyphen/dijetsnodego/codec"
	"github.com/lasthyphen/dijetsnodego/genesis"
	"github.com/lasthyphen/dijetsnodego/ids"
	"github.com/lasthyphen/dijetsnodego/utils/math"
	"github.com/lasthyphen/dijetsnodego/version"
	"github.com/lasthyphen/dijetsnodego/vms/components/verify"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/modelsc"
	"github.com/lasthyphen/mages/services"
	djtxIndexer "github.com/lasthyphen/mages/services/indexes/djtx"
	"github.com/lasthyphen/mages/utils"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrUnknownBlockType = errors.New("unknown block type")
)

type Writer struct {
	networkID   uint32
	djtxAssetID ids.ID

	codec         codec.Manager
	djtx          *djtxIndexer.Writer
	ap5Activation uint64
}

func NewWriter(networkID uint32, chainID string) (*Writer, error) {
	_, djtxAssetID, err := genesis.FromConfig(genesis.GetConfig(networkID))
	if err != nil {
		return nil, err
	}

	ap5Activation := version.GetApricotPhase5Time(networkID).Unix()

	return &Writer{
		networkID:     networkID,
		djtxAssetID:   djtxAssetID,
		codec:         evm.Codec,
		djtx:          djtxIndexer.NewWriter(chainID, djtxAssetID),
		ap5Activation: uint64(ap5Activation),
	}, nil
}

func (*Writer) Name() string { return "cvm-index" }

func (w *Writer) ParseJSON(txdata []byte) ([]byte, error) {
	block, err := modelsc.Unmarshal(txdata)
	if err != nil {
		return nil, err
	}
	var atomicTxs []*evm.Tx
	if block.BlockExtraData == nil || len(block.BlockExtraData) == 0 {
		return []byte(""), nil
	}
	if block.Header.Time < w.ap5Activation {
		atomicTxs, err = w.extractAtomicTxsPreApricotPhase5(block.BlockExtraData)
	} else {
		atomicTxs, err = w.extractAtomicTxsPostApricotPhase5(block.BlockExtraData)
	}

	if err != nil {
		return nil, err
	}

	return json.Marshal(atomicTxs)
}

func (w *Writer) extractAtomicTxsPreApricotPhase5(atomicTxBytes []byte) ([]*evm.Tx, error) {
	atomicTx := new(evm.Tx)
	if _, err := w.codec.Unmarshal(atomicTxBytes, atomicTx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal atomic tx (pre-AP5): %w", err)
	}
	if err := atomicTx.Sign(w.codec, nil); err != nil {
		return nil, fmt.Errorf("failed to initialize singleton atomic tx due to: %w", err)
	}
	return []*evm.Tx{atomicTx}, nil
}

// [extractAtomicTxsPostApricotPhase5] extracts a slice of atomic transactions from [atomicTxBytes].
// Note: this function assumes [atomicTxBytes] is non-empty.
func (w *Writer) extractAtomicTxsPostApricotPhase5(atomicTxBytes []byte) ([]*evm.Tx, error) {
	var atomicTxs []*evm.Tx
	if _, err := w.codec.Unmarshal(atomicTxBytes, &atomicTxs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal atomic tx (post-AP5): %w", err)
	}

	for index, atx := range atomicTxs {
		if err := atx.Sign(w.codec, nil); err != nil {
			return nil, fmt.Errorf("failed to initialize atomic tx at index %d: %w", index, err)
		}
	}
	return atomicTxs, nil
}

func (w *Writer) ConsumeReceipt(ctx context.Context, conns *utils.Connections, c services.Consumable, transactionReceipt *modelsc.TransactionReceipt, persist db.Persist) error {
	job := conns.Stream().NewJob("cvm-index")
	sess := conns.DB().NewSessionForEventReceiver(job)

	dbTx, err := sess.Begin()
	if err != nil {
		return err
	}
	defer dbTx.RollbackUnlessCommitted()

	cCtx := services.NewConsumerContext(ctx, dbTx, c.Timestamp(), c.Nanosecond(), persist, c.ChainID())

	txReceiptService := &db.CvmTransactionsReceipt{
		Hash:          transactionReceipt.Hash,
		Status:        transactionReceipt.Status,
		GasUsed:       transactionReceipt.GasUsed,
		Serialization: transactionReceipt.Receipt,
		CreatedAt:     cCtx.Time(),
	}

	err = persist.InsertCvmTransactionsReceipt(ctx, dbTx, txReceiptService, cfg.PerformUpdates)
	if err != nil {
		return err
	}

	return dbTx.Commit()
}

func (w *Writer) Consume(ctx context.Context, conns *utils.Connections, c services.Consumable, block *modelsc.Block, persist db.Persist) error {
	job := conns.Stream().NewJob("cvm-index")
	sess := conns.DB().NewSessionForEventReceiver(job)

	dbTx, err := sess.Begin()
	if err != nil {
		return err
	}
	defer dbTx.RollbackUnlessCommitted()

	// Consume the tx and commit
	err = w.indexBlock(services.NewConsumerContext(ctx, dbTx, c.Timestamp(), c.Nanosecond(), persist, c.ChainID()), block)
	if err != nil {
		return err
	}
	return dbTx.Commit()
}

func (w *Writer) indexBlock(ctx services.ConsumerCtx, block *modelsc.Block) error {
	var atomicTxs []*evm.Tx
	if len(block.BlockExtraData) > 0 {
		var err error
		if block.Header.Time < w.ap5Activation {
			atomicTxs, err = w.extractAtomicTxsPreApricotPhase5(block.BlockExtraData)
		} else {
			atomicTxs, err = w.extractAtomicTxsPostApricotPhase5(block.BlockExtraData)
		}

		if err != nil {
			return err
		}
	}
	return w.indexBlockInternal(ctx, atomicTxs, block)
}

func (w *Writer) indexBlockInternal(ctx services.ConsumerCtx, atomicTXs []*evm.Tx, block *modelsc.Block) error {
	txIDs := make([]string, len(atomicTXs))

	var typ models.CChainType = 0
	var err error

	for i, atomicTX := range atomicTXs {
		txID := atomicTX.ID()
		txIDs[i] = txID.String()
		switch atx := atomicTX.UnsignedAtomicTx.(type) {
		case *evm.UnsignedExportTx:
			typ = models.CChainExport
			err = w.indexExportTx(ctx, txID, atx, block.BlockExtraData)
			if err != nil {
				return err
			}
		case *evm.UnsignedImportTx:
			unsignedBytes, err := w.codec.Marshal(0, &atomicTX.UnsignedAtomicTx)
			if err != nil {
				return err
			}

			typ = models.CChainImport
			err = w.indexImportTx(ctx, txID, atx, atomicTX.Creds, block.BlockExtraData, unsignedBytes)
			if err != nil {
				return err
			}
		default:
		}
	}

	for ipos, rawtx := range block.Txs {
		rawtxCp := rawtx
		txdata, err := json.Marshal(&rawtxCp)
		if err != nil {
			return err
		}
		rawhash := rawtx.Hash()
		toStr := utils.CommonAddressHexRepair(rawtx.To())

		signer := types.LatestSignerForChainID(rawtx.ChainId())
		fromAddr, err := signer.Sender(&rawtxCp)
		if err != nil {
			return err
		}
		fromStr := utils.CommonAddressHexRepair(&fromAddr)

		cvmTransactionTxdata := &db.CvmTransactionsTxdata{
			Hash:          rawhash.String(),
			Block:         block.Header.Number.String(),
			Idx:           uint64(ipos),
			FromAddr:      fromStr,
			ToAddr:        toStr,
			Nonce:         rawtx.Nonce(),
			Serialization: txdata,
			CreatedAt:     ctx.Time(),
		}
		err = ctx.Persist().InsertCvmTransactionsTxdata(ctx.Ctx(), ctx.DB(), cvmTransactionTxdata, cfg.PerformUpdates)
		if err != nil {
			return err
		}
	}

	for _, txIDString := range txIDs {
		cvmTransaction := &db.CvmTransactionsAtomic{
			TransactionID: txIDString,
			Block:         block.Header.Number.String(),
			ChainID:       ctx.ChainID(),
			Type:          typ,
			CreatedAt:     ctx.Time(),
		}
		err = ctx.Persist().InsertCvmTransactionsAtomic(ctx.Ctx(), ctx.DB(), cvmTransaction, cfg.PerformUpdates)
		if err != nil {
			return err
		}
	}

	blockBytes, err := json.Marshal(&block.Header)
	if err != nil {
		return err
	}

	cvmBlocks := &db.CvmBlocks{
		Block:         block.Header.Number.String(),
		Hash:          block.Header.Hash().String(),
		ChainID:       ctx.ChainID(),
		EvmTx:         int16(len(block.Txs)),
		AtomicTx:      int16(len(txIDs)),
		Serialization: blockBytes,
		CreatedAt:     ctx.Time(),
	}
	err = ctx.Persist().InsertCvmBlocks(ctx.Ctx(), ctx.DB(), cvmBlocks)
	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) indexTransaction(
	ctx services.ConsumerCtx,
	id ids.ID,
	typ models.CChainType,
	blockChainID ids.ID,
	txFee uint64,
	unsignedBytes []byte,
) error {
	avmTxtype := ""
	switch typ {
	case models.CChainImport:
		avmTxtype = "atomic_import_tx"
	case models.CChainExport:
		avmTxtype = "atomic_export_tx"
	}

	return w.djtx.InsertTransactionBase(
		ctx,
		id,
		blockChainID.String(),
		avmTxtype,
		[]byte(""),
		unsignedBytes,
		txFee,
		false,
		w.networkID,
	)
}

func (w *Writer) insertAddress(
	typ models.CChainType,
	ctx services.ConsumerCtx,
	idx uint64,
	id ids.ID,
	address common.Address,
	assetID ids.ID,
	amount uint64,
	nonce uint64,
) error {
	idprefix := id.Prefix(idx)

	cvmAddress := &db.CvmAddresses{
		ID:            idprefix.String(),
		Type:          typ,
		Idx:           idx,
		TransactionID: id.String(),
		Address:       address.String(),
		AssetID:       assetID.String(),
		Amount:        amount,
		Nonce:         nonce,
		CreatedAt:     ctx.Time(),
	}
	return ctx.Persist().InsertCvmAddresses(ctx.Ctx(), ctx.DB(), cvmAddress, cfg.PerformUpdates)
}

func (w *Writer) indexExportTx(ctx services.ConsumerCtx, txID ids.ID, tx *evm.UnsignedExportTx, blockBytes []byte) error {
	var err error

	var totalin uint64
	for icnt, in := range tx.Ins {
		icntval := uint64(icnt)
		err = w.insertAddress(models.CChainIn, ctx, icntval, txID, in.Address, in.AssetID, in.Amount, in.Nonce)
		if err != nil {
			return err
		}
		if in.AssetID == w.djtxAssetID {
			totalin, err = math.Add64(totalin, in.Amount)
			if err != nil {
				return err
			}
		}
	}

	var totalout uint64
	var idx uint32
	for _, out := range tx.ExportedOutputs {
		totalout, err = w.djtx.InsertTransactionOuts(idx, ctx, totalout, out, txID, tx.DestinationChain.String(), false, false)
		if err != nil {
			return err
		}
		idx++
	}

	return w.indexTransaction(ctx, txID, models.CChainExport, tx.BlockchainID, totalin-totalout, blockBytes)
}

func (w *Writer) indexImportTx(ctx services.ConsumerCtx, txID ids.ID, tx *evm.UnsignedImportTx, creds []verify.Verifiable, blockBytes []byte, unsignedBytes []byte) error {
	var err error

	var totalout uint64
	for icnt, out := range tx.Outs {
		icntval := uint64(icnt)
		err = w.insertAddress(models.CchainOut, ctx, icntval, txID, out.Address, out.AssetID, out.Amount, 0)
		if err != nil {
			return err
		}
		if out.AssetID == w.djtxAssetID {
			totalout, err = math.Add64(totalout, out.Amount)
			if err != nil {
				return err
			}
		}
	}

	var totalin uint64
	for inidx, in := range tx.ImportedInputs {
		totalin, err = w.djtx.InsertTransactionIns(inidx, ctx, totalin, in, txID, creds, unsignedBytes, tx.SourceChain.String())
		if err != nil {
			return err
		}
	}

	return w.indexTransaction(ctx, txID, models.CChainImport, tx.BlockchainID, totalin-totalout, blockBytes)
}
