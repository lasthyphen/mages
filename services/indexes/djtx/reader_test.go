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

package djtx

import (
	"context"
	"testing"
	"time"

	"github.com/lasthyphen/dijetsnodego/utils/logging"
	"github.com/lasthyphen/mages/caching"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services"
	"github.com/lasthyphen/mages/services/indexes/params"
	"github.com/lasthyphen/mages/servicesctrl"
)

func TestCollectInsAndOuts(t *testing.T) {
	reader, closeFn := newTestIndex(t)
	defer closeFn()

	ctx := newTestContext()
	session, _ := reader.conns.DB().NewSession("reader_test_tx", cfg.RequestTimeout)

	_, _ = session.DeleteFrom("avm_outputs").ExecContext(ctx)
	_, _ = session.DeleteFrom("avm_output_addresses").ExecContext(ctx)
	_, _ = session.DeleteFrom("avm_outputs_redeeming").ExecContext(ctx)

	inputID := "in1"
	outputID := "out1"
	chainID := "ch1"
	txID := "tx1"
	intxID := "tx0"
	address := "addr1"
	idx := uint32(0)
	assetID := "assid1"
	outputType := models.OutputTypesSECP2556K1Transfer
	amount := uint64(1)
	locktime := uint64(0)
	thresholD := uint32(0)
	groupID := uint32(0)
	payload := []byte("")
	stakeLocktime := uint64(99991)
	tm := time.Now().Truncate(1 * time.Hour)

	inputIDUnmatched := "inu"

	persist := db.NewPersist()

	outputs := &db.Outputs{
		ID:            outputID,
		ChainID:       chainID,
		TransactionID: txID,
		OutputIndex:   idx,
		AssetID:       assetID,
		OutputType:    outputType,
		Amount:        amount,
		Locktime:      locktime,
		Threshold:     thresholD,
		GroupID:       groupID,
		Payload:       payload,
		StakeLocktime: stakeLocktime,
		CreatedAt:     tm,
	}
	_ = persist.InsertOutputs(ctx, session, outputs, false)

	outputAddresses := &db.OutputAddresses{
		OutputID:  outputID,
		Address:   address,
		CreatedAt: tm,
		UpdatedAt: time.Now().UTC(),
	}
	_ = persist.InsertOutputAddresses(ctx, session, outputAddresses, false)

	outputsRedeeming := &db.OutputsRedeeming{
		ID:                     inputID,
		RedeemedAt:             tm,
		RedeemingTransactionID: txID,
		Amount:                 amount,
		OutputIndex:            idx,
		Intx:                   intxID,
		AssetID:                assetID,
		CreatedAt:              tm,
	}
	_ = persist.InsertOutputsRedeeming(ctx, session, outputsRedeeming, false)

	outputsRedeeming = &db.OutputsRedeeming{
		ID:                     inputIDUnmatched,
		RedeemedAt:             tm,
		RedeemingTransactionID: txID,
		Amount:                 amount,
		OutputIndex:            idx,
		Intx:                   intxID,
		AssetID:                assetID,
		CreatedAt:              tm,
	}
	_ = persist.InsertOutputsRedeeming(ctx, session, outputsRedeeming, false)

	records, _ := collectInsAndOuts(ctx, session, []models.StringID{models.StringID(txID)})

	if len(records) != 3 {
		t.Error("invalid input/outputs")
	}

	if records[0].Output.ID != models.StringID(outputID) &&
		records[1].ID != models.StringID(inputID) &&
		records[2].ID != models.StringID(inputIDUnmatched) {
		t.Error("invalid input/outputs")
	}

	if records[0].Output.OutputType != models.OutputTypesSECP2556K1Transfer &&
		records[1].Output.OutputType != 0 &&
		records[2].Output.OutputType != 0 {
		t.Error("invalid output type")
	}

	if records[0].Output.StakeLocktime != stakeLocktime &&
		records[1].Output.StakeLocktime != 0 &&
		records[2].Output.StakeLocktime != 0 {
		t.Error("invalid stake locktime")
	}
}

