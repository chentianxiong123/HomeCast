# wails3/ — 🤖🪟 安卓 + Windows 统一壳（纯 Go，零 C 依赖）

**这个目录是什么**：HomeCast 的**跨平台薄壳**。同一个 `main.go`，不加一行平台代码，编出：

| 端 | 产物 | 渲染 | 编译方式 |
|---|---|---|---|
| 🤖 安卓 | `bin/hcexp.apk` | WebView（JNI 桥） | `task android:package ARCH=amd64` |
| 🪟 Windows | `bin/hcexp-win.exe` | WebView2（COM syscall） | `GOOS=windows CGO_ENABLED=0 go build` |

核心逻辑：`application.New` + `WebviewWindow` + `Assets.Handler`（反向代理到 `go/` server）——壳只负责窗口和生命周期，业务在 `go/`。

**为什么它是纯 Go**：WebView2 的 COM 接口是文档化 ABI（go-ole 纯 Go syscall 调）；安卓走 wails3 内置 JNI 桥。两者都不碰 C → 交叉编译零依赖。**代价：不带桌面歌词挂件**（挂件是 cgo，见 `desktop/`）。

## 构建

```bash
# Windows exe（无 CGO 交叉编译）
GOOS=windows CGO_ENABLED=0 go build -o bin/hcexp-win.exe .

# 安卓 APK（x86_64 模拟器；真机改 ARCH=arm64）
task android:package ARCH=amd64
```

## 平台骨架

```
main.go                 # 统一入口（窗口 + Assets.Handler 反代 go/ server）
ensurehome_android.go   # //go:build android —— HOME 指向私有 filesDir + 注册 JNI 入口
ensurehome_other.go     # //go:build !android —— hcDataPath 到 ~/.config/homecast（Windows/Linux 共用）
```

## 已知坑（详见 docs/）

- **包名锁死 `com.wails.app`**：wails3 安卓 JNI export 符号写死，不可改（改包名要 fork 依赖）
- **安卓 asset 桥三丢**：query 被剥 / header 不透传 / body 丢弃 → 前端参数一律走 query + `hc=1` 标记；收藏/歌词源上报已按此绕
- **APK 被冻结**（task 报错）：`adb reboot` 清
- **SQLite 驱动**：`ncruces/go-sqlite3`（modernc 老 syscall 触发安卓 seccomp SIGSYS——已根治）
- **Wine 跑 exe**：prefix 里要先装 WebView2 Runtime；渲染区光标不显示（WineHQ 58922，真 Windows 正常）

## 数据落盘

- 安卓：`filesDir/.config/homecast/`（`hc-http.log` + `homecast.db`）
- Windows/Linux：`~/.config/homecast/`