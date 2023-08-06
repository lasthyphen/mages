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

package caching

import (
	"context"
	"time"

	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/utils"
)

const (
	WorkerQueueSize   = 10
	WorkerThreadCount = 1
)

type CacheJob struct {
	Key  string
	Body *[]byte
	TTL  time.Duration
}

type DelayCache struct {
	Cache  Cacher
	Worker utils.Worker
}

func NewDelayCache(cache Cacher) *DelayCache {
	c := &DelayCache{Cache: cache}
	c.Worker = utils.NewWorker(WorkerQueueSize, WorkerThreadCount, c.Processor)
	return c
}

func (c *DelayCache) Processor(_ int, job interface{}) {
	if j, ok := job.(*CacheJob); ok {
		ctxset, cancelFnSet := context.WithTimeout(context.Background(), cfg.CacheTimeout)
		defer cancelFnSet()

		// if cache did not set, we can just ignore.
		_ = c.Cache.Set(ctxset, j.Key, *j.Body, j.TTL)
	}
}
