#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -e

if ! [[ "$0" =~ scripts/lint.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

TARGET="./..."

go install -v github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2
golangci-lint run --config .golangci.yml

echo "ALL SUCCESS!"
