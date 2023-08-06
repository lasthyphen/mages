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

package pvm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/lasthyphen/dijetsnodego/vms/platformvm/validator"

	"github.com/lasthyphen/dijetsnodego/utils/constants"

	p_genesis "github.com/lasthyphen/dijetsnodego/vms/platformvm/genesis"

	"github.com/lasthyphen/dijetsnodego/vms/platformvm/txs"

	"github.com/lasthyphen/dijetsnodego/vms/platformvm/blocks"

	"github.com/lasthyphen/dijetsnodego/api/metrics"
	"github.com/lasthyphen/dijetsnodego/codec"
	"github.com/lasthyphen/dijetsnodego/genesis"
	"github.com/lasthyphen/dijetsnodego/ids"
	"github.com/lasthyphen/dijetsnodego/snow"
	"github.com/lasthyphen/dijetsnodego/utils/logging"
	"github.com/lasthyphen/dijetsnodego/utils/wrappers"
	"github.com/lasthyphen/dijetsnodego/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodego/vms/components/verify"
	"github.com/lasthyphen/dijetsnodego/vms/proposervm/block"
	"github.com/lasthyphen/dijetsnodego/vms/secp256k1fx"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/db"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services"
	djtxIndexer "github.com/lasthyphen/mages/services/indexes/djtx"
	"github.com/lasthyphen/mages/utils"
)

var (
	MaxSerializationLen = (16 * 1024 * 1024) - 1

	ChainID = ids.ID{}

	ErrUnknownBlockType = errors.New("unknown block type")
)

type Writer struct {
	chainID     string
	networkID   uint32
	djtxAssetID ids.ID

	djtx *djtxIndexer.Writer
	ctx  *snow.Context
}

func NewWriter(networkID uint32, chainID string) (*Writer, error) {
	_, djtxAssetID, err := genesis.FromConfig(genesis.GetConfig(networkID))
	if err != nil {
		return nil, err
	}

	bcLookup := ids.NewAliaser()
	id, err := ids.FromString(chainID)
	if err != nil {
		return nil, err
	}
	if err = bcLookup.Alias(id, "P"); err != nil {
		return nil, err
	}

	ctx := &snow.Context{
		NetworkID: networkID,
		ChainID:   id,
		Log:       logging.NoLog{},
		Metrics:   metrics.NewOptionalGatherer(),
		BCLookup:  bcLookup,
	}

	return &Writer{
		chainID:     chainID,
		networkID:   networkID,
		djtxAssetID: djtxAssetID,
		djtx:        djtxIndexer.NewWriter(chainID, djtxAssetID),
		ctx:         ctx,
	}, nil
}

func (*Writer) Name() string { return "pvm-index" }

type PtxDataModel struct {
	Tx        *txs.Tx               `json:"tx,omitempty"`
	TxType    *string               `json:"txType,omitempty"`
	Block     *blocks.Block         `json:"block,omitempty"`
	BlockID   *string               `json:"blockID,omitempty"`
	BlockType *string               `json:"blockType,omitempty"`
	Proposer  *models.BlockProposal `json:"proposer,omitempty"`
}

func (w *Writer) ParseJSON(b []byte, proposer *models.BlockProposal) ([]byte, error) {
	// Try and parse as a tx
	tx, err := txs.Parse(blocks.GenesisCodec, b)
	if err == nil {
		tx.Unsigned.InitCtx(w.ctx)
		// TODO: Should we be reporting the type of [tx.Unsigned] rather than
		//       `tx`?
		txtype := reflect.TypeOf(tx)
		txtypeS := txtype.String()
		return json.Marshal(&PtxDataModel{
			Tx:     tx,
			TxType: &txtypeS,
		})
	}

	// Try and parse as block
	blk, err := blocks.Parse(blocks.GenesisCodec, b)
	if err == nil {
		blk.InitCtx(w.ctx)
		blkID := blk.ID()
		blkIDStr := blkID.String()
		btype := reflect.TypeOf(blk)
		btypeS := btype.String()
		return json.Marshal(&PtxDataModel{
			BlockID:   &blkIDStr,
			Block:     &blk,
			BlockType: &btypeS,
		})
	}

	// Try and parse as proposervm block
	proposerBlock, err := block.Parse(b)
	if err != nil {
		return nil, err
	}

	blk, err = blocks.Parse(blocks.GenesisCodec, proposerBlock.Block())
	if err != nil {
		return nil, err
	}

	blk.InitCtx(w.ctx)
	blkID := blk.ID()
	blkIDStr := blkID.String()
	btype := reflect.TypeOf(blk)
	btypeS := btype.String()
	return json.Marshal(&PtxDataModel{
		BlockID:   &blkIDStr,
		Block:     &blk,
		BlockType: &btypeS,
		Proposer:  models.NewBlockProposal(proposerBlock, nil),
	})
}

