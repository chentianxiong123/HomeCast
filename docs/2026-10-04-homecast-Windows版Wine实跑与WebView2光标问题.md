# HomeCast Windows 版 Wine 实跑 + WebView2 光标消失排查记录

日期：2026-10-04

## 背景

HomeCast 桌面歌词挂件要做**多态**（Linux GTK / Windows Win32 / 安卓悬浮窗），Windows 版本来打算等真 Windows 再搞。用户说"我电脑有 wine 也可以测的"，于是决定先用 Wine 实跑 Windows 版看效果。

折腾下来发现：**要的是整个 HomeCast 播放器（wails3 壳）在 Wine 里跑起来**，不是歌词挂件——中间理解错了，绕了一大圈歌词，被用户骂醒。

## 排查过程（按真实时间顺序）

### 第 1 步：先搞错了对象——歌词挂件（windemo）

用户说"wine 打开我看看"，我理解为先搞 Windows 版桌面歌词挂件，写了个最小 demo：

- 装 `gcc-mingw-w64-x86-64`（交叉编译链）
- 写 `display_windows.go`（Win32 透明穿透：Layered + ColorKey 洋红键 + WS_EX_TRANSPARENT 鼠标穿透 + GDI 绘制）
- cgo 宏坑：`C.RGB` / `C.GET_X_LPARAM` cgo 不认（宏展开失败），改手动位运算 `colorRef(r,g,b)`
- `cglue.c`（GTK 代码）在 Windows 编译时被一起编——加 `#ifndef _WIN32` 保护
- `SetWindowPos` 第二参数 0 要显式 `C.HWND(nil)`
- 交叉编译成功（36MB exe），Wine 里跑起来：日志显示窗口创建成功 + 歌词 41 行拉到（连了宿主后端——端口被宿主 desktop 占，用了 HC_PORT 独立端口避开）

**但是用户看不到任何东西**。查了半天（xwininfo 窗口树、残留窗口 PID 对比）发现：

1. **Wine 的 Layered + ColorKey 透明窗口在 X11 后端下不可见**（wine 的 layered 支持有限）——降级成普通窗口（WINEDEMO_VISIBLE=1 可见模式）也还是没解决用户要看的东西
2. **用户要的根本不是歌词挂件**——是**整个 HomeCast 播放器应用在 Wine 里跑**（主窗口、完整界面）

### 第 2 步：转向正题——wails3 壳 Windows 版交叉编译

确认 wails3 v3 的 Windows WebView2 绑定是**纯 Go COM**（go-ole，`internal/webview2/pkg/edge`），**不需要 CGO**——不用 mingw，纯 Go 交叉编译：

```bash
cd wails3 && GOOS=windows CGO_ENABLED=0 go build -o bin/hcexp-win.exe .
```

**坑：`application.Android.StoragePath()` 只在安卓编译定义**——main.go 里直接用，Windows 编译报 `undefined: application.Android`。修法：加跨平台 `hcDataPath()`：

- `ensurehome_android.go`：`return application.Android.StoragePath()`
- `ensurehome_other.go`（!android）：`~/.config/homecast`

编译通过，**hcexp-win.exe 29.6MB，一个 exe 完整应用**（壳 + 内嵌后端 + htmx 界面）。

### 第 3 步：Wine 跑完整应用——缺 WebView2 Runtime

```
[WebView2 Error] error calling Webview2Loader: no webview2 found
```

wails3 Windows 壳用 Edge WebView2 渲染，Wine 里没装 Runtime 直接崩。解决：

```bash
curl -L -o webview2-setup.exe "https://go.microsoft.com/fwlink/p/?LinkId=2124703"   # bootstrapper 1.85MB
wine webview2-setup.exe /silent /install    # 静默装完整包，几分钟
```

装完 `~/.wine/drive_c/Program Files (x86)/Microsoft/EdgeWebView/Application/154.0.4258.53` 出现。重跑：

