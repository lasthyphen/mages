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
	"errors"
	"strings"
	"time"

	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/utils"
)

var (
	processorFailureRetryInterval = 200 * time.Millisecond

	// ErrNoMessage is no message
	ErrNoMessage = errors.New("no message")
)

type ProcessorFactoryChainDB func(*servicesctrl.Control, cfg.Config, string, string) (ProcessorDB, error)
type ProcessorFactoryInstDB func(*servicesctrl.Control, cfg.Config) (ProcessorDB, error)

type ProcessorDB interface {
	Process(*utils.Connections, *db.TxPool) error
	Close() error
	ID() string
	Topic() []string
}

func UpdateTxPool(
	ctxTimeout time.Duration,
	conns *utils.Connections,
	persist db.Persist,
	txPool *db.TxPool,
	sc *servicesctrl.Control,
) error {
	sess := conns.DB().NewSessionForEventReceiver(conns.Stream().NewJob("update-tx-pool"))

	ctx, cancelCtx := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancelCtx()

	err := persist.InsertTxPool(ctx, sess, txPool)
	if err == nil {
		sc.Enqueue(txPool)
	}
	return err
}

func TrimNL(msg string) string {
	oldmsg := msg
	for {
		msg = strings.TrimPrefix(msg, "\n")
		if msg == oldmsg {
			break
		}
		oldmsg = msg
	}
	oldmsg = msg
	for {
		msg = strings.TrimSuffix(msg, "\n")
		if msg == oldmsg {
			break
		}
		oldmsg = msg
	}
	return msg
}