func (w *Writer) ConsumeConsensus(_ context.Context, _ *utils.Connections, _ services.Consumable, _ db.Persist) error {
	return nil
}

func (w *Writer) Consume(ctx context.Context, conns *utils.Connections, c services.Consumable, persist db.Persist) error {
	job := conns.Stream().NewJob("pvm-index")
	sess := conns.DB().NewSessionForEventReceiver(job)

	dbTx, err := sess.Begin()
	if err != nil {
		return err
	}
	defer dbTx.RollbackUnlessCommitted()

	// Consume the tx and commit
	err = w.indexBlock(services.NewConsumerContext(ctx, dbTx, c.Timestamp(), c.Nanosecond(), persist, c.ChainID()), c.Body())
	if err != nil {
		return err
	}
	return dbTx.Commit()
}

func (w *Writer) Bootstrap(ctx context.Context, conns *utils.Connections, persist db.Persist) error {
	genesisBytes, _, err := genesis.FromConfig(genesis.GetConfig(w.networkID))
	if err != nil {
		return err
	}

	platformGenesis, err := p_genesis.Parse(genesisBytes)
	if err != nil {
		return err
	}

	var (
		job  = conns.Stream().NewJob("bootstrap")
		db   = conns.DB().NewSessionForEventReceiver(job)
		errs = wrappers.Errs{}
		cCtx = services.NewConsumerContext(ctx, db, int64(platformGenesis.Timestamp), 0, persist, w.chainID)
	)

	for idx, utxo := range platformGenesis.UTXOs {
		select {
		case <-ctx.Done():
		default:
		}

		_, _, err = w.djtx.ProcessStateOut(
			cCtx,
			utxo.Out,
			ChainID,
			uint32(idx),
			utxo.AssetID(),
			0,
			0,
			w.chainID,
			false,
			true,
		)
		if err != nil {
			return err
		}
	}

	for _, tx := range append(platformGenesis.Validators, platformGenesis.Chains...) {
		select {
		case <-ctx.Done():
		default:
		}

		errs.Add(w.indexTransaction(cCtx, ChainID, *tx, true))
	}

	return errs.Err
}

func initializeTx(version uint16, c codec.Manager, tx txs.Tx) error {
	unsignedBytes, err := c.Marshal(version, &tx.Unsigned)
	if err != nil {
		return err
	}
	signedBytes, err := c.Marshal(version, &tx)
	if err != nil {
		return err
	}
	tx.Initialize(unsignedBytes, signedBytes)
	return nil
}

func (w *Writer) indexBlock(ctx services.ConsumerCtx, proposerblockBytes []byte) error {
	proposerBlock, err := block.Parse(proposerblockBytes)
	var innerBlockBytes []byte
	if err != nil {
		innerBlockBytes = proposerblockBytes
		// We use the "nil"ness below, so we explicitly empty the value here to
		// avoid unexpected errors
		proposerBlock = nil
	} else {
		innerBlockBytes = proposerBlock.Block()
	}

	blk, err := blocks.Parse(blocks.GenesisCodec, innerBlockBytes)
	if err != nil {
		return err
	}

	blkID := blk.ID()
	ctxTime := ctx.Time()
	pvmProposer := models.NewBlockProposal(proposerBlock, &ctxTime)

	errs := wrappers.Errs{}

	switch blk := blk.(type) {
	case *blocks.ApricotProposalBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeProposal, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.ApricotStandardBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeStandard, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.ApricotAtomicBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeProposal, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.ApricotAbortBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeAbort, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.ApricotCommitBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeCommit, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.BanffProposalBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeProposal, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.BanffStandardBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeStandard, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.BanffAbortBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeProposal, blk.CommonBlock, pvmProposer, innerBlockBytes))
	case *blocks.BanffCommitBlock:
		errs.Add(w.indexCommonBlock(ctx, blkID, models.BlockTypeCommit, blk.CommonBlock, pvmProposer, innerBlockBytes))

	default:
		return fmt.Errorf("unknown type %s", reflect.TypeOf(blk))
	}
	for _, tx := range blk.Txs() {
		errs.Add(w.indexTransaction(ctx, blkID, *tx, false))
	}

	return errs.Err
}

