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

package utils

import (
	"errors"

	"github.com/lasthyphen/dijetsnodego/codec"
	"github.com/lasthyphen/dijetsnodego/genesis"
	"github.com/lasthyphen/dijetsnodego/utils/constants"
	"github.com/lasthyphen/dijetsnodego/vms/avm"
	"github.com/lasthyphen/dijetsnodego/vms/nftfx"
	"github.com/lasthyphen/dijetsnodego/vms/platformvm"
	"github.com/lasthyphen/dijetsnodego/vms/secp256k1fx"
)

var (
	ErrIncorrectGenesisChainTxType = errors.New("incorrect genesis chain tx type")
)

const MaxCodecSize = 100_000_000

func NewAVMCodec(networkID uint32) (codec.Manager, error) {
	genesisBytes, _, err := genesis.FromConfig(genesis.GetConfig(networkID))
	if err != nil {
		return nil, nil
	}

	g, err := genesis.VMGenesis(genesisBytes, constants.AVMID)
	if err != nil {
		return nil, err
	}

	createChainTx, ok := g.UnsignedTx.(*platformvm.UnsignedCreateChainTx)
	if !ok {
		return nil, ErrIncorrectGenesisChainTxType
	}

	var (
		fxIDs = createChainTx.FxIDs
		fxs   = make([]avm.Fx, 0, len(fxIDs))
	)

	for _, fxID := range fxIDs {
		switch {
		case fxID == secp256k1fx.ID:
			fxs = append(fxs, &secp256k1fx.Fx{})
		case fxID == nftfx.ID:
			fxs = append(fxs, &nftfx.Fx{})
		default:
			// return nil, fmt.Errorf("Unknown FxID: %s", fxID)
		}
	}

	_, codec, err := avm.NewCodecs(fxs)
	codec.SetMaxSize(MaxCodecSize)
	return codec, err
}
