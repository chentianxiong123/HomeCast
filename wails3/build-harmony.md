# 🧭 鸿蒙版打法（规划，2026-11-08 沉淀）

产物：`hcexp.hap`（鸿蒙 App 包）。**规划阶段**——工具链/真机到位后动工。

## 核心理念（沿用项目原则）

- **前端↔后端真实 HTTP**（`http://127.0.0.1:28976`），壳只当系统壳（窗口/WebView/系统API）
- **逻辑一份住后端（Go）**——鸿蒙版后端 = 现有 `go/cmd/server`（纯 Go），**不改逻辑**
- 前端复用 go:embed 的 HTML/JS/CSS（ArkWeb 加载同源）

## 决定性前提（调研结论）

1. **Go 官方无 `ohos` target**；**社区 patch 版 `star4277/ohos-go`**（go1.26.5 + `GOOS=openharmony`，源码 `src/internal/syslist/syslist.go` 第 30/53 行 `"openharmony": true`）——**跟 HomeCast go 1.26 匹配**。另有 `sysit/ohos-go`（go1.27.3-ohos）、`yourblacksky/ohos_golang_go`（Go 1.25.12）。
2. **HomeCast 后端纯 Go 零 CGO**（`ncruces/go-sqlite3` WASM，全项目无 `import "C"`）→ ohos-go 交叉编译**可行，无需鸿蒙 NDK/cgo**。
3. **wails3 不支持鸿蒙**（linux/windows/darwin/android/ios）→ 鸿蒙版**不用 wails3 壳**，壳用 **ArkTS 自建**（DevEco + ArkWeb + Native .so）。
4. **Go 官方另认 HarmonyOS**：`GOOS=android` + OHOS NDK clang（HarmonyOS 兼容安卓 ABI，issue #82260 FixPending，c-shared TLS bug 官方在修）。

## 环境（win11 鸿蒙开发机，已确认够）

DevEco Studio 26.0 + OpenHarmony SDK（`sdk\default\openharmony`，hdc 在）+ **模拟器 Emulator26.0** + Go 1.27 + Node 24 + Java 21 + WebView2 155。

## Go 鸿蒙交叉编译（ohos-go，独立工具链）

```bash
# 独立工具链（不碰系统 go /usr/local/go）
git clone https://github.com/star4277/ohos-go ~/ohos-go
cd ~/ohos-go/src && ./make.bash        # 产出 ~/ohos-go/bin/go（独立 GOROOT）

# 交叉编译 HomeCast 后端为鸿蒙 c-shared 库（纯 Go 零 CGO，无需 NDK）
cd /path/to/homecast/go
GOROOT=~/ohos-go ~/ohos-go/bin/go build -buildmode=c-shared -o hcexp.so ./cmd/server
```

- **不冲突**：独立目录 + 独立 GOROOT + 显式路径调用；系统 go 零影响（前提：别装进 `/usr/local/go`、别把 ohos-go 放 PATH 前面）
- `cmd/server` 需加 `//export StartServer`（启动 HTTP server goroutine）供 ArkTS 调；`StopServer` 退出
- 产物 `hcexp.so`（Go HTTP server）+ `hcexp.h`（导出头）

## 鸿蒙壳（ArkTS，DevEco 自建）

1. **ArkWeb**（WebView 组件）加载 `http://127.0.0.1:28976`——需 `ohos.permission.INTERNET` + **明文 HTTP 网络安全配置**（127.0.0.1 例外，同安卓 cleartext 处理）
2. **Native .so 加载 Go 后端**：ArkTS `NativeModule` 加载 `hcexp.so` → 调 `StartServer` → Go HTTP server 在 .so 里跑（`127.0.0.1:28976`）
3. 前端：go:embed 的 HTML/JS/CSS 搬进鸿蒙 `entry/src/main/resources/rawfile`（ArkWeb 加载同源，或直接 HTTP）

## 平台解耦

- 鸿蒙版后端 = `go/cmd/server`（纯 Go，**无 wails3 壳依赖**——鸿蒙不用 wails3）
- 壳层 no-op：无需 `widget_route_*`/`ensurehome_*`/`http_*`（那些是 wails3 壳层，鸿蒙版不用）
- Go 文件分类：`//go:build`（darwin/linux/windows/ios/android）已清查；鸿蒙壳在 ArkTS，不混 Go

## 已知约束 / 坑

- **ArkWeb 明文 HTTP**（`127.0.0.1:28976`）需网络安全配置允许（同安卓 `network_security_config`）
- **Go .so 在 HarmonyOS 的 TLS bug**（`#82260` FixPending）——c-shared 库 TLS 未初始化可能崩，需关注官方修复/绕行
- **包名自定**（非 wails3 的 `com.wails.app`）——鸿蒙 bundle id 独立
- **Go 后端数据落盘**：鸿蒙沙盒（`@ohos.data` / 应用目录），需 `ensurehome` 适配鸿蒙路径
- **模拟器架构**：win11 DevEco 模拟器 x86_64/arm64 按需（Go `GOARCH` 匹配）

## 歌词挂件（规划）

- 鸿蒙**悬浮窗**（SDK 26+ 标准 Floating Window / `createSubWindow` / 全局悬浮窗）**可行**
- 数据源 = SSE 订阅（复用 `speaker/events` 单流，带算好的 `line`）
- 控制条/穿透逻辑：照 Windows GDI / macOS Canvas 模式（ArkTS ArkUI 绘制）

## 验证路径（照 iOS 模式）

1. ohos-go 编译 `cmd/server` → `hcexp.so`（本机验证）
2. DevEco 建 ArkWeb 壳 → 加载 `http://127.0.0.1:28976`（模拟器）
3. Native .so 加载 + `StartServer` → 页面/API 通
4. 模拟器 UI 验证（首页/搜索/播放/队列/收藏）
5. 歌词挂件（悬浮窗）后续
