// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file is a derived work, based on ava-labs code
//
// It is distributed under the same license conditions as the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********************************************************

package services

import (
	"context"
	"time"

	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/modelsc"
	"github.com/lasthyphen/mages/utils"
	"github.com/gocraft/dbr/v2"
)

type Consumable interface {
	ID() string
	ChainID() string
	Body() []byte
	Timestamp() int64
	Nanosecond() int64
}

// Consumer takes in Consumables and adds them to the service's backend
type Consumer interface {
	Name() string
	Bootstrap(context.Context, *utils.Connections, db.Persist) error
	Consume(context.Context, *utils.Connections, Consumable, db.Persist) error
	ConsumeConsensus(context.Context, *utils.Connections, Consumable, db.Persist) error
	ParseJSON([]byte) ([]byte, error)
}

type ConsumerCChain interface {
	Name() string
	Consume(context.Context, *utils.Connections, Consumable, *modelsc.Block, db.Persist) error
	ParseJSON([]byte) ([]byte, error)
}

type ConsumerCtx struct {
	ctx     context.Context
	db      dbr.SessionRunner
	time    time.Time
	persist db.Persist
	chainID string
}

func NewConsumerContext(ctx context.Context, db dbr.SessionRunner, ts int64, nanosecond int64, persist db.Persist, chainID string) ConsumerCtx {
	return ConsumerCtx{
		ctx:     ctx,
		db:      db,
		time:    time.Unix(ts, nanosecond),
		persist: persist,
		chainID: chainID,
	}
}

func (ic *ConsumerCtx) Time() time.Time       { return ic.time }
func (ic *ConsumerCtx) DB() dbr.SessionRunner { return ic.db }
func (ic *ConsumerCtx) Ctx() context.Context  { return ic.ctx }
func (ic *ConsumerCtx) Persist() db.Persist   { return ic.persist }
func (ic *ConsumerCtx) ChainID() string       { return ic.chainID }
