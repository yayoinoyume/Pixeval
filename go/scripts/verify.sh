#!/usr/bin/env bash
# M0 验收脚本：在 go/ 下运行。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== gofmt =="
test -z "$(gofmt -l . | grep -v '^$')" || { gofmt -l .; exit 1; }

echo "== vet =="
go vet ./...

echo "== test =="
go test -race ./...

echo "== build =="
go build -o /tmp/pixevalgo-verify .

echo "== size =="
ls -lh /tmp/pixevalgo-verify
rm -f /tmp/pixevalgo-verify

echo "OK"
