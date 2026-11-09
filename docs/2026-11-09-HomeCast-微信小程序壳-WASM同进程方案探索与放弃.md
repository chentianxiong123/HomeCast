# 小程序 WASM 同进程方案探索（已放弃，代码留档于此）

> 2026-11-09 探索记录。**结论：放弃**。代码移到 `explore/wasm-mp/` 留档，壳撤回 `web-view` 版本。

## ① 目标与动机

让 Go 后端（业务逻辑**一份**）编译成 wasm 直接跑在**微信小程序进程**里，**不需要网络服务器**（服务对象只有小程序自己）。小程序侧注入全局 `fetch`（`wx.request` 适配），Go 的 `net/http` 在 js/wasm 下自动走宿主 fetch（Go 内置 `roundtrip_js.js`），业务逻辑零改动。渲染用 mp-html（**wasm 逻辑 + mp-html 显示**结合）。

## ② 资源拓扑

- **小程序 WASM 支持**：`WXWebAssembly`（基础库 ≥2.13.0），`instantiate(path, imports)`，imports 传宿主函数（WASM 调宿主机制）；支持 `.wasm.br` brotli 压缩。
- **Go wasm 运行时胶水**：`wasm_exec.js`——Go 官方版（无 wasi）/ tinygo 配套版（**含 `wasi_snapshot_preview1` stub**），两者不通用。
- **Go net/http js/wasm fetch 桥**：`/usr/local/go/src/net/http/roundtrip_js.go`——`http.Get` 自动走宿主全局 `fetch`。
- **小程序侧注入**：`globalThis.fetch`（wx.request 适配）+ polyfill（`TextDecoder`/`TextEncoder`/`crypto`/`performance`/`global`/`fs`）。
- **编译产物**：`backend.wasm.br`（brotli q=11）。

## ③ 工具链清单

| 工具 | 版本 | 用途 | 安装 |
|---|---|---|---|
| Go | 1.26.5 | 标准 wasm（`GOOS=js GOARCH=wasm`） | 系统 `/usr/local/go` |
| tinygo | 0.41.0 | 压体积 wasm（3.2M→brotli 775KB） | `~/tinygo41/bin/tinygo` |
| go1.24 | 1.24.10 | tinygo 0.41 的 fork 匹配（net/http 桥适配 go1.24） | `~/go124` |
| brotli | python3 | `quality=11` 压缩 | `python3 -c "import brotli"` |
| 微信开发者工具 | 2.02.2609250 | 模拟器编译/预览 | Mac `/Applications/wechatwebdevtools.app` |

> tinygo 0.37 只支持 go≤1.24（`got go1.26` 报错）；0.41 用 go1.26 但 net/http fork 落后 → 需匹配 go1.24。

## ④ 成功路径（已验证部分）

- **Go 标准 wasm 编译**：`GOOS=js GOARCH=wasm go build -ldflags "-s -w" -o backend.wasm ./cmd/wasmmp` → 9.7M。
- **tinygo 编译**：`GOROOT=~/go124 PATH=~/go124/bin:$PATH ~/tinygo41/bin/tinygo build -o backend.wasm ./cmd/wasmmp` → 3.2M → **brotli 775KB**，主包 802KB < 2M（OK）。
- **胶水验证（node 模拟小程序环境）**：`require wasm_exec.js` 成功、`new Go()` OK、`importObject` 含 `wasi_snapshot_preview1/gojs/env`。
- **关键代码模式**：
```go
js.Global().Set("__searchBili", js.FuncOf(func(this js.Value, args []js.Value) any {
    go func() { html := searchHTML(kw); cb.Invoke(html) }() // 网络放 goroutine，go.run 事件循环驱动
    return nil
}))
select{} // 保活：Go 运行时事件循环持续驱动
```

## ⑤ 踩坑记录（每条：现象 → 根因 → 解法）

1. **wasm 9.9M 超主包 2M** → 微信编译/加载闪退 → brotli q=11 压到 1.9M（Go 标准）；**tinygo 压到 775KB**。
2. **wasm_exec.js 顶层 `if (!globalThis.TextDecoder/crypto/TextEncoder) throw`** → 小程序没有这些全局 → require 崩 → **白屏无报错** → 先注 polyfill。
3. **wasmexport 同步调用不驱动 Go 调度器**（goroutine 不跑）→ 必须 `js.FuncOf` 注册回调 + `go.run` 事件循环 + `select{}` 保活。
4. **`crypto/rand` 在 js/wasm 挂起** → 用 `math/rand`。
5. **B站防风控**必须带 `Cookie: buvid3=<随机uuid>`。
6. **tinygo `-target=wasm` 输出 wasi**（`llvm-target: wasm32-unknown-wasi`，import `wasi_snapshot_preview1`）→ 小程序无 wasi → `instantiate: import 2 wasi_snapshot_preview1: module is not an object or function` → **换 tinygo 配套 wasm_exec.js**（含 wasi stub）。
7. **tinygo 配套 wasm_exec.js 需要 `global`/`fs`**（小程序无 global/window/self）+ `require("node:fs")` 崩 → 注入 `global=globalThis` + `fs` stub 跳过。
8. **tinygo 0.41 fork net/http 落后 go1.26**（`roundtrip_js.go: t.roundTrip undefined`）→ 修 fork（fallback 返回错误）或用匹配 go1.24。
9. **tinygo 0.37 只支持 go≤1.24**（`got go1.26`）→ 换 0.41。
10. **微信工具日志（WeappLog）不含页面 console**（模拟器内存）→ 只能让用户贴 Console 报错，无法自主抓取。

## ⑥ 验证成果

- node 模拟：`require tinygo wasm_exec.js` 成功（Go 类）、`new Go()` OK、importObject 含 `wasi_snapshot_preview1`。
- tinygo wasm 775KB brotli、主包 802KB < 2M（**解决包超限**）。
- 小程序模拟器：早期（Go wasm_exec.js）能出画面但 `__searchBili 未定义`（包超限 → wasm 没加载）；tinygo 换胶水后**"连骨架都不出"**（疑似 BigInt，未确认）。
- **最终未在小程序模拟器完整跑通搜索**（用户决定放弃）。

## ⑦ 决策记录

- **选 wasm 同进程**（不选网络服务器）：逻辑一份住 Go、小程序薄壳、服务对象只有小程序自己。
- **但小程序 WXWebAssembly 环境坑多且深**：wasi 协议、polyfill 一堆、BigInt 疑似、Go/tinygo 胶水不通用——**难稳定**。
- **放弃 wasm 路径**（用户决定）→ 回 **web-view（A 方案）**：一行 `<web-view>` 加载 H5 整站，零重写、JS 全跑，最优解。
- **mp-html 高频刷新（B 方案）未放弃**（备选）：后端吐 HTML + 小程序 500ms 轮询，逻辑在 Go 后端（本地 HTTP），昨天验证过流畅。

## ⑧ 未解之谜 / 下一步岔路

- **BigInt**（tinygo wasm_exec.js 196/294 行）是否真崩——未确认（用户放弃没验证）。
- wasm 搜索在小程序模拟器/真机是否通——未验证。
- **若未来回头**：tinygo + wasi stub + polyfill + 分包，链路已摸到"就差搜索回调"。
- **下一步（非 wasm）**：回 web-view 版本，真机扫码（miniprogram-ci + 正式账号）验证；B 完整版（mp-html + 真实播放状态）备选。
