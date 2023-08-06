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

package consumers

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"

	avlancheGoUtils "github.com/lasthyphen/dijetsnodego/utils"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services"
	"github.com/lasthyphen/mages/services/indexes/avm"
	"github.com/lasthyphen/mages/services/indexes/cvm"
	"github.com/lasthyphen/mages/services/indexes/pvm"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/stream"
	"github.com/lasthyphen/mages/utils"
)

const (
	MaximumRecordsRead = 10000
	MaxTheads          = 1

	IteratorTimeout = 3 * time.Minute
)

type ConsumerFactory func(uint32, string, string, *cfg.Config) (services.Consumer, error)

var IndexerConsumer = func(networkID uint32, chainVM string, chainID string, conf *cfg.Config) (indexer services.Consumer, err error) {
	switch chainVM {
	case models.AVMName:
		indexer, err = avm.NewWriter(networkID, chainID)
	case models.PVMName:
		indexer, err = pvm.NewWriter(networkID, chainID)
	case models.CVMName:
		indexer, err = cvm.NewWriter(networkID, chainID, conf)
	default:
		return nil, stream.ErrUnknownVM
	}
	return indexer, err
}

type ConsumerDBFactory func(uint32, string, string) (stream.ProcessorFactoryChainDB, error)

var (
	IndexerDB          = stream.NewConsumerDBFactory(IndexerConsumer, stream.EventTypeDecisions)
	IndexerConsensusDB = stream.NewConsumerDBFactory(IndexerConsumer, stream.EventTypeConsensus)
)

func Bootstrap(sc *servicesctrl.Control, networkID uint32, conf *cfg.Config, factories []ConsumerFactory) error {
	if sc.IsDisableBootstrap {
		return nil
	}

	conns, err := sc.Database()
	if err != nil {
		return err
	}
	defer func() {
		_ = conns.Close()
	}()

	persist := db.NewPersist()
	ctx := context.Background()
	job := conns.Stream().NewJob("bootstrap-key-value")
	sess := conns.DB().NewSessionForEventReceiver(job)

	bootstrapValue := "true"

	// check if we have bootstrapped..
	keyValueStore := &db.KeyValueStore{
		K: utils.KeyValueBootstrap,
	}
	keyValueStore, _ = persist.QueryKeyValueStore(ctx, sess, keyValueStore)
	if keyValueStore.V == bootstrapValue {
		sc.Log.Info("skipping bootstrap")
		return nil
	}

	errs := avlancheGoUtils.AtomicInterface{}

	wg := sync.WaitGroup{}
	for _, chain := range conf.Chains {
		for _, factory := range factories {
			bootstrapfactory, err := factory(networkID, chain.VMType, chain.ID, conf)
			if err != nil {
				return err
			}
			sc.Log.Info("starting bootstrap",
				zap.Uint32("networkID", networkID),
				zap.String("vmType", chain.VMType),
				zap.String("chainID", chain.ID),
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				err = bootstrapfactory.Bootstrap(ctx, conns, sc.Persist)
				if err != nil {
					errs.SetValue(err)
				}
				sc.Log.Info("finished bootstrap",
					zap.Uint32("networkID", networkID),
					zap.String("vmType", chain.VMType),
					zap.String("chainID", chain.ID),
				)
			}()
		}
	}

	wg.Wait()

	if errs.GetValue() != nil {
		return errs.GetValue().(error)
	}

	// write a complete row.
	keyValueStore = &db.KeyValueStore{
		K: utils.KeyValueBootstrap,
		V: bootstrapValue,
	}
	return persist.InsertKeyValueStore(ctx, sess, keyValueStore)
}

type IndexerFactoryControl struct {
	sc     *servicesctrl.Control
	fsm    map[string]stream.ProcessorDB
	doneCh chan struct{}
}

func (c *IndexerFactoryControl) removeTxPool(conns *utils.Connections, txPool *db.TxPool) error {
	sess := conns.DB().NewSessionForEventReceiver(conns.Stream().NewJob("update-txpool-status"))
	ctx, cancelFn := context.WithTimeout(context.Background(), cfg.DefaultConsumeProcessWriteTimeout)
	defer cancelFn()
	return c.sc.Persist.RemoveTxPool(ctx, sess, txPool)
}

