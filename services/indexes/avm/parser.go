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

package avm

import (
	"github.com/lasthyphen/dijetsnodego/codec"
	"github.com/lasthyphen/dijetsnodego/vms/avm"
)

func parseTx(c codec.Manager, bytes []byte) (*avm.Tx, error) {
	tx := &avm.Tx{}
	ver, err := c.Unmarshal(bytes, tx)
	if err != nil {
		return nil, err
	}
	unsignedBytes, err := c.Marshal(ver, &tx.UnsignedTx)
	if err != nil {
		return nil, err
	}

	tx.Initialize(unsignedBytes, bytes)
	return tx, nil
}
