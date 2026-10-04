#!/usr/bin/env bash
# 🪟 Windows 端构建（统一壳 wails3/，交叉编译）
# 依赖：mingw-w64（apt install gcc-mingw-w64-x86-64）—— 挂件 Win32 层是 cgo，必须带 C 工具链
set -e
cd "$(dirname "$0")/.."
GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -trimpath -ldflags="-s -w" -o bin/hcexp-win.exe .
echo "OK → wails3/bin/hcexp-win.exe ($(du -h bin/hcexp-win.exe | cut -f1))"
echo "运行（真机）: 装 WebView2 Runtime 后运行 exe；Wine 测试: wine hcexp-win.exe"