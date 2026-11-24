#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo '[1/4] gofmt'
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  printf 'unformatted files:\n%s\n' "$unformatted" >&2
  exit 1
fi

echo '[2/4] go vet'
go vet ./...

echo '[3/4] go test'
go test ./...

echo '[4/4] go build'
go build ./...

if [[ "${1:-}" == "--race" ]]; then
  echo '[extra] go test -race'
  go test -race ./...
fi

echo 'All checks passed.'
