# wails3/ — 统一壳（🐧 Linux 🪟 Windows 🤖 安卓，一个入口三端构建）

HomeCast 的应用壳。**同一个 `main.go`**，平台差异全部走同目录 build tag 文件，构建脚本按端分开。

```
wails3/
├── main.go                    # 统一入口：内嵌 go/ 后端 + WebView 直载 htmx + 生命周期
├── ensurehome_android.go      # 🤖 安卓：HOME→私有 filesDir + JNI 注册
├── ensurehome_other.go        # 🐧🪟 桌面：数据目录 ~/.config/homecast
├── widget_desktop.go          # 🐧🪟 桌面歌词挂件启动
├── widget_android.go          # 🤖 挂件 no-op（悬浮窗待写）
├── http_desktop.go            # 🐧🪟 局域网 0.0.0.0 监听 + hcEnv 注入
├── http_android.go            # 🤖 no-op
├── widget/                    # 挂件多态核心（display.go 接口 + linux/windows 实装 + 纯核心）
├── windemo/                   # 🪟 Windows 挂件最小 demo
├── build-linux.sh             # 🐧 构建
├── build-windows.sh           # 🪟 构建
├── build-android.md           # 🤖 构建
└── bin/                       # 三端产物
```

## 构建

| 端 | 命令 | 产物 | 依赖 |
|---|---|---|---|
| 🐧 Linux | `./build-linux.sh` | `bin/hcexp-linux` | 本机 GTK3 + WebKitGTK |
| 🪟 Windows | `./build-windows.sh` | `bin/hcexp-win.exe` | mingw-w64（交叉） |
| 🤖 安卓 | `task android:package ARCH=amd64` | `bin/hcexp.apk` | Android SDK/NDK |

## 平台分层逻辑

- **公共代码**（main.go）：后端 handler、日志、窗口、挂件启动调用点
- **build tag 文件**：# 平台差异的最小化薄层（数据路径/挂件开关/HTTP 增强），`//go:build android` / `//go:build !android`
- **挂件**（widget/）：三端共用一个接口，显示层按端实现（GTK/Win32/安卓悬浮窗待写）
- **Go 语言约束**：平台适配文件必须和 main.go 同目录同包 —— 所以端差异体现在**文件名后缀**而不是子目录

## 挂件（widget/）

```
widget/
├── display.go          # Display/Canvas 接口（多态）
├── display_linux.go    # 🐧 GTK3 实装（cgo）—— 在用
├── display_windows.go  # 🪟 Win32 实装（cgo，Layered+ColorKey 透明穿透）—— 待真机验证
└── widget.go           # 纯核心（tick/绘制/交互，三端共享）
```

`windemo/`：Windows 挂件独立 demo（`GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o windemo.exe ./windemo/`，Wine 里验证 GUI）。

## 已知坑（详见 docs/）

- **安卓 asset 桥三丢**：query 剥 / header 不透传 / body 丢弃 → 前端参数一律走 query + `hc=1` 标记
- **包名锁死 `com.wails.app`**（JNI export 符号写死）；APK 被冻结 `adb reboot` 清
- **SQLite 驱动**：`ncruces/go-sqlite3`（modernc 老 syscall 触发安卓 seccomp SIGSYS）
- **Wine + WebView2**：渲染区光标不显示（WineHQ 58922，真 Windows 正常）；Wine 需先装 WebView2 Runtime
- **挂件 = cgo**：Windows 交叉编译必须 mingw（`build-windows.sh` 已配）

## 数据落盘

- 🤖 安卓：`filesDir/.config/homecast/`（hc-http.log + homecast.db）
- 🐧🪟 桌面：`~/.config/homecast/`