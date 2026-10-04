#!/usr/bin/env bash
# 🐧 Linux 端构建（统一壳 wails3/）
# 依赖：本机 GTK3 + WebKitGTK 开发库（wails3 后端 + 挂件都是 cgo）
# -tags gtk3：壳用 GTK3 后端（webkit2gtk-4.1）——与 GTK3 歌词挂件同库共存；
#            默认（无 gtk3 tag）后端是 GTK4，与 GTK3 挂件同进程直接崩
#            （GTK 2/3 symbols detected，GTK4 禁止与 GTK2/3 共存）
set -e
cd "$(dirname "$0")"
CGO_ENABLED=1 go build -tags gtk3 -trimpath -ldflags="-s -w" -o bin/hcexp-linux .
echo "OK → wails3/bin/hcexp-linux ($(du -h bin/hcexp-linux | cut -f1))"
echo "运行: ./bin/hcexp-linux   （HC_NO_WIDGET=1 可关挂件）"