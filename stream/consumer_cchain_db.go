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

package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lasthyphen/dijetsnodego/utils/hashing"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/modelsc"
	"github.com/lasthyphen/mages/services"
	"github.com/lasthyphen/mages/services/indexes/cvm"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/utils"
)

type consumerCChainDB struct {
	id string
	sc *servicesctrl.Control

	// metrics
	metricProcessedCountKey       string
	metricProcessMillisCounterKey string
	metricSuccessCountKey         string
	metricFailureCountKey         string

	conf cfg.Config

	// Concurrency control
	quitCh   chan struct{}
	consumer *cvm.Writer

	topicName     string
	topicRcptName string
}

func NewConsumerCChainDB() ProcessorFactoryInstDB {
	return func(sc *servicesctrl.Control, conf cfg.Config) (ProcessorDB, error) {
		c := &consumerCChainDB{
			conf:                          conf,
			sc:                            sc,
			metricProcessedCountKey:       fmt.Sprintf("consume_records_processed_%s_cchain", conf.CchainID),
			metricProcessMillisCounterKey: fmt.Sprintf("consume_records_process_millis_%s_cchain", conf.CchainID),
			metricSuccessCountKey:         fmt.Sprintf("consume_records_success_%s_cchain", conf.CchainID),
			metricFailureCountKey:         fmt.Sprintf("consume_records_failure_%s_cchain", conf.CchainID),
			id:                            fmt.Sprintf("consumer %d %s cchain", conf.NetworkID, conf.CchainID),

			quitCh: make(chan struct{}),
		}
		utils.Prometheus.CounterInit(c.metricProcessedCountKey, "records processed")
		utils.Prometheus.CounterInit(c.metricProcessMillisCounterKey, "records processed millis")
		utils.Prometheus.CounterInit(c.metricSuccessCountKey, "records success")
		utils.Prometheus.CounterInit(c.metricFailureCountKey, "records failure")
		sc.InitConsumeMetrics()

		var err error
		c.consumer, err = cvm.NewWriter(c.conf.NetworkID, c.conf.CchainID)
		if err != nil {
			_ = c.Close()
			return nil, err
		}

		c.topicName = fmt.Sprintf("%d-%s-cchain", c.conf.NetworkID, c.conf.CchainID)
		c.topicRcptName = fmt.Sprintf("%d-%s-cchain-trc", c.conf.NetworkID, c.conf.CchainID)

		return c, nil
	}
}

// Close shuts down the producer
func (c *consumerCChainDB) Close() error {
	return nil
}

func (c *consumerCChainDB) ID() string {
	return c.id
}

func (c *consumerCChainDB) Topic() []string {
	return []string{c.topicName, c.topicRcptName}
}

func (c *consumerCChainDB) Process(conns *utils.Connections, row *db.TxPool) error {
	switch row.Topic {
	case c.topicName:
		msg := &Message{
			id:         row.MsgKey,
			chainID:    c.conf.CchainID,
			body:       row.Serialization,
			timestamp:  row.CreatedAt.UTC().Unix(),
			nanosecond: int64(row.CreatedAt.UTC().Nanosecond()),
		}
		return c.Consume(conns, msg)
	case c.topicRcptName:
		msg := &Message{
			id:         row.MsgKey,
			chainID:    c.conf.CchainID,
			body:       row.Serialization,
			timestamp:  row.CreatedAt.UTC().Unix(),
			nanosecond: int64(row.CreatedAt.UTC().Nanosecond()),
		}
		return c.ConsumeReceipt(conns, msg)
	default:
		return nil
	}
}

