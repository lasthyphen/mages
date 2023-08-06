// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file is a derived work, based on ava-labs code.
//
// It is distributed under the same license conditions as the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********************************************************

package pvm

import (
	"context"
	"testing"
	"time"

	"github.com/lasthyphen/dijetsnodego/ids"
	"github.com/lasthyphen/dijetsnodego/utils/logging"
	"github.com/lasthyphen/dijetsnodego/vms/platformvm"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services"
	"github.com/lasthyphen/mages/services/indexes/djtx"
	"github.com/lasthyphen/mages/services/indexes/params"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/utils"
)

var (
	testXChainID = ids.ID([32]byte{7, 193, 50, 215, 59, 55, 159, 112, 106, 206, 236, 110, 229, 14, 139, 125, 14, 101, 138, 65, 208, 44, 163, 38, 115, 182, 177, 179, 244, 34, 195, 120})
)

func TestBootstrap(t *testing.T) {
	conns, w, r, closeFn := newTestIndex(t, 12345, ChainID)
	defer closeFn()

	persist := db.NewPersist()
	if err := w.Bootstrap(context.Background(), conns, persist); err != nil {
		t.Fatal(err)
	}

	txList, err := r.ListTransactions(context.Background(), &params.ListTransactionsParams{
		ChainIDs: []string{ChainID.String()},
	}, ids.Empty)
	if err != nil {
		t.Fatal("Failed to list transactions:", err.Error())
	}

	if txList == nil || *txList.Count < 1 {
		if txList.Count == nil {
			t.Fatal("Incorrect number of transactions:", txList.Count)
		} else {
			t.Fatal("Incorrect number of transactions:", *txList.Count)
		}
	}
}

func newTestIndex(t *testing.T, networkID uint32, chainID ids.ID) (*utils.Connections, *Writer, *djtx.Reader, func()) {
	logConf := logging.DefaultConfig

	conf := cfg.Services{
		Logging: logConf,
		DB: &cfg.DB{
			Driver: "mysql",
			DSN:    "root:password@tcp(127.0.0.1:3306)/magellan_test?parseTime=true",
		},
	}

	sc := &servicesctrl.Control{Log: logging.NoLog{}, Services: conf}
	conns, err := sc.Database()
	if err != nil {
		t.Fatal("Failed to create connections:", err.Error())
	}

	// Create index
	writer, err := NewWriter(networkID, chainID.String())
	if err != nil {
		t.Fatal("Failed to create writer:", err.Error())
	}

	cmap := make(map[string]services.Consumer)
	reader, _ := djtx.NewReader(networkID, conns, cmap, nil, sc)
	return conns, writer, reader, func() {
		_ = conns.Close()
	}
}

func TestInsertTxInternal(t *testing.T) {
	conns, writer, _, closeFn := newTestIndex(t, 5, testXChainID)
	defer closeFn()
	ctx := context.Background()

	tx := platformvm.Tx{}
	validatorTx := &platformvm.UnsignedAddValidatorTx{}
	tx.UnsignedTx = validatorTx

	persist := db.NewPersistMock()
	session, _ := conns.DB().NewSession("pvm_test_tx", cfg.RequestTimeout)
	cCtx := services.NewConsumerContext(ctx, session, time.Now().Unix(), 0, persist, testXChainID.String())
	err := writer.indexTransaction(cCtx, tx.ID(), tx, false)
	if err != nil {
		t.Fatal("insert failed", err)
	}
	if len(persist.Transactions) != 1 {
		t.Fatal("insert failed")
	}
	if len(persist.TransactionsBlock) != 1 {
		t.Fatal("insert failed")
	}
	if len(persist.TransactionsValidator) != 1 {
		t.Fatal("insert failed")
	}
}

func TestInsertTxInternalRewards(t *testing.T) {
	conns, writer, _, closeFn := newTestIndex(t, 5, testXChainID)
	defer closeFn()
	ctx := context.Background()

	tx := platformvm.Tx{}
	validatorTx := &platformvm.UnsignedRewardValidatorTx{}
	tx.UnsignedTx = validatorTx

	persist := db.NewPersistMock()
	session, _ := conns.DB().NewSession("pvm_test_tx", cfg.RequestTimeout)
	cCtx := services.NewConsumerContext(ctx, session, time.Now().Unix(), 0, persist, testXChainID.String())
	err := writer.indexTransaction(cCtx, tx.ID(), tx, false)
	if err != nil {
		t.Fatal("insert failed", err)
	}
	if len(persist.Transactions) != 0 {
		t.Fatal("insert failed")
	}
	if len(persist.TransactionsBlock) != 0 {
		t.Fatal("insert failed")
	}
	if len(persist.TransactionsValidator) != 0 {
		t.Fatal("insert failed")
	}
	if len(persist.Rewards) != 1 {
		t.Fatal("insert failed")
	}
}

func TestCommonBlock(t *testing.T) {
	conns, writer, _, closeFn := newTestIndex(t, 5, testXChainID)
	defer closeFn()
	ctx := context.Background()

	tx := platformvm.CommonBlock{}
	blkid := ids.ID{}

	persist := db.NewPersistMock()
	session, _ := conns.DB().NewSession("pvm_test_tx", cfg.RequestTimeout)
	cCtx := services.NewConsumerContext(ctx, session, time.Now().Unix(), 0, persist, testXChainID.String())
	err := writer.indexCommonBlock(cCtx, blkid, models.BlockTypeCommit, tx, []byte(""))
	if err != nil {
		t.Fatal("insert failed", err)
	}
	if len(persist.PvmBlocks) != 1 {
		t.Fatal("insert failed")
	}
}
