# HomeCast

<p align="center">
  <b>🏠 家庭影音平台 · B站音乐播放器</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/htmx-2.x-3366CC?logo=htmx" alt="htmx">
  <img src="https://img.shields.io/badge/wails3-beta.25-3B3B5C?logo=wails" alt="wails3">
  <img src="https://img.shields.io/badge/SQLite-内嵌-003B57?logo=sqlite" alt="SQLite">
</p>

---

## 多端适配

**一个壳（`wails3/`），三端构建**：同一个 `main.go` 入口，平台差异走同目录的 build tag 文件。

```
                    ┌─────────────────────────────────────────┐
                    │  go/ 业务核心（三端共用，一个端口跑全部）    │
                    │  服务端 + htmx 前端 + SQLite 全内嵌        │
                    └──────────────────┬───────────────────────┘
                                       │
              ┌────────────────────────┼────────────────────────┐
              ▼                        ▼                        ▼
   ┌───────────────────┐    ┌───────────────────┐    ┌───────────────────┐
   │ 🐧 Linux          │    │ 🪟 Windows        │    │ 🤖 安卓            │
   │ wails3/           │    │ wails3/           │    │ wails3/           │
   │ hcexp-linux       │    │ hcexp-win.exe     │    │ hcexp.apk         │
   │ GTK 歌词挂件 ✓    │    │ Win32 挂件（待真机）│    │ 悬浮窗挂件（待写）  │
   └───────────────────┘    └───────────────────┘    └───────────────────┘
```

| 端 | 产物 | 挂件状态 | 构建 |
|---|---|---|---|
| 🐧 Linux 桌面 | `bin/hcexp-linux` | ✅ GTK3 透明穿透挂件在用 | `./build-linux.sh` |
| 🪟 Windows | `bin/hcexp-win.exe` | 🟡 Win32 层代码就位，待真机验证 | `./build-windows.sh` |
| 🤖 安卓 | `bin/hcexp.apk` | 🔲 悬浮窗待写 | `task android:package ARCH=amd64`（见 `build-android.md`） |

**统一壳的平台分层**（`wails3/` 内按文件名标明端）：

```
wails3/
├── main.go                    # 统一入口：内嵌后端 + WebView 直载 htmx + 生命周期
├── ensurehome_android.go      # 🤖 安卓：HOME→filesDir + JNI 注册
├── ensurehome_other.go        # 🐧🪟 桌面：数据目录 ~/.config/homecast
├── widget_desktop.go          # 🐧🪟 桌面歌词挂件启动（GTK/Win32 显示层）
├── widget_android.go          # 🤖 挂件 no-op（悬浮窗待写）
├── http_desktop.go            # 🐧🪟 局域网 0.0.0.0 监听 + hcEnv 注入
├── http_android.go            # 🤖 no-op（wails 直走 handler）
├── widget/                    # 挂件多态核心
│   ├── display.go             #   Display/Canvas 接口（三端共享）
│   ├── display_linux.go       #   🐧 GTK3 实装（cgo）
│   ├── display_windows.go     #   🪟 Win32 实装（cgo，待真机）
│   └── widget.go              #   纯核心（tick/绘制/交互）
├── windemo/                   # 🪟 Windows 挂件最小 demo（wine 验证 GUI）
├── build-linux.sh             # 🐧 构建
├── build-windows.sh           # 🪟 构建
├── build-android.md           # 🤖 构建
└── README.md                  # 三端构建/坑
```

> 挂件为什么是 cgo：GTK/Win32 都是 C 库，Go 调 C 库只有 cgo 一条路（编译要各自 C 工具链）。
> 壳本体（WebView）不需要：WebView2 是文档化 COM（纯 Go syscall）、安卓走 JNI 桥、Linux 用 WebKitGTK（cgo）。
> 即：**凡带挂件的端 = 必须 cgo 编译**；这是语言边界，不是取舍。

---

## 架构演进：Python 探索 → Go 实现

核心技术路线 **「Python 探索、Go 实现」**：

- **`explore/`（历史只读，不维护）** —— 探索路径收纳：
  - `explore/backend/`（Python/FastAPI）：早期验证玩法（B站音乐、DLNA、音箱、嗅探）
  - `explore/frontend/`（Vue 前端）：htmx 重构前的旧界面
  - `explore/android/`（手搓 WebView 壳）：安卓壳早期实现，兜底参考
- **`go/` 正式实现** —— 单二进制：htmx + Tailwind + SQLite（驱动 `github.com/ncruces/go-sqlite3`，纯 Go/WASM，规避安卓 seccomp），全部内嵌
- **`wails3/` 统一壳** —— wails3 v3 固定 `v3.0.0-beta.25`，三端构建
- Python 端沉淀的接口语义（`/api/v1/*` 前缀、`{code,message,data}` 信封）已 1:1 对齐进 Go 实现

---

## 功能

- 🎧 **B站音乐** — 搜索 / 收藏 / 播放 / 本地歌单，滑块细节与 YesPlayMusic 对齐
- 🪟 **桌面歌词挂件** — 主界面跟随歌词 + 桌面穿透挂件（悬停浮现控制条/锁定态/小锁）
- 📺 **DLNA 投屏** — 自动发现设备，一键投屏视频
- 🔊 **小米音箱** — 登录 / 设备管理 / 音乐推送
- 📱 **三端界面同一套** — htmx 服务端渲染，壳只负责窗口和生命周期

---

## 构建

### 🐧 Linux 桌面

```bash
cd wails3 && ./build-linux.sh        # 产 bin/hcexp-linux（含 GTK 挂件）
```

### 🪟 Windows（交叉编译）

```bash
cd wails3 && ./build-windows.sh      # 产 bin/hcexp-win.exe（mingw cgo，含 Win32 挂件）
```

真机：装 WebView2 Runtime 后直接跑。Wine：先用 `wine bin/hcexp-win.exe`（Wine 内需装 WebView2 Runtime；已知：渲染区内光标不显示，见 docs/）。

### 🤖 安卓 APK

```bash
cd wails3 && task android:package ARCH=amd64   # x86_64 模拟器（waydroid）；真机 arm64
```

详见 `wails3/build-android.md`。包名锁死 `com.wails.app`（wails3 安卓 JNI 符号写死）。

### Go 服务单独跑（调试）

```bash
cd go && go run ./cmd/server
```

---

## 项目结构

```
homecast/
├── go/          # 业务核心：cmd/server（入口）+ web（htmx/模板/静态）+ internal（api/service/store）
├── wails3/      # 统一壳（三端）：main.go + 平台 build tag 文件 + widget/ 挂件多态 + 三端构建脚本
├── explore/     # 历史只读：backend/（Python）/ frontend/（Vue）/ android/（手搓壳）
├── docs/        # 排查/架构记录（troubleshooting 风格）
├── scripts/     # E2E 冒烟等
└── README.md
```

---

## 已踩的关键坑（详见 docs/）

- 安卓 SIGSYS：SQLite 驱动换 `ncruces/go-sqlite3`（modernc 老 syscall × seccomp）
- wails3 安卓 asset 桥三丢（query 剥/header 丢/body 弃）→ 前端传参数走 query + `hc=1` 标记
- 安卓包名锁死 `com.wails.app`；APK 被冻结需 `adb reboot` 清
- Wine/WebView2：光标在渲染区内消失（WineHQ 58922 未修复，真 Windows 正常）
- cgo 宏不认（`C.RGB` 等）→ 手动位运算；`cglue.c` 用 `#ifndef _WIN32` 保护
- wails3 安卓 asset 桥只透传 path+method：query/header/body 全丢——依赖这些的功能都必须走 query

## License

MIT