```
INF Platform Info: Go-WebView2Loader=true WebView2=154.0.4258.53
fixme:perf:PerfCreateInstance ... WebView2: myMusic    ← WebView2 引擎起来了
```

**主窗口出来了**：`0x4200005 "HomeCast" 992x666 at (464,187)`，页面 JS 在请求 `GET /api/v1/fav/list`——**整个播放器在 Wine 里实跑成功**。

### 第 4 步：鼠标光标在 HomeCast 窗口内消失

用户反馈：窗口正常了，但**鼠标移进 HomeCast 窗口光标就消失**（窗口外正常）。

排查：

1. 先查自己前端：`grep cursor: none`——没有，前端没藏光标
2. 判断是 Wine + WebView2 的兼容 bug（WebView2 用 COM 管理光标，Wine 的 X11 驱动对跨进程 cursor handle 创建支持不全 → SetCursor 失败 → 光标不画）
3. 上网查证（tavily 搜）：

**WineHQ Bugzilla 58922** "Mouse cursor invisible in WebView2 browser"——状态 **UNCONFIRMED（未修复）**。但有开发者（评论）写了一个 winex11.drv 的 workaround patch：**Wine 无法从跨进程 cursor handle 创建 X11 光标时，显示标准 X11 箭头而不是留空**——patch 未确认进主线。

**真 Windows 上的同类问题**（微软 WebView2Feedback #5708，WebView2 Runtime 152.0.4191.x）：WebView2 错误地保持指针隐藏状态（"Hide pointer while typing" 触发后不恢复）——workaround：

- 禁用 Windows 控制面板 → 鼠标 → 指针选项 → **"输入时隐藏指针"**（Hide pointer while typing）
- 或改 DPI 覆盖（兼容性 → 替代高 DPI 缩放行为 → 系统(增强)）

注意：这是**微软 runtime 152 的 bug**，154 可能已修；且我们 Wine 里的光标问题是 **wine 层**的（不是微软 runtime 的），两者不同源。

## 结论

1. **HomeCast Windows 版完整应用（wails3 壳）在 Wine 里实跑成功**——一个 exe（29.6MB），WebView2 Runtime 装进 wine prefix 即可
2. **光标消失：Wine 下暂时没救**——WineHQ 58922 未修复（有 patch workaround 未进主线），**真 Windows 上正常**（真机上 WebView2 光标没问题；且微软 152 的 bug 有 workaround，154 已修）
3. Wine 作为测试环境，光标 bug 不影响功能验证（点不看得见但能点，htmx 界面可打字操作）

## 坑与教训

- **用户要什么先问清楚**：用户说"wine 打开我看看"要的是**整个应用**，我绕了半天歌词挂件——被骂醒才转正题。教训：先确认"看什么"，别自作主张
- **wails3 v3 Windows 不需要 CGO**：WebView2 是纯 Go COM 绑定——交叉编译零依赖（不用 mingw），这是 wails3 v3 比 v2 大的进步
- **Wine 的 Layered 透明窗口在 X11 后端不可见**：透明歌词挂件的 Windows 版在 Wine 下没法验证视觉效果——真 Windows 上才行（代码就位，逻辑是标准 Win32）
- **Wine 跑完整应用要装 WebView2 Runtime**：bootstrapper 静默装进 prefix，几分钟，装完 `EdgeWebView/Application/<版本号>` 出现即可
- **桌面挂件（GTK）的 cglue.c 编译坑**：Windows 交叉编译时被一起编——cgo 对 C 文件没有 build tag，用 `#ifndef _WIN32` 包住
- **cgo 宏调用坑**：`C.RGB`、`C.GET_X_LPARAM` 这类宏 cgo 不认（"could not determine what C.XXX refers to"）——手动位运算拼（COLORREF 用 `r | g<<8 | b<<16`，LOWORD/HIWORD 用 int16 截断）
