#!/usr/bin/env bash
# 构建桌面壳（Wails v3）：窗口直载内嵌 htmx 后端，无前端构建（Vue 已废弃停用）
# v3 = 普通 Go 程序（CGO 链接 GTK4 + WebKitGTK-6.0），go build 即出包
set -e
cd "$(dirname "$0")"
go build -trimpath -ldflags="-s -w" -o homecast-desktop .
echo "OK → desktop/homecast-desktop ($(du -h homecast-desktop | cut -f1))"