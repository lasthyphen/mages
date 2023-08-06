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
	"testing"

	"github.com/lasthyphen/mages/models"
)

func TestReaderAggregate(t *testing.T) {
	reader := &ReaderAggregateTxList{}
	var txs []*models.Transaction
	txs = append(txs, &models.Transaction{ID: "1"})
	reader.Set(txs)
	ftxs := reader.FindTxs(nil, 1)
	if len(ftxs) != 1 {
		t.Fatal("found len(txs) != 1")
	}
	if ftxs[0].ID != "1" {
		t.Fatal("found ftxs[0].ID != '1'")
	}
	ftx := reader.First()
	if ftx == nil {
		t.Fatal("first tx is nil")
	}
	if ftx.ID != "1" {
		t.Fatal("first tx.ID != '1'")
	}
	ftx, _ = reader.Get("1")
	if ftx == nil {
		t.Fatal("first tx is nil")
	}
	if ftx.ID != "1" {
		t.Fatal("first tx.ID != '1'")
	}
}
