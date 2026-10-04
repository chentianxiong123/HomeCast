#!/usr/bin/env bash
# 🐧 Linux 端构建（统一壳 wails3/）
# 依赖：本机 GTK3 + WebKitGTK 开发库（wails3 后端 + 挂件都是 cgo）
set -e
cd "$(dirname "$0")/.."
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o bin/hcexp-linux .
echo "OK → wails3/bin/hcexp-linux ($(du -h bin/hcexp-linux | cut -f1))"
echo "运行: ./bin/hcexp-linux   （HC_NO_WIDGET=1 可关挂件）"