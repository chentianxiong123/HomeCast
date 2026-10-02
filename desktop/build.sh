#!/usr/bin/env bash
# 构建桌面壳：前端构建 → dist 同步 → wails build（webkit2gtk-4.1）
set -e
cd "$(dirname "$0")"

echo "==> 构建前端"
(cd ../frontend && npm run build)

echo "==> 同步 dist 到 desktop/dist"
rm -rf dist
cp -r ../frontend/dist dist

echo "==> wails build (webkit2gtk-4.1)"
export PATH="$PATH:$HOME/go/bin"
exec wails build -tags webkit2_41 "$@"