func TestAggregates(t *testing.T) {
	reader, closeFn := newTestIndex(t)
	defer closeFn()

	ctx := newTestContext()

	persist := db.NewPersist()

	sessTx, _ := reader.conns.DB().NewSession("test_aggregate_tx_fee", cfg.RequestTimeout)
	_, _ = sessTx.DeleteFrom("avm_transactions").ExecContext(ctx)

	sessOuts, _ := reader.conns.DB().NewSession("test_aggregate_tx", cfg.RequestTimeout)
	_, _ = sessOuts.DeleteFrom("avm_outputs").ExecContext(ctx)

	timeNow := time.Now().UTC().Truncate(1 * time.Second)
	yesterdayDateTime := time.Now().UTC().AddDate(0, 0, -1)
	prevWeekDateTime := time.Now().UTC().AddDate(0, 0, -7)
	prevMonthDateTime := time.Now().UTC().AddDate(0, -1, 0)

	// Add transaction and output to test last days' aggregates
	transaction := &db.Transactions{
		ID:        "id1",
		ChainID:   "cid",
		Type:      "type",
		Txfee:     10,
		CreatedAt: timeNow.Add(-1 * time.Hour),
	}
	_ = persist.InsertTransactions(ctx, sessTx, transaction, false)
	output := &db.Outputs{
		ID:            "id1",
		TransactionID: "id1",
		ChainID:       "cid",
		CreatedAt:     timeNow.Add(-1 * time.Hour),
	}
	_ = persist.InsertOutputs(ctx, sessOuts, output, false)

	err := reader.sc.AggregatesCache.GetAggregatesFeesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		yesterdayDateTime,
		timeNow.Add(1*time.Hour),
		"day")
	if err != nil {
		t.Error("error", err)
	}
	err = reader.sc.AggregatesCache.GetAggregatesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		yesterdayDateTime,
		timeNow.Add(1*time.Hour),
		"day")
	if err != nil {
		t.Error("error", err)
	}

	// Add transaction and output to test last week's aggregates
	transaction = &db.Transactions{
		ID:        "id2",
		ChainID:   "cid",
		Type:      "type",
		Txfee:     15,
		CreatedAt: timeNow.Add(-48 * time.Hour),
	}
	_ = persist.InsertTransactions(ctx, sessTx, transaction, false)
	output = &db.Outputs{
		ID:            "id2",
		TransactionID: "id2",
		ChainID:       "cid",
		CreatedAt:     timeNow.Add(-48 * time.Hour),
	}
	_ = persist.InsertOutputs(ctx, sessOuts, output, false)

	err = reader.sc.AggregatesCache.GetAggregatesFeesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		prevWeekDateTime,
		timeNow.Add(1*time.Hour),
		"week")
	if err != nil {
		t.Error("error", err)
	}
	err = reader.sc.AggregatesCache.GetAggregatesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		prevWeekDateTime,
		timeNow.Add(1*time.Hour),
		"week")
	if err != nil {
		t.Error("error", err)
	}

	// Add transaction and output to test last month's aggregates
	transaction = &db.Transactions{
		ID:        "id3",
		ChainID:   "cid",
		Type:      "type",
		Txfee:     20,
		CreatedAt: timeNow.Add(-200 * time.Hour),
	}
	_ = persist.InsertTransactions(ctx, sessTx, transaction, false)
	output = &db.Outputs{
		ID:            "id3",
		TransactionID: "id3",
		ChainID:       "cid",
		CreatedAt:     timeNow.Add(-200 * time.Hour),
	}
	_ = persist.InsertOutputs(ctx, sessOuts, output, false)

	err = reader.sc.AggregatesCache.GetAggregatesFeesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		prevMonthDateTime,
		timeNow.Add(1*time.Hour),
		"month")
	if err != nil {
		t.Error("error", err)
	}
	err = reader.sc.AggregatesCache.GetAggregatesAndUpdate(
		reader.sc.Chains,
		reader.conns,
		"cid",
		prevMonthDateTime,
		timeNow.Add(1*time.Hour),
		"month")
	if err != nil {
		t.Error("error", err)
	}

	// Test last month's aggregates
	startTime := timeNow.Add(-250 * time.Hour)
	endTime := timeNow.Add(1 * time.Hour)

	feeAggregateParams := params.TxfeeAggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	aggFees, err := reader.TxfeeAggregate(reader.sc.AggregatesCache, &feeAggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if aggFees.TxfeeAggregates.Txfee != 45 {
		t.Errorf("Expected %d tx aggregate fees", 45)
	}
	if aggFees.StartTime != startTime || aggFees.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}
	aggregateParams := params.AggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	agg, err := reader.Aggregate(reader.sc.AggregatesCache, &aggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if agg.Aggregates.TransactionCount != 3 {
		t.Errorf("Expected %d txs", 3)
	}
	if agg.StartTime != startTime || agg.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}

	// Test last week's aggregate fees
	startTime = timeNow.Add(-50 * time.Hour)
	endTime = timeNow.Add(1 * time.Hour)

	feeAggregateParams = params.TxfeeAggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	aggFees, err = reader.TxfeeAggregate(reader.sc.AggregatesCache, &feeAggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if aggFees.TxfeeAggregates.Txfee != 25 {
		t.Errorf("Expected %d tx aggregate fees", 25)
	}
	if aggFees.StartTime != startTime || aggFees.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}
	aggregateParams = params.AggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	agg, err = reader.Aggregate(reader.sc.AggregatesCache, &aggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if agg.Aggregates.TransactionCount != 2 {
		t.Errorf("Expected %d txs", 2)
	}
	if agg.StartTime != startTime || agg.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}

	// Test last days' aggregate fees
	startTime = timeNow.Add(-5 * time.Hour)
	endTime = timeNow.Add(1 * time.Hour)

	feeAggregateParams = params.TxfeeAggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	aggFees, err = reader.TxfeeAggregate(reader.sc.AggregatesCache, &feeAggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if aggFees.TxfeeAggregates.Txfee != 10 {
		t.Errorf("Expected %d tx aggregate fees", 10)
	}
	if aggFees.StartTime != startTime || aggFees.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}
	aggregateParams = params.AggregateParams{
		ListParams: params.ListParams{StartTime: startTime, EndTime: endTime},
		ChainIDs:   []string{"cid"},
	}
	agg, err = reader.Aggregate(reader.sc.AggregatesCache, &aggregateParams)
	if err != nil {
		t.Error("error", err)
	}
	if agg.Aggregates.TransactionCount != 1 {
		t.Errorf("Expected %d txs", 1)
	}
	if agg.StartTime != startTime || agg.EndTime != endTime {
		t.Error("aggregate tx invalid")
	}
}

func newTestIndex(t *testing.T) (*Reader, func()) {
	logConf := logging.Config{
		DisplayLevel: logging.Info,
		LogLevel:     logging.Debug,
	}

	conf := cfg.Services{
		Logging: logConf,
		DB: &cfg.DB{
			Driver: "mysql",
			DSN:    "root:password@tcp(127.0.0.1:3306)/magellan_test?parseTime=true",
		},
	}

	chains := cfg.Chains{
		"cid": {
			ID:     "cid",
			VMType: models.AVMName,
		},
	}

	sc := &servicesctrl.Control{Log: logging.NoLog{}, Services: conf, Chains: chains}
	sc.AggregatesCache = caching.NewAggregatesCache()
	sc.AggregatesCache.InitCacheStorage(chains)
	conns, err := sc.Database()
	if err != nil {
		t.Fatal("Failed to create connections:", err.Error())
	}

	cmap := make(map[string]services.Consumer)
	reader, _ := NewReader(5, conns, cmap, sc)
	return reader, func() {
		_ = conns.Close()
	}
}

func newTestContext() context.Context {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	time.AfterFunc(5*time.Second, cancelFn)
	return ctx
}
