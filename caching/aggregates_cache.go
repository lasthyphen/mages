package caching

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services/indexes/params"
	"github.com/lasthyphen/mages/utils"
	"github.com/gocraft/dbr/v2"
)

const (
	MaxAggregateIntervalCount = 20000
)

var (
	ErrAggregateIntervalCountTooLarge = errors.New("requesting too many intervals")
	ErrFailedToParseStringAsBigInt    = errors.New("failed to parse string to big.Int")
)

type AggregatesCache interface {
	GetAggregateTransactionsMap() map[string]map[string]uint64
	GetAggregateFeesMap() map[string]map[string]uint64
	InitCacheStorage(cfg.Chains)
	GetAggregatesFeesAndUpdate(map[string]cfg.Chain, *utils.Connections, string, time.Time, time.Time, string) error
	GetAggregatesAndUpdate(map[string]cfg.Chain, *utils.Connections, string, time.Time, time.Time, string) error
}

type aggregatesCache struct {
	aggregateTransactionsMap map[string]map[string]uint64
	aggregateFeesMap         map[string]map[string]uint64
}

func NewAggregatesCache() AggregatesCache {
	return &aggregatesCache{
		aggregateTransactionsMap: make(map[string]map[string]uint64),
		aggregateFeesMap:         make(map[string]map[string]uint64),
	}
}

func (ac *aggregatesCache) GetAggregateTransactionsMap() map[string]map[string]uint64 {
	return ac.aggregateTransactionsMap
}

func (ac *aggregatesCache) GetAggregateFeesMap() map[string]map[string]uint64 {
	return ac.aggregateFeesMap
}

func (ac *aggregatesCache) InitCacheStorage(chains cfg.Chains) {
	aggregateTransMap := ac.aggregateTransactionsMap
	aggregateFeesMap := ac.aggregateFeesMap

	for id := range chains {
		aggregateTransMap[id] = map[string]uint64{}
		aggregateFeesMap[id] = map[string]uint64{}

		// we initialize the value for all 3 chains
		aggregateTransMap[id]["day"] = 0
		aggregateTransMap[id]["week"] = 0
		aggregateTransMap[id]["month"] = 0

		// we initialize the value for all 3 chains also here
		aggregateFeesMap[id]["day"] = 0
		aggregateFeesMap[id]["week"] = 0
		aggregateFeesMap[id]["month"] = 0
	}
}