func (c *consumerCChainDB) ConsumeReceipt(conns *utils.Connections, msg services.Consumable) error {
	transactionReceipt := &modelsc.TransactionReceipt{}
	err := json.Unmarshal(msg.Body(), transactionReceipt)
	if err != nil {
		return err
	}
	collectors := utils.NewCollectors(
		utils.NewCounterIncCollect(c.metricProcessedCountKey),
		utils.NewCounterObserveMillisCollect(c.metricProcessMillisCounterKey),
		utils.NewCounterIncCollect(servicesctrl.MetricConsumeProcessedCountKey),
		utils.NewCounterObserveMillisCollect(servicesctrl.MetricConsumeProcessMillisCounterKey),
	)
	defer func() {
		err := collectors.Collect()
		if err != nil {
			c.sc.Log.Error("collectors.Collect: %s", err)
		}
	}()

	id := hashing.ComputeHash256(transactionReceipt.Receipt)

	nmsg := NewMessage(string(id), msg.ChainID(), transactionReceipt.Receipt, msg.Timestamp(), msg.Nanosecond())

	rsleep := utils.NewRetrySleeper(1, 100*time.Millisecond, time.Second)
	for {
		err = c.persistConsumeReceipt(conns, nmsg, transactionReceipt)
		if !utils.ErrIsLockError(err) {
			break
		}
		rsleep.Inc()
	}

	if err != nil {
		c.Failure()
		collectors.Error()
		c.sc.Log.Error("consumer.Consume: %s", err)
		return err
	}
	c.Success()

	return nil
}

func (c *consumerCChainDB) Consume(conns *utils.Connections, msg services.Consumable) error {
	block, err := modelsc.Unmarshal(msg.Body())
	if err != nil {
		return err
	}

	collectors := utils.NewCollectors(
		utils.NewCounterIncCollect(c.metricProcessedCountKey),
		utils.NewCounterObserveMillisCollect(c.metricProcessMillisCounterKey),
		utils.NewCounterIncCollect(servicesctrl.MetricConsumeProcessedCountKey),
		utils.NewCounterObserveMillisCollect(servicesctrl.MetricConsumeProcessMillisCounterKey),
	)
	defer func() {
		err := collectors.Collect()
		if err != nil {
			c.sc.Log.Error("collectors.Collect: %s", err)
		}
	}()

	if block.BlockExtraData == nil {
		block.BlockExtraData = []byte("")
	}

	id := hashing.ComputeHash256(block.BlockExtraData)
	nmsg := NewMessage(string(id), msg.ChainID(), []byte{}, msg.Timestamp(), msg.Nanosecond())

	rsleep := utils.NewRetrySleeper(1, 100*time.Millisecond, time.Second)
	for {
		err = c.persistConsume(conns, nmsg, block)
		if !utils.ErrIsLockError(err) {
			break
		}
		rsleep.Inc()
	}

	if err != nil {
		c.Failure()
		collectors.Error()
		c.sc.Log.Error("consumer.Consume: %s", err)
		return err
	}
	c.Success()

	c.sc.BalanceManager.Exec()

	return nil
}

func (c *consumerCChainDB) persistConsumeReceipt(conns *utils.Connections, msg services.Consumable, transactionTrace *modelsc.TransactionReceipt) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), cfg.DefaultConsumeProcessWriteTimeout)
	defer cancelFn()
	return c.consumer.ConsumeReceipt(ctx, conns, msg, transactionTrace, c.sc.Persist)
}

func (c *consumerCChainDB) persistConsume(conns *utils.Connections, msg services.Consumable, block *modelsc.Block) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), cfg.DefaultConsumeProcessWriteTimeout)
	defer cancelFn()
	return c.consumer.Consume(ctx, conns, msg, block, c.sc.Persist)
}

func (c *consumerCChainDB) Failure() {
	_ = utils.Prometheus.CounterInc(c.metricFailureCountKey)
	_ = utils.Prometheus.CounterInc(servicesctrl.MetricConsumeFailureCountKey)
}

func (c *consumerCChainDB) Success() {
	_ = utils.Prometheus.CounterInc(c.metricSuccessCountKey)
	_ = utils.Prometheus.CounterInc(servicesctrl.MetricConsumeSuccessCountKey)
}
