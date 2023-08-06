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

const defaultJSON = `{
  "networkID": 1001,
  "logDirectory": "/tmp/magellan/logs",
  "listenAddr": ":8080",
  "chains": {},
  "services": {
    "db": {
      "dsn": "root:password@tcp(127.0.0.1:3306)/magellan_dev",
      "driver": "mysql"
    }
  }
}`
