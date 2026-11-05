#!/usr/bin/env bash
# 在 wails3 generate 生成 build/android/ 后，把本目录沉淀的真实 HTTP 版
# 安卓壳文件覆盖回去（否则默认生成的是 asset 桥版，非真实 HTTP 架构）。
#
# 用法：bash android-patches/apply.sh   （从 wails3/ 或 android-patches/ 均可）
set -euo pipefail
cd "$(dirname "$0")"

# 目标 = wails3/build/android/app/src/main（generate 生成的安卓工程）
DEST=../build/android/app/src/main

[[ -d "$DEST" ]] || { echo "✗ 未找到 $DEST——请先 wails3 generate"; exit 1; }

cp MainActivity.java            "$DEST/java/com/wails/app/MainActivity.java"
cp network_security_config.xml  "$DEST/res/xml/network_security_config.xml"
cp AndroidManifest.xml          "$DEST/AndroidManifest.xml"

echo "✓ android-patches 已应用（真实 HTTP 版覆盖默认 asset 桥版）"