func (w *Writer) indexCommonBlock(
	ctx services.ConsumerCtx,
	blkID ids.ID,
	blkType models.BlockType,
	blk blocks.CommonBlock,
	proposer *models.BlockProposal,
	blockBytes []byte,
) error {
	if len(blockBytes) > MaxSerializationLen {
		blockBytes = []byte("")
	}

	pvmBlocks := &db.PvmBlocks{
		ID:            blkID.String(),
		ChainID:       w.chainID,
		Type:          blkType,
		ParentID:      blk.Parent().String(),
		Serialization: blockBytes,
		CreatedAt:     ctx.Time(),
		Height:        blk.Height(),
		Proposer:      proposer.Proposer,
		ProposerTime:  proposer.TimeStamp,
	}
	return ctx.Persist().InsertPvmBlocks(ctx.Ctx(), ctx.DB(), pvmBlocks, cfg.PerformUpdates)
}

func (w *Writer) indexTransaction(ctx services.ConsumerCtx, blkID ids.ID, tx txs.Tx, genesis bool) error {
	var (
		txID   = tx.ID()
		baseTx djtx.BaseTx
		typ    models.TransactionType
		ins    *djtxIndexer.AddInsContainer
		outs   *djtxIndexer.AddOutsContainer
	)

	var err error
	switch castTx := tx.Unsigned.(type) {
	case *txs.AddValidatorTx:
		baseTx = castTx.BaseTx.BaseTx
		outs = &djtxIndexer.AddOutsContainer{
			Outs:    castTx.StakeOuts,
			Stake:   true,
			ChainID: w.chainID,
		}
		typ = models.TransactionTypeAddValidator
		err = w.InsertTransactionValidator(ctx, txID, castTx.Validator)
		if err != nil {
			return err
		}
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
		if castTx.RewardsOwner != nil {
			err = w.insertTransactionsRewardsOwners(ctx, txID, castTx.RewardsOwner, baseTx, castTx.StakeOuts)
			if err != nil {
				return err
			}
		}
	case *txs.AddSubnetValidatorTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeAddSubnetValidator
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
	case *txs.AddDelegatorTx:
		baseTx = castTx.BaseTx.BaseTx
		outs = &djtxIndexer.AddOutsContainer{
			Outs:    castTx.StakeOuts,
			Stake:   true,
			ChainID: w.chainID,
		}
		typ = models.TransactionTypeAddDelegator
		err = w.InsertTransactionValidator(ctx, txID, castTx.Validator)
		if err != nil {
			return err
		}
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
		err = w.insertTransactionsRewardsOwners(ctx, txID, castTx.DelegationRewardsOwner, baseTx, castTx.StakeOuts)
		if err != nil {
			return err
		}
	case *txs.CreateSubnetTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeCreateSubnet
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
	case *txs.CreateChainTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeCreateChain
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
	case *txs.ImportTx:
		baseTx = castTx.BaseTx.BaseTx
		ins = &djtxIndexer.AddInsContainer{
			Ins:     castTx.ImportedInputs,
			ChainID: castTx.SourceChain.String(),
		}
		typ = models.TransactionTypePVMImport
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
	case *txs.ExportTx:
		baseTx = castTx.BaseTx.BaseTx
		outs = &djtxIndexer.AddOutsContainer{
			Outs:    castTx.ExportedOutputs,
			ChainID: castTx.DestinationChain.String(),
		}
		typ = models.TransactionTypePVMExport
		err = w.InsertTransactionBlock(ctx, txID, blkID)
		if err != nil {
			return err
		}
	case *txs.AdvanceTimeTx:
		return nil
	case *txs.RewardValidatorTx:
		rewards := &db.Rewards{
			ID:                 txID.String(),
			BlockID:            blkID.String(),
			Txid:               castTx.TxID.String(),
			Shouldprefercommit: castTx.ShouldPreferCommit,
			CreatedAt:          ctx.Time(),
		}
		return ctx.Persist().InsertRewards(ctx.Ctx(), ctx.DB(), rewards, cfg.PerformUpdates)
	case *txs.RemoveSubnetValidatorTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeRemoveSubnetValidator
	case *txs.TransformSubnetTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeTransformSubnet
	case *txs.AddPermissionlessValidatorTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeAddPermissionlessValidator

		// TODO: Handle this for all subnetIDs
		if castTx.Subnet != constants.PrimaryNetworkID {
			break
		}

		outs = &djtxIndexer.AddOutsContainer{
			Outs:    castTx.StakeOuts,
			Stake:   true,
			ChainID: w.chainID,
		}
		err := w.InsertTransactionValidator(ctx, txID, castTx.Validator)
		if err != nil {
			return err
		}
		// TODO: What to do about the different rewards owners?
		err = w.insertTransactionsRewardsOwners(ctx, txID, castTx.ValidatorRewardsOwner, baseTx, castTx.StakeOuts)
		if err != nil {
			return err
		}
	case *txs.AddPermissionlessDelegatorTx:
		baseTx = castTx.BaseTx.BaseTx
		typ = models.TransactionTypeAddPermissionlessDelegator

		// TODO: Handle this for all subnetIDs
		if castTx.Subnet != constants.PrimaryNetworkID {
			break
		}

		outs = &djtxIndexer.AddOutsContainer{
			Outs:    castTx.StakeOuts,
			Stake:   true,
			ChainID: w.chainID,
		}
		err := w.InsertTransactionValidator(ctx, txID, castTx.Validator)
		if err != nil {
			return err
		}
		err = w.insertTransactionsRewardsOwners(ctx, txID, castTx.DelegationRewardsOwner, baseTx, castTx.StakeOuts)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown tx type %s", reflect.TypeOf(castTx))
	}

	return w.djtx.InsertTransaction(
		ctx,
		tx.Bytes(),
		tx.ID(),
		tx.Unsigned.Bytes(),
		&baseTx,
		tx.Creds,
		typ,
		ins,
		outs,
		0,
		genesis,
	)
}

