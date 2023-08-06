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

package servicesctrl

import (
	"time"

	"github.com/lasthyphen/dijetsnodego/utils/logging"
	"github.com/lasthyphen/mages/caching"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/utils"

	avlancheGoUtils "github.com/lasthyphen/dijetsnodego/utils"
)

const (
	MetricProduceProcessedCountKey = "produce_records_processed"
	MetricProduceSuccessCountKey   = "produce_records_success"
	MetricProduceFailureCountKey   = "produce_records_failure"

	MetricConsumeProcessedCountKey       = "consume_records_processed"
	MetricConsumeProcessMillisCounterKey = "consume_records_process_millis"
	MetricConsumeSuccessCountKey         = "consume_records_success"
	MetricConsumeFailureCountKey         = "consume_records_failure"
)

type LocalTxPoolJob struct {
	TxPool *db.TxPool
	Errs   *avlancheGoUtils.AtomicInterface
}

type Control struct {
	Services                   cfg.Services
	ServicesCfg                cfg.Config
	Chains                     map[string]cfg.Chain `json:"chains"`
	Log                        logging.Logger
	Persist                    db.Persist
	Features                   map[string]struct{}
	BalanceManager             utils.ExecIface
	GenesisContainer           *utils.GenesisContainer
	IsAccumulateBalanceIndexer bool
	IsAccumulateBalanceReader  bool
	IsDisableBootstrap         bool
	IsAggregateCache           bool
	IndexedList                utils.IndexedList
	LocalTxPool                chan *LocalTxPoolJob
	AggregatesCache            caching.AggregatesCache
}

func (s *Control) Logger() logging.Logger {
	return s.Log
}

func (s *Control) Init(networkID uint32) error {
	s.AggregatesCache.InitCacheStorage(s.Chains)
	s.IndexedList = utils.NewIndexedList(cfg.MaxSizedList)
	s.LocalTxPool = make(chan *LocalTxPoolJob, cfg.MaxTxPoolSize)

	if _, ok := s.Features["accumulate_balance_indexer"]; ok {
		s.Log.Info("enable feature accumulate_balance_indexer")
		s.IsAccumulateBalanceIndexer = true

		// reader will work only if we enable indexer.
		if _, ok := s.Features["accumulate_balance_reader"]; ok {
			s.Log.Info("enable feature accumulate_balance_reader")
			s.IsAccumulateBalanceReader = true
		}
	}
	if _, ok := s.Features["disable_bootstrap"]; ok {
		s.IsDisableBootstrap = true
	}
	if _, ok := s.Features["aggregate_cache"]; ok {
		s.IsAggregateCache = true
	}
	var err error
	s.GenesisContainer, err = utils.NewGenesisContainer(networkID)
	if err != nil {
		return err
	}

	return nil
}

func (s *Control) StartCacheScheduler(config *cfg.Config) error {
	// create new database connection
	connections, err := s.DatabaseRO()
	if err != nil {
		return err
	}

	MyTimer := time.NewTimer(time.Duration(config.CacheUpdateInterval) * time.Second)

	for range MyTimer.C {
		MyTimer.Stop()
		toDate := time.Now()
		yesterdayDateTime := time.Now().AddDate(0, 0, -1)
		prevWeekDateTime := time.Now().AddDate(0, 0, -7)
		prevMonthDateTime := time.Now().AddDate(0, -1, 0)

		// update cache for all chains
		for id := range config.Chains {
			// previous day aggregate number
			err := s.AggregatesCache.GetAggregatesAndUpdate(s.Chains, connections, id, yesterdayDateTime, toDate, "day")
			if err != nil {
				return err
			}
			err = s.AggregatesCache.GetAggregatesFeesAndUpdate(s.Chains, connections, id, yesterdayDateTime, toDate, "day")
			if err != nil {
				return err
			}
			// previous week aggregate number
			err = s.AggregatesCache.GetAggregatesAndUpdate(s.Chains, connections, id, prevWeekDateTime, toDate, "week")
			if err != nil {
				return err
			}
			err = s.AggregatesCache.GetAggregatesFeesAndUpdate(s.Chains, connections, id, prevWeekDateTime, toDate, "week")
			if err != nil {
				return err
			}
			// previous month aggregate number
			err = s.AggregatesCache.GetAggregatesAndUpdate(s.Chains, connections, id, prevMonthDateTime, toDate, "month")
			if err != nil {
				return err
			}
			err = s.AggregatesCache.GetAggregatesFeesAndUpdate(s.Chains, connections, id, prevMonthDateTime, toDate, "month")
			if err != nil {
				return err
			}
		}

		MyTimer.Reset(time.Duration(config.CacheUpdateInterval) * time.Second)
	}
	return nil
}

func (s *Control) InitProduceMetrics() {
	utils.Prometheus.CounterInit(MetricProduceProcessedCountKey, "records processed")
	utils.Prometheus.CounterInit(MetricProduceSuccessCountKey, "records success")
	utils.Prometheus.CounterInit(MetricProduceFailureCountKey, "records failure")
}

func (s *Control) InitConsumeMetrics() {
	utils.Prometheus.CounterInit(MetricConsumeProcessedCountKey, "records processed")
	utils.Prometheus.CounterInit(MetricConsumeProcessMillisCounterKey, "records processed millis")
	utils.Prometheus.CounterInit(MetricConsumeSuccessCountKey, "records success")
	utils.Prometheus.CounterInit(MetricConsumeFailureCountKey, "records failure")
}

func (s *Control) Database() (*utils.Connections, error) {
	c, err := utils.NewDBFromConfig(s.Services, false)
	if err != nil {
		return nil, err
	}
	c.DB().SetMaxIdleConns(32)
	c.DB().SetConnMaxIdleTime(10 * time.Second)
	c.Eventer.SetLog(s.Log)
	return c, nil
}

func (s *Control) DatabaseRO() (*utils.Connections, error) {
	c, err := utils.NewDBFromConfig(s.Services, true)
	if err != nil {
		return nil, err
	}
	c.DB().SetMaxIdleConns(32)
	c.DB().SetConnMaxIdleTime(10 * time.Second)
	c.Eventer.SetLog(s.Log)
	return c, nil
}

func (s *Control) Enqueue(pool *db.TxPool) {
	select {
	case s.LocalTxPool <- &LocalTxPoolJob{TxPool: pool}:
	default:
	}
}