//gocyclo:ignore
func (ac *aggregatesCache) GetAggregatesAndUpdate(chains map[string]cfg.Chain, conns *utils.Connections, chainid string, startTime time.Time, endTime time.Time, rangeKeyType string) error {
	chainIds := []string{chainid}
	// Validate params and set defaults if necessary
	if startTime.IsZero() {
		var err error
		startTime, err = getFirstTransactionTime(conns, chainIds)
		if err != nil {
			return err
		}
	}

	intervals := models.AggregatesList{}
	urlv := url.Values{}
	assetID, err := params.GetQueryID(urlv, params.KeyAssetID)
	if err != nil {
		return err
	}
	intervalSize, err := params.GetQueryInterval(urlv, params.KeyIntervalSize)
	if err != nil {
		return err
	}

	// Ensure the interval count requested isn't too large
	intervalSeconds := int64(intervalSize.Seconds())
	requestedIntervalCount := 0
	if intervalSeconds != 0 {
		requestedIntervalCount = int(math.Ceil(endTime.Sub(startTime).Seconds() / intervalSize.Seconds()))
		if requestedIntervalCount > MaxAggregateIntervalCount {
			return ErrAggregateIntervalCountTooLarge
		}
		if requestedIntervalCount < 1 {
			requestedIntervalCount = 1
		}
	}

	// Split chains
	var avmChains, cvmChains []string
	if len(chainIds) == 0 {
		for id, chain := range chains {
			switch chain.VMType {
			case models.CVMName:
				cvmChains = append(cvmChains, id)
			default:
				avmChains = append(avmChains, id)
			}
		}
	} else {
		for _, id := range chainIds {
			chain, exist := chains[id]
			if exist {
				switch chain.VMType {
				case models.CVMName:
					cvmChains = append(cvmChains, id)
				default:
					avmChains = append(avmChains, id)
				}
			}
		}
	}

	var dbRunner *dbr.Session

	if conns != nil {
		dbRunner = conns.DB().NewSessionForEventReceiver(conns.Stream().NewJob("get_transaction_aggregates_histogram"))
	} else {
		dbRunner, err = conns.DB().NewSession("get_transaction_aggregates_histogram", cfg.DBTimeout)
		if err != nil {
			return err
		}
	}

	var builder *dbr.SelectStmt

	if len(avmChains) > 0 {
		columns := []string{
			"COALESCE(SUM(avm_outputs.amount), 0) AS transaction_volume",
			"COUNT(DISTINCT(avm_outputs.transaction_id)) AS transaction_count",
			"COUNT(DISTINCT(avm_output_addresses.address)) AS address_count",
			"COUNT(DISTINCT(avm_outputs.asset_id)) AS asset_count",
			"COUNT(avm_outputs.id) AS output_count",
		}

		if requestedIntervalCount > 0 {
			columns = append(columns, fmt.Sprintf(
				"FLOOR((UNIX_TIMESTAMP(avm_outputs.created_at)-%d) / %d) AS interval_id",
				startTime.Unix(),
				intervalSeconds))
		}

		builder = dbRunner.
			Select(columns...).
			From("avm_outputs").
			LeftJoin("avm_output_addresses", "avm_output_addresses.output_id = avm_outputs.id").
			Where("avm_outputs.created_at >= ?", startTime).
			Where("avm_outputs.created_at < ?", endTime)

		if len(chainIds) != 0 {
			builder.Where("avm_outputs.chain_id IN ?", avmChains)
		}

		if assetID != nil {
			builder.Where("avm_outputs.asset_id = ?", assetID.String())
		}

		if requestedIntervalCount > 0 {
			builder.
				GroupBy("interval_id").
				OrderAsc("interval_id").
				Limit(uint64(requestedIntervalCount))
		}

		_, err = builder.Load(&intervals)
		if err != nil {
			return err
		}
	}

	if len(cvmChains) > 0 {
		// 1st step: we obtain the first block from the inputted date range
		builder = dbRunner.
			Select("block").
			From("cvm_blocks").
			Where("cvm_blocks.created_at >= ?", startTime).
			Where("cvm_blocks.created_at < ?", endTime).
			OrderBy("block asc").
			Limit(1)

		firstBlockValue := models.BlockValue{}

		_, err = builder.Load(&firstBlockValue)
		if err != nil {
			return err
		}

		// 2nd step: we obtain the last block
		// we get the last block from the cache(cam_last_block_cache db table)
		builder = dbRunner.
			Select("current_block as block").
			From("cam_last_block_cache").
			Where("chainid=?", chainIds[0])

		lastBlockValue := models.BlockValue{}
		_, err = builder.Load(&lastBlockValue)
		if err != nil {
			return err
		}

		if lastBlockValue.Block <= firstBlockValue.Block {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// handle edge case in case we do not have a transaction block during this range
		if firstBlockValue.Block <= 0 {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// handle edge case in case we do not have a transaction block during this range
		if firstBlockValue.Block <= 0 {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// 3rd step: we obtain the count of the transactions based on the block range we acquired from the previous steps
		// we will construct based on the block numbers the relevant block_idx filters since this is our main index in the cvm_transactions_txdata table
		builder = dbRunner.
			Select("count(*) as  transaction_count").
			From("cvm_transactions_txdata").
			Where("cvm_transactions_txdata.block_idx >= concat(?,'000')", firstBlockValue.Block).
			Where("cvm_transactions_txdata.block_idx <= concat(?,'999')", lastBlockValue.Block)

		cvmIntervals := models.AggregatesList{}

		_, err = builder.Load(&cvmIntervals)
		if err != nil {
			return err
		}

		intervals = append(intervals, cvmIntervals[0])
	}

	// This check should never fail if the SQL query is correct, but added for
	// robustness to prevent panics if the invariant does not hold.
	if len(intervals) > 0 {
		intervals[0].StartTime = startTime
		intervals[0].EndTime = endTime
		aggs := &models.AggregatesHistogram{
			Aggregates: intervals[0],
			StartTime:  startTime,
			EndTime:    endTime,
		}
		ac.aggregateTransactionsMap[chainid][rangeKeyType] = aggs.Aggregates.TransactionCount
		return nil
	}
	aggs := &models.AggregatesHistogram{
		StartTime: startTime,
		EndTime:   endTime,
	}
	ac.aggregateTransactionsMap[chainid][rangeKeyType] = aggs.Aggregates.TransactionCount
	return nil
}

//gocyclo:ignore
func (ac *aggregatesCache) GetAggregatesFeesAndUpdate(chains map[string]cfg.Chain, conns *utils.Connections, chainid string, startTime time.Time, endTime time.Time, rangeKeyType string) error {
	chainIds := []string{chainid}
	// Validate params and set defaults if necessary
	if startTime.IsZero() {
		var err error
		startTime, err = getFirstTransactionTime(conns, chainIds)
		if err != nil {
			return err
		}
	}

	intervals := models.TxfeeAggregatesList{}
	urlv := url.Values{}
	intervalSize, err := params.GetQueryInterval(urlv, params.KeyIntervalSize)
	if err != nil {
		return err
	}

	// Ensure the interval count requested isn't too large
	intervalSeconds := int64(intervalSize.Seconds())
	requestedIntervalCount := 0
	if intervalSeconds != 0 {
		requestedIntervalCount = int(math.Ceil(endTime.Sub(startTime).Seconds() / intervalSize.Seconds()))
		if requestedIntervalCount > MaxAggregateIntervalCount {
			return ErrAggregateIntervalCountTooLarge
		}
		if requestedIntervalCount < 1 {
			requestedIntervalCount = 1
		}
	}

	// Split chains
	var avmChains, cvmChains []string
	if len(chainIds) == 0 {
		for id, chain := range chains {
			switch chain.VMType {
			case models.CVMName:
				cvmChains = append(cvmChains, id)
			default:
				avmChains = append(avmChains, id)
			}
		}
	} else {
		for _, id := range chainIds {
			chain, exist := chains[id]
			if exist {
				switch chain.VMType {
				case models.CVMName:
					cvmChains = append(cvmChains, id)
				default:
					avmChains = append(avmChains, id)
				}
			}
		}
	}

	// Build the query and load the base data
	dbRunner, err := conns.DB().NewSession("get_txfee_aggregates_histogram", cfg.DBTimeout)
	if err != nil {
		return err
	}

	var builder *dbr.SelectStmt

	if len(avmChains) > 0 {
		columns := []string{
			"CAST(COALESCE(SUM(avm_transactions.txfee), 0) AS UNSIGNED) AS txfee",
		}

		if requestedIntervalCount > 0 {
			columns = append(columns, fmt.Sprintf(
				"FLOOR((UNIX_TIMESTAMP(avm_transactions.created_at)-%d) / %d) AS interval_id",
				startTime.Unix(),
				intervalSeconds))
		}

		builder = dbRunner.
			Select(columns...).
			From("avm_transactions").
			Where("avm_transactions.created_at >= ?", startTime).
			Where("avm_transactions.created_at < ?", endTime)

		if requestedIntervalCount > 0 {
			builder.
				GroupBy("interval_id").
				OrderAsc("interval_id").
				Limit(uint64(requestedIntervalCount))
		}

		if len(chainIds) != 0 {
			builder.Where("avm_transactions.chain_id IN ?", chainIds)
		}

		_, err = builder.Load(&intervals)
		if err != nil {
			return err
		}
	}

	if len(cvmChains) > 0 {
		// 1st step: we obtain the first block from the inputted date range
		builder = dbRunner.
			Select("block").
			From("cvm_blocks").
			Where("cvm_blocks.created_at >= ?", startTime).
			Where("cvm_blocks.created_at < ?", endTime).
			OrderBy("block asc").
			Limit(1)

		firstBlockValue := models.BlockValue{}

		_, err = builder.Load(&firstBlockValue)
		if err != nil {
			return err
		}

		// 2nd step: we obtain the last block from the inputted date range(initially creating a query but will be substituted with the last blockid from the Node)
		// we get the last block from the cache(cam_last_block_cache db table)
		builder = dbRunner.
			Select("current_block as block").
			From("cam_last_block_cache").
			Where("chainid=?", chainIds[0])

		lastBlockValue := models.BlockValue{}
		_, err = builder.Load(&lastBlockValue)
		if err != nil {
			return err
		}

		if lastBlockValue.Block <= firstBlockValue.Block {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// handle edge case in case we do not have a transaction block during this range
		if firstBlockValue.Block <= 0 {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// handle edge case in case we do not have a transaction block during this range
		if firstBlockValue.Block <= 0 {
			lastBlockValue.Block = firstBlockValue.Block
		}

		// 3rd step: we obtain the count of the transactions based on the block range we acquired from the previous steps
		// we will construct based on the block numbers the relevant block_idx filters since this is our main index in the cvm_transactions_txdata table
		builder = dbRunner.
			Select("cast((cvm_transactions_txdata.gas_price / 1000000000) * cvm_transactions_txdata.gas_used AS UNSIGNED) as txfee").
			From("cvm_transactions_txdata").
			Where("cvm_transactions_txdata.block_idx >= concat(?,'000')", firstBlockValue.Block).
			Where("cvm_transactions_txdata.block_idx <= concat(?,'999')", lastBlockValue.Block)

		cvmIntervals := models.TxfeeAggregatesList{}

		_, err = builder.Load(&cvmIntervals)
		if err != nil {
			return err
		}

		// we calculate the sum of the fees here because of the db cost
		var totalVolume uint64
		for _, interval := range cvmIntervals {
			// Add to the overall aggregates counts
			totalVolume += interval.Txfee
		}

		if len(cvmIntervals) > 0 {
			intervals = append(intervals, cvmIntervals[0])
			intervals[0].Txfee = totalVolume
		}
	}

	// This check should never fail if the SQL query is correct, but added for
	// robustness to prevent panics if the invariant does not hold.
	if len(intervals) > 0 {
		intervals[0].StartTime = startTime
		intervals[0].EndTime = endTime
		aggs := &models.TxfeeAggregatesHistogram{
			TxfeeAggregates: intervals[0],
			StartTime:       startTime,
			EndTime:         endTime,
		}
		ac.aggregateFeesMap[chainid][rangeKeyType] = aggs.TxfeeAggregates.Txfee
		return nil
	}
	aggs := &models.TxfeeAggregatesHistogram{
		StartTime: startTime,
		EndTime:   endTime,
	}
	ac.aggregateFeesMap[chainid][rangeKeyType] = aggs.TxfeeAggregates.Txfee
	return nil
}

func getFirstTransactionTime(conns *utils.Connections, chainIDs []string) (time.Time, error) {
	dbRunner, err := conns.DB().NewSession("get_first_transaction_time", cfg.RequestTimeout)
	if err != nil {
		return time.Time{}, err
	}

	var ts float64
	builder := dbRunner.
		Select("COALESCE(UNIX_TIMESTAMP(MIN(created_at)), 0)").
		From("avm_transactions")

	if len(chainIDs) > 0 {
		builder.Where("avm_transactions.chain_id IN ?", chainIDs)
	}

	err = builder.LoadOne(&ts)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(math.Floor(ts)), 0).UTC(), nil
}
