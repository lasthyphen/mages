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

package cfg

const (
	keysNetworkID    = "networkID"
	keysLogDirectory = "logDirectory"
	keysFeatures     = "features"

	keysChains       = "chains"
	keysChainsID     = "id"
	keysChainsVMType = "vmtype"

	keysServices = "services"

	keysServicesAPIListenAddr     = "listenAddr"
	keysServicesAdminListenAddr   = "adminListenAddr"
	keysServicesMetricsListenAddr = "metricsListenAddr"

	keysServicesDB       = "db"
	keysServicesDBDriver = "driver"
	keysServicesDBDSN    = "dsn"
	keysServicesDBRODSN  = "ro_dsn"

	keysStreamProducerCaminogo     = "caminogo"
	keysStreamProducerNodeInstance = "nodeInstance"

	keysStreamProducerCchainID = "cchainID"
)
