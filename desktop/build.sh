#!/usr/bin/env bash
# 构建桌面壳（Wails v2）：窗口直载内嵌 htmx 后端，无前端构建（Vue 已废弃停用）
set -e
cd "$(dirname "$0")"
export PATH="$PATH:$HOME/go/bin"
exec wails build -tags webkit2_41 "$@"
