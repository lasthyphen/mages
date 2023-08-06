# Magellan

A data processing pipeline for the [Camino network](https://camino.foundation).

## Features

- Maintains a persistent log of all consensus events and decisions made on the Camino network.
- Indexes Exchange (X), Platform (P), and Contract (C) chain transactions.
- An [API](https://docs.camino.foundation/apis/magellan) allowing easy exploration of the index.

## Prerequisite

https://docs.docker.com/engine/install/ubuntu/

https://docs.docker.com/compose/install/

## Quick Start with Standalone Mode on Columbus (testnet) network

The easiest way to get started is to try out the standalone mode.

```shell script
git clone https://github.com/lasthyphen/mages.git $GOPATH/github.com/lasthyphen/mages
cd $GOPATH/github.com/lasthyphen/mages
make dev_env_start
make standalone_run
```

## [Production Deployment](docs/deployment.md)

