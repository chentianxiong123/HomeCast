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

## 多端适配总览

```
                    ┌─────────────────────────────────────────┐
                    │  core/ = go/ 业务核心（三端共用）         │
                    │  服务端 + htmx 前端 + SQLite 全内嵌       │
                    │  一个端口 / 一个二进制，反复用            │
                    └──────────────────┬──────────────────────┘
                                       │
             ┌─────────────────────────┼────────────────────────┐
             ▼                         ▼                        ▼
   ┌───────────────────┐   ┌────────────────────┐   ┌────────────────────┐
   │ desktop/          │   │ wails3/            │   │ wails3/            │
   │ 🐧 Linux 桌面壳    │   │ 🤖 安卓 APK        │   │ 🪟 Windows exe     │
   │ WebView 窗口       │   │ (JNI 桥 + WebView)  │   │ (纯 Go COM 无 CGO)  │
   │ + GTK 歌词挂件     │   │ hcexp.apk          │   │ hcexp-win.exe      │
   │ homecast-desktop  │   └────────────────────┘   └────────────────────┘
   └───────────────────┘
```

**壳的分工（为什么 Linux 单独一个目录）**：

- `desktop/` 集成了**桌面歌词挂件** → 挂件是 cgo（GTK/Win32 都是 C 库，Go 调 C 库只能 cgo）→ 编译要各平台 C 工具链 → 只能做本机 Linux 壳
- `wails3/` **不带挂件** → 零 C 依赖（WebView2 走文档化 COM 由纯 Go syscall 调、安卓走 JNI 桥）→ `CGO_ENABLED=0` 一个命令交叉编出安卓 APK + Windows exe

**挂件多态层**（`desktop/widget/`，一套接口三实现）：

```
widget/
├── display.go          # Display/Canvas 接口（多态）
├── display_linux.go    # GTK3 实装（cgo）      —— 已在用
├── display_windows.go  # Win32 实装（cgo）     —— 代码就位，待真机验证
└── display_android.go  # 悬浮窗 Service         —— 待写
```

---

## 三端支持

| 端 | 壳 | 状态 | 说明 |
|---|---|---|---|
| 🐧 Linux 桌面 | `desktop/`（wails3 v3 + GTK 挂件） | ✅ 正式用 | 主窗口 WebView + 透明穿透桌面歌词挂件 |
| 🪟 Windows | `wails3/` → `hcexp-win.exe` | 🟡 交叉编译通过，Wine 实跑通 | 纯 Go（WebView2 走 COM，无 CGO）；挂件 Win32 层代码就位待真机验证 |
| 🤖 安卓 | `wails3/` → `hcexp.apk` | 🟡 壳+持久化+升级实测过 | 悬浮窗歌词挂件待写 |

> 一律一个二进制：后端 + 前端（htmx 全内嵌）+ SQLite 全塞进壳里，一个端口跑全部，无构建链依赖。

---

## 架构演进：Python 探索 → Go 实现

核心技术路线 **「Python 探索、Go 实现」**：

- **`explore/`（历史只读，不维护）** —— 两条探索路径的收纳：
  - `explore/backend/`（Python/FastAPI）：早期验证玩法（B站音乐、DLNA、音箱、嗅探）
  - `explore/android/`（手搓 WebView 壳）：安卓壳早期实现，兜底参考
- **`go/` 正式实现** —— 单二进制：htmx + Tailwind + SQLite（驱动 `github.com/ncruces/go-sqlite3`，纯 Go/WASM，规避安卓 seccomp），全部内嵌
- **`desktop/` Linux 桌面壳** —— wails3 v3（`Assets.Handler` 反向代理到 go/ server）+ 桌面歌词挂件（GTK 多态显示层）
- **`wails3/` 安卓+Windows 统一壳** —— wails3 v3 固定 `v3.0.0-beta.25`；安卓 WebView 渲染同套 htmx 界面
- Python 端沉淀的接口语义（`/api/v1/*` 前缀、`{code,message,data}` 信封）已 1:1 对齐进 Go 实现

### 为什么两个壳

- `desktop/` 集成 GTK 画笔挂件（cgo）→ **仅 Linux 编译**
- `wails3/` 无平台耦合 → **安卓 APK + Windows exe 都从它出**
- wails3 生态里桌面挂件无现成方案，挂件是自己写的多态显示层：

```
widget/
├── display.go          # Display/Canvas 接口（多态）
├── display_linux.go    # GTK3 实装（cgo）
├── display_windows.go  # Win32：Layered+ColorKey 透明穿透（真机验证待定）
└── widget.go           # 纯核心（tick/绘制/交互，三端共享）
```

---

## 功能

- 🎧 **B站音乐** — 搜索 / 收藏 / 播放 / 本地歌单，滑块细节与 YesPlayMusic 对齐
- 🪟 **桌面歌词挂件** — 主界面跟随歌词 + 桌面穿透挂件（悬停浮现控制条/锁定态）
- 📺 **DLNA 投屏** — 自动发现设备，一键投屏视频
- 🔊 **小米音箱** — 登录 / 设备管理 / 音乐推送
- 📱 **三端界面同一套** — htmx 服务端渲染，壳只负责窗口和生命周期

---

## 构建

### Linux 桌面

```bash
cd desktop && ./build.sh        # 产 homecast-desktop，含 GTK 挂件
```

### Windows（交叉编译，纯 Go 无 CGO）

```bash
cd wails3 && GOOS=windows CGO_ENABLED=0 go build -o bin/hcexp-win.exe .
```

运行（真机）：装 WebView2 Runtime 后直接跑 `hcexp-win.exe`。
Wine 测试：`wine bin/hcexp-win.exe`（Wine 内需先装 WebView2 Runtime；已知限制：WebView2 区域内光标不显示，见 docs/）。

### 安卓 APK

```bash
cd wails3 && task android:package ARCH=amd64   # 产 bin/hcexp.apk（x86_64 模拟器）
# 真机改 ARCH=arm64；包名 com.wails.app（wails3 安卓 JNI 符号写死，不可改）
```

### Go 服务单独跑（调试）

```bash
cd go && go run ./cmd/server    # 一个端口跑全部（htmx 界面 + API）
```

---

## 项目结构

```
homecast/
├── go/          # core：三端共用业务核心（cmd/server + web 前端 + internal）—— 壳都反代到它
├── desktop/     # 🐧 Linux 端壳：wails3 v3 + GTK 歌词挂件（widget/ 多态层在此）—— README 见目录内
├── wails3/      # 🤖🪟 安卓+Windows 端壳：纯 Go 交叉编 APK/exe —— 不带挂件（cgo）—— README 见目录内
├── explore/     # 历史只读：backend/（Python 探索）+ frontend/（Vue 旧前端）+ android/（手搓壳早期）
├── docs/        # 排查/架构记录（troubleshooting 风格）
├── scripts/     # E2E 冒烟等
└── README.md
```

---

## 已踩的关键坑（详见 docs/）

- 安卓 SIGSYS：SQLite 驱动换 `ncruces/go-sqlite3`（modernc 老 syscall × seccomp）
- wails3 安卓 asset 桥三丢（query 剥/header 丢/body 弃）→ 前端传参数走 query + `hc=1` 标记
- 安卓包名锁死 `com.wails.app`（JNI 符号写死）；apk 被冻结需 `adb reboot` 清
- Wine/WebView2：光标在渲染区内消失（WineHQ 58922 未修复，真 Windows 正常）
- cgo 宏不认（`C.RGB` 等）→ 手动位运算；`cglue.c` 用 `#ifndef _WIN32` 保护

## License

MIT