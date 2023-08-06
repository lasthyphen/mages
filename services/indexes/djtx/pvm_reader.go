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

	"github.com/lasthyphen/dijetsnodego/ids"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services/indexes/params"
)

func (r *Reader) ListBlocks(ctx context.Context, params *params.ListBlocksParams) (*models.BlockList, error) {
	dbRunner, err := r.conns.DB().NewSession("list_blocks", cfg.RequestTimeout)
	if err != nil {
		return nil, err
	}

	blocks := []*models.Block{}

	_, err = params.Apply(dbRunner.
		Select("id", "type", "parent_id", "chain_id", "created_at").
		From("pvm_blocks")).
		LoadContext(ctx, &blocks)

	if err != nil {
		return nil, err
	}
	return &models.BlockList{Blocks: blocks}, nil
}

func (r *Reader) GetBlock(ctx context.Context, id ids.ID) (*models.Block, error) {
	list, err := r.ListBlocks(ctx, &params.ListBlocksParams{ListParams: params.ListParams{ID: &id}})
	if err != nil || len(list.Blocks) == 0 {
		return nil, err
	}
	return list.Blocks[0], nil
}