func (w *Writer) insertTransactionsRewardsOwners(ctx services.ConsumerCtx, txID ids.ID, rewardsOwner verify.Verifiable, baseTx djtx.BaseTx, stakeOuts []*djtx.TransferableOutput) error {
	var err error

	owner, ok := rewardsOwner.(*secp256k1fx.OutputOwners)
	if !ok {
		return fmt.Errorf("rewards owner %T", rewardsOwner)
	}

	// Ingest each Output Address
	for ipos, addr := range owner.Addresses() {
		addrid := ids.ShortID{}
		copy(addrid[:], addr)
		txRewardsOwnerAddress := &db.TransactionsRewardsOwnersAddress{
			ID:          txID.String(),
			Address:     addrid.String(),
			OutputIndex: uint32(ipos),
			UpdatedAt:   time.Now().UTC(),
		}

		err = ctx.Persist().InsertTransactionsRewardsOwnersAddress(ctx.Ctx(), ctx.DB(), txRewardsOwnerAddress, cfg.PerformUpdates)
		if err != nil {
			return err
		}
	}

	// write out outputs in the len(outs) and len(outs)+1 positions to identify these rewards
	outcnt := len(baseTx.Outs) + len(stakeOuts)
	for ipos := outcnt; ipos < outcnt+2; ipos++ {
		outputID := txID.Prefix(uint64(ipos))

		txRewardsOutputs := &db.TransactionsRewardsOwnersOutputs{
			ID:            outputID.String(),
			TransactionID: txID.String(),
			OutputIndex:   uint32(ipos),
			CreatedAt:     ctx.Time(),
		}

		err = ctx.Persist().InsertTransactionsRewardsOwnersOutputs(ctx.Ctx(), ctx.DB(), txRewardsOutputs, cfg.PerformUpdates)
		if err != nil {
			return err
		}
	}

	txRewardsOwner := &db.TransactionsRewardsOwners{
		ID:        txID.String(),
		ChainID:   w.chainID,
		Threshold: owner.Threshold,
		Locktime:  owner.Locktime,
		CreatedAt: ctx.Time(),
	}

	return ctx.Persist().InsertTransactionsRewardsOwners(ctx.Ctx(), ctx.DB(), txRewardsOwner, cfg.PerformUpdates)
}

func (w *Writer) InsertTransactionValidator(ctx services.ConsumerCtx, txID ids.ID, validator validator.Validator) error {
	transactionsValidator := &db.TransactionsValidator{
		ID:        txID.String(),
		NodeID:    validator.NodeID.String(),
		Start:     validator.Start,
		End:       validator.End,
		CreatedAt: ctx.Time(),
	}
	return ctx.Persist().InsertTransactionsValidator(ctx.Ctx(), ctx.DB(), transactionsValidator, cfg.PerformUpdates)
}

func (w *Writer) InsertTransactionBlock(ctx services.ConsumerCtx, txID ids.ID, blkTxID ids.ID) error {
	transactionsBlock := &db.TransactionsBlock{
		ID:        txID.String(),
		TxBlockID: blkTxID.String(),
		CreatedAt: ctx.Time(),
	}
	return ctx.Persist().InsertTransactionsBlock(ctx.Ctx(), ctx.DB(), transactionsBlock, cfg.PerformUpdates)
}
