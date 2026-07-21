#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

unformatted=$(find . -name '*.go' -not -path './vendor/*' -print0 | xargs -0 -r gofmt -l)
if [ -n "$unformatted" ]; then
  printf 'gofmt required:\n%s\n' "$unformatted" >&2
  exit 1
fi

go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 run
go test ./...
