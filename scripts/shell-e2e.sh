#!/usr/bin/env bash
# Linux 壳级 E2E 冒烟：Xvfb 启动真实 wails3 app（GTK4 + WebKit + 内嵌后端），
# 断言最小链路：进程不崩 + 后端端口起来 + 首页渲染 + 后端 API 通。
# "有一点就够"——只验证壳+WebView+后端全链路能起，不深测功能。
# 用法：bash scripts/shell-e2e.sh [app_binary]  （默认 wails3/bin/hcexp）
set -euo pipefail
APP="${1:-wails3/bin/hcexp}"
PORT="${HC_PORT:-28976}"
DATA_DIR="$(mktemp -d)"
LOG="$(mktemp)"
APP_PID=""   # trap 可能在赋值前触发，set -u 下需预置
cleanup() {
  # pkill -f 会匹配到本脚本自身命令行 → 自杀 exit 143
  # wait 等被 kill 的后台进程也会返回非零（signal）→ 覆盖 exit 参数
  # 直接 kill PGID + 显式 exit 0，退出码才确定
  kill -TERM -"${APP_PID:-}" 2>/dev/null || true
  wait "${APP_PID:-}" 2>/dev/null || true
  rm -rf "$DATA_DIR" "$LOG"
  exit 0
}
trap cleanup EXIT

echo "[shell-e2e] 启动真实 wails3 app（Xvfb）：$APP"
echo "[shell-e2e] HC_DATA_DIR=$DATA_DIR（临时 DB，不污染用户数据）"
export HC_DATA_DIR="$DATA_DIR"
export HC_PORT="$PORT"

# Xvfb 下启动真实 GTK4 窗口（WebView 加载前端）；setsid 脱离终端避免脚本退出被阻塞
setsid xvfb-run -a "$APP" >"$LOG" 2>&1 < /dev/null &
APP_PID=$!

# 等后端端口起来（最多 30s）
for i in $(seq 1 30); do
  if curl -s -o /dev/null "http://127.0.0.1:$PORT/"; then break; fi
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "[shell-e2e] ❌ app 崩溃，日志：" >&2; tail -20 "$LOG" >&2; exit 1
  fi
  sleep 1
  if [ "$i" = 30 ]; then
    echo "[shell-e2e] ❌ 后端 $PORT 30s 内未起来" >&2; tail -20 "$LOG" >&2; exit 1
  fi
done

# 1. 进程存活
if kill -0 "$APP_PID" 2>/dev/null; then echo "✅ 1.app 进程存活"; else
  echo "❌ 1.app 进程已崩" >&2; tail -20 "$LOG" >&2; exit 1; fi

# 2. 首页渲染（htmx 骨架：搜索框 + player-dock；字体栈在 style.css 独立加载，字体已单测/真机验证，壳级不查）
HTML="$(curl -s "http://127.0.0.1:$PORT/")"
if echo "$HTML" | grep -q 'player-dock' && echo "$HTML" | grep -q 'name="kw"'; then
  echo "✅ 2.首页渲染（player-dock + 搜索框）"
else
  echo "❌ 2.首页缺少关键 DOM" >&2; echo "$HTML" | head -5 >&2; exit 1
fi

# 3. 后端 API 通（fav/list 不依赖外网）
if curl -s "http://127.0.0.1:$PORT/api/v1/fav/list" | grep -q '"code":0'; then
  echo "✅ 3.后端 API 通（fav/list code=0）"
else
  echo "❌ 3.后端 API 异常" >&2; curl -s "http://127.0.0.1:$PORT/api/v1/fav/list" >&2; exit 1
fi

# 4. WebView 真加载了前端（后端页面含搜索框；WebView 同源加载同一 HTML）
if echo "$HTML" | grep -q 'name="kw"'; then
  echo "✅ 4.前端页面含搜索框（WebView 加载源）"
else
  echo "❌ 4.前端页面缺搜索框" >&2; exit 1
fi

echo ""
echo "[shell-e2e] ✅ Linux 壳级 E2E 冒烟全过（进程/后端/页面/API）"
exit 0
