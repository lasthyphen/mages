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

package api

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/lasthyphen/dijetsnodego/ids"
	"github.com/lasthyphen/mages/caching"
	"github.com/lasthyphen/mages/cfg"
	"github.com/lasthyphen/mages/models"
	"github.com/lasthyphen/mages/services"
	"github.com/lasthyphen/mages/services/indexes/djtx"
	"github.com/lasthyphen/mages/servicesctrl"
	"github.com/lasthyphen/mages/stream/consumers"
	"github.com/gocraft/web"
)

// Server is an HTTP server configured with various magellan APIs
type Server struct {
	sc     *servicesctrl.Control
	server *http.Server
}

// NewServer creates a new *Server based on the given config
func NewServer(sc *servicesctrl.Control, conf cfg.Config) (*Server, error) {
	router, err := newRouter(sc, conf)
	if err != nil {
		return nil, err
	}

	// Set address prefix to use the configured network
	models.SetBech32HRP(conf.NetworkID)

	return &Server{
		sc: sc,
		server: &http.Server{
			Addr:              conf.ListenAddr,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       15 * time.Second,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, err
}

// Listen begins listening for new socket connections and blocks until closed
func (s *Server) Listen() error {
	s.sc.Log.Info("server listening",
		zap.String("addr", s.server.Addr),
	)
	return s.server.ListenAndServe()
}

// Close shuts the server down
func (s *Server) Close() error {
	s.sc.Log.Info("Server shutting down")
	ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()
	return s.server.Shutdown(ctx)
}

func newRouter(sc *servicesctrl.Control, conf cfg.Config) (*web.Router, error) {
	sc.Log.Info("creating new router",
		zap.Stringer("chainID", sc.GenesisContainer.XChainID),
	)
	var xChainID, cChainID ids.ID
	for key, chain := range conf.Chains {
		switch chain.VMType {
		case models.CVMName:
			cChainID, _ = ids.FromString(key)
		case models.AVMName:
			xChainID, _ = ids.FromString(key)
		}
	}

	indexBytes, err := newIndexResponse(
		conf.NetworkID,
		xChainID,
		cChainID,
		sc.GenesisContainer.DjtxAssetID,
	)
	if err != nil {
		return nil, err
	}

	legacyIndexResponse, err := newLegacyIndexResponse(
		conf.NetworkID,
		sc.GenesisContainer.XChainID,
		sc.GenesisContainer.DjtxAssetID,
	)
	if err != nil {
		return nil, err
	}

	// Create connections and readers
	connections, err := sc.DatabaseRO()
	if err != nil {
		return nil, err
	}

	cache := caching.NewCache()
	delayCache := caching.NewDelayCache(cache)

	consumersmap := make(map[string]services.Consumer)
	for chid, chain := range conf.Chains {
		consumer, err := consumers.IndexerConsumer(conf.NetworkID, chain.VMType, chid, &conf)
		if err != nil {
			return nil, err
		}
		consumersmap[chid] = consumer
	}
	djtxReader, err := djtx.NewReader(conf.NetworkID, connections, consumersmap, sc)
	if err != nil {
		return nil, err
	}

	ctx := Context{sc: sc}

	// Build router
	router := web.New(ctx).
		Middleware(newContextSetter(sc, conf.NetworkID, connections, delayCache)).
		Middleware((*Context).setHeaders).
		Get("/", func(c *Context, resp web.ResponseWriter, _ *web.Request) {
			if _, err := resp.Write(indexBytes); err != nil {
				sc.Log.Warn("response write failed",
					zap.Error(err),
				)
			}
		}).
		NotFound((*Context).notFoundHandler).
		Middleware(func(c *Context, w web.ResponseWriter, r *web.Request, next web.NextMiddlewareFunc) {
			c.djtxReader = djtxReader
			c.djtxAssetID = sc.GenesisContainer.DjtxAssetID

			next(w, r)
		})

	AddV2Routes(&ctx, router, "/v2", indexBytes, nil)

	// Legacy routes.
	AddV2Routes(&ctx, router, "/x", legacyIndexResponse, &sc.GenesisContainer.XChainID)
	AddV2Routes(&ctx, router, "/X", legacyIndexResponse, &sc.GenesisContainer.XChainID)

	return router, nil
}
