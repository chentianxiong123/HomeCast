#!/usr/bin/env bash
# 起小程序后端（局域网可达，供真机扫码访问）：
#   mockbili（假 B站 :39001）+ Go server（:28976，监听 0.0.0.0）
# 用法：scripts/mp-backend.sh
set -euo pipefail
cd "$(dirname "$0")/../go"

echo "[1/2] 起 mockbili :39001 (假 B站)"
HC_BILI_BASE=http://127.0.0.1:39001 HC_DATA_DIR=/tmp/hc-mp go run ./cmd/mockbili > /tmp/hc-mp-mockbili.log 2>&1 &
echo $! > /tmp/hc-mp-mockbili.pid

sleep 1
echo "[2/2] 起 homecast server :28976 (0.0.0.0)"
HC_BILI_BASE=http://127.0.0.1:39001 HC_DATA_DIR=/tmp/hc-mp nohup go run ./cmd/server > /tmp/hc-mp-server.log 2>&1 &
echo $! > /tmp/hc-mp-server.pid

sleep 3
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "后端已起："
echo "  本机 curl http://127.0.0.1:28976/hx/search-mp?kw=test"
echo "  手机访问 http://${IP}:28976 (填进 wails3/build/wechat/utils/config.js 的 BASE)"
echo "停止：kill \$(cat /tmp/hc-mp-mockbili.pid) \$(cat /tmp/hc-mp-server.pid)"
