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
	"strings"
	"testing"

	"github.com/lasthyphen/mages/cfg"
)

func TestParse(t *testing.T) {
	var err error
	var dsn string
	dsn, err = ForceParseTimeParam("mysql://root:password@tcp(mysql:3306)/magellan_dev")
	if err != nil || dsn != "mysql://root:password@tcp(mysql:3306)/magellan_dev?parseTime=true" {
		t.Fatal("Unexpected dsn")
	}
	dsn, err = ForceParseTimeParam("root:password@tcp(mysql:3306)/magellan_dev")
	if err != nil || dsn != "root:password@tcp(mysql:3306)/magellan_dev?parseTime=true" {
		t.Fatal("Unexpected dsn")
	}
	dsn, err = ForceParseTimeParam("root:password@tcp(mysql:3306)/magellan_dev?xyz=123")
	if err != nil || dsn != "root:password@tcp(mysql:3306)/magellan_dev?parseTime=true&xyz=123" {
		t.Fatal("Unexpected dsn")
	}
}

func TestNewErrors(t *testing.T) {
	conn, err := New(&EventRcvr{}, cfg.DB{
		Driver: "mysql",
		DSN:    "---",
	}, false)

	if conn != nil {
		t.Fatal("Expected conn to be nil")
	}
	if !strings.HasPrefix(err.Error(), "invalid DSN") {
		t.Fatal("Expected an invalid DSN error")
	}

	conn, err = New(&EventRcvr{}, cfg.DB{
		Driver: "mysql",
		DSN:    "::a.a.a.a.a.a.a.a::",
	}, false)

	if conn != nil {
		t.Fatal("Expected conn to be nil")
	}
	if !strings.HasPrefix(err.Error(), "invalid DSN") {
		t.Fatal("Expected an invalid URI")
	}

	conn, err = New(&EventRcvr{}, cfg.DB{
		Driver: "mysql",
		DSN:    "a:b@tcp(1.2.3.4)/foo",
	}, false)
	if conn != nil {
		t.Fatal("Expected conn to be nil")
	}

	if err == nil {
		t.Fatal("Expected i/o or context deadline timeout")
	}
}
