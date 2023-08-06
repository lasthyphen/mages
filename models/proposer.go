// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
// See the file LICENSE for licensing terms.

package models

import (
	"time"

	"github.com/lasthyphen/dijetsnodego/vms/proposervm/block"
)

// Proposal information P/C
type BlockProposal struct {
	Proposer  string     `json:"proposer,omitempty"`
	TimeStamp *time.Time `json:"timeStamp,omitempty"`
}

func NewBlockProposal(b block.Block, t *time.Time) *BlockProposal {
	switch properBlockDetail := b.(type) {
	case block.SignedBlock:
		t := properBlockDetail.Timestamp()
		return &BlockProposal{
			Proposer:  properBlockDetail.Proposer().String(),
			TimeStamp: &t,
		}
	default:
		return &BlockProposal{TimeStamp: t}
	}
}