func (c *IndexerFactoryControl) handleTxPool(_ int, conns *utils.Connections) {
	defer func() {
		_ = conns.Close()
	}()

	for {
		select {
		case txd := <-c.sc.LocalTxPool:
			if c.sc.IndexedList.Exists(txd.TxPool.ID) {
				continue
			}
			if txd.Errs != nil && txd.Errs.GetValue() != nil {
				continue
			}
			if p, ok := c.fsm[txd.TxPool.Topic]; ok {
				err := p.Process(conns, txd.TxPool)
				if err != nil {
					if txd.Errs != nil {
						txd.Errs.SetValue(err)
					}
					continue
				}
				err = c.removeTxPool(conns, txd.TxPool)
				if err != nil {
					if txd.Errs != nil {
						txd.Errs.SetValue(err)
					}
					continue
				}
				c.sc.IndexedList.PushFront(txd.TxPool.ID, txd.TxPool.ID)
			}
		case <-c.doneCh:
			return
		}
	}
}

func IndexerFactories(
	sc *servicesctrl.Control,
	config *cfg.Config,
	factoriesChainDB []stream.ProcessorFactoryChainDB,
	factoriesInstDB []stream.ProcessorFactoryInstDB,
	wg *sync.WaitGroup,
	runningControl utils.Running,
) error {
	ctrl := &IndexerFactoryControl{
		sc:     sc,
		fsm:    make(map[string]stream.ProcessorDB),
		doneCh: make(chan struct{}),
	}

	var topicNames []string

	for _, factory := range factoriesChainDB {
		for _, chainConfig := range config.Chains {
			f, err := factory(sc, *config, chainConfig.VMType, chainConfig.ID)
			if err != nil {
				return err
			}
			for _, topic := range f.Topic() {
				if _, ok := ctrl.fsm[topic]; ok {
					return fmt.Errorf("duplicate topic %v", topic)
				}
				ctrl.fsm[topic] = f
				topicNames = append(topicNames, topic)
			}
		}
	}
	for _, factory := range factoriesInstDB {
		f, err := factory(sc, *config)
		if err != nil {
			return err
		}
		for _, topic := range f.Topic() {
			if _, ok := ctrl.fsm[topic]; ok {
				return fmt.Errorf("duplicate topic %v", topic)
			}
			ctrl.fsm[topic] = f
			topicNames = append(topicNames, topic)
		}
	}

	conns, err := sc.Database()
	if err != nil {
		return err
	}

	for ipos := 0; ipos < MaxTheads; ipos++ {
		conns1, err := sc.Database()
		if err != nil {
			_ = conns.Close()
			close(ctrl.doneCh)
			return err
		}
		go ctrl.handleTxPool(ipos, conns1)
	}

	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			close(ctrl.doneCh)
			_ = conns.Close()
		}()
		for !runningControl.IsStopped() {
			iterateTxPool := func() {
				ctx, cancelCTX := context.WithTimeout(context.Background(), IteratorTimeout)
				defer cancelCTX()

				name := "tx-pool"
				sess, err := conns.DB().NewSession(name, IteratorTimeout)
				if err != nil {
					sc.Log.Error("failed creating session",
						zap.String("name", name),
						zap.Error(err),
					)
					time.Sleep(250 * time.Millisecond)
					return
				}
				iterator, err := sess.Select(
					"id",
					"network_id",
					"chain_id",
					"msg_key",
					"serialization",
					"topic",
					"created_at",
				).From(db.TableTxPool).
					Where("topic in ?", topicNames).
					OrderAsc("created_at").
					IterateContext(ctx)
				if err != nil {
					sc.Log.Warn("failed creating iterator",
						zap.String("name", name),
						zap.Error(err),
					)
					return
				}

				errs := &avlancheGoUtils.AtomicInterface{}

				var readMessages uint64

				for iterator.Next() {
					if errs.GetValue() != nil {
						break
					}

					if readMessages > MaximumRecordsRead {
						break
					}

					err = iterator.Err()
					if err != nil {
						if err != io.EOF {
							sc.Log.Error("failed iterating",
								zap.String("name", name),
								zap.Error(err),
							)
						}
						break
					}

					txp := &db.TxPool{}
					err = iterator.Scan(txp)
					if err != nil {
						sc.Log.Error("failed scanning iterator",
							zap.String("name", name),
							zap.Error(err),
						)
						break
					}
					// skip previously processed
					if sc.IndexedList.Exists(txp.ID) {
						continue
					}
					readMessages++
					sc.LocalTxPool <- &servicesctrl.LocalTxPoolJob{TxPool: txp, Errs: errs}
				}

				for ipos := 0; ipos < (5*1000) && len(sc.LocalTxPool) > 0; ipos++ {
					time.Sleep(1 * time.Millisecond)
				}

				if errIntf := errs.GetValue(); errIntf != nil {
					err := errIntf.(error)
					sc.Log.Error("failed processing",
						zap.String("name", name),
						zap.Error(err),
					)
					time.Sleep(250 * time.Millisecond)
					return
				}

				if readMessages == 0 {
					time.Sleep(2 * time.Second)
				}
			}
			iterateTxPool()
		}
	}()

	return nil
}
