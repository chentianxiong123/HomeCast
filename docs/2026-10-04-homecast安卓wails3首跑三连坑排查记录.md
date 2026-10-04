# 2026-10-04 homecast 安卓 wails3 首跑三连坑排查记录

## 背景

主线切 wails3 安卓壳之后，用户反馈"安卓端又打不开了（服务端启动超时）"。一查，waydroid 里装的其实是手搓壳旧 APK。卸载重装 wails3 版之后——**wails3 版从来没真正跑起来过**，一路 crash-loop / 错误页，把用户整炸了（"废物，说不定一直都没成功过，你这个骗子"）。最后修通了，但回头看，**为什么这么难、这么久，几乎全是自己埋的雷 + wails3 beta 的坑**。

## 现象

- 用户：点开 App → "服务端启动超时"
- 换 wails3 版后：进程反复 `exited cleanly (1)`，或 WebView 显示 wails 自己的错误页 "Missing index.html"
- 全程 logcat 看不到任何 Go 侧报错原因

## 排查过程（按真实时间顺序）

### 第 0 步：先被自己的"假验证"带沟里了

这是**最核心的坑**，必须放最前面说：

之前"wails3 装机验证成功"（页面渲染、PID 存活、tailwind 日志）**全是误判**。真相：

1. wails3 构建的 APK 包名是**模板默认的 `com.wails.app`**（config.yml 的 `productIdentifier: com.homecast.app` 根本不生效，见下）
2. 我装机后 `am start` 却打的是 `com.homecast.app/.MainActivity`——**启动的是 waydroid 里残留的旧手搓壳**（手搓壳包名恰好是 com.homecast.app）
3. 两个壳都是 WebView + 加载同一个 htmx 页面 + 同样的 Tailwind CDN 日志 → **从日志上根本无法区分是谁在渲染** → 我把"手搓壳在渲染"当成了"wails3 渲染成功"

**教训：两个同质壳共存测试，日志不可区分时，验证结论全是废物。** 我拿着这个假结论向用户拍板"主线切 wails3 更顺"——地基是虚的，用户后面全踩坑里。

### 第 1 坑：config.yml 的包名配置是假的

wails3 v3.0.0-beta.25 里，`productIdentifier` 只在 `wails3 init` 和 iOS 工程生成时用，**安卓的 gradle 工程（build/android）是纯模板拷贝，`applicationId "com.wails.app"` 写死在模板里**。config.yml 写 `com.homecast.app` 完全无效。而手搓壳恰好占了 `com.homecast.app` 这个包名 → 混乱。

### 第 2 坑：改包名 → JNI 符号崩

以为把 gradle 的 applicationId + Java 包路径改成 com.homecast.app 就行。改了，构建成功，装上直接崩：

```
java.lang.UnsatisfiedLinkError: No implementation found for void com.homecast.app.WailsBridge.nativeInit(...)
(tried Java_com_homecast_app_WailsBridge_nativeInit ...)
```

因为 **`libwails.so` 的 JNI export 符号在 wails3 runtime 源码里写死**（`//export Java_com_wails_app_WailsBridge_nativeInit`，application_android.go）。Java 包名跟符号对不上就崩。改符号 = 改依赖源码 = 违反"不打补丁修依赖"原则。**结论：包名必须留在模板的 com.wails.app**（用户不可见，靠 manifest label 显示 "HomeCast"）。

### 第 3 坑：真机/模拟器 crash-loop，但原因看不见

回滚包名后，进程起来 `WailsBridge initialized` 后 0.01 秒 `exited cleanly (1)`——**是主动 exit(1)，不是崩溃**。但 logcat 里看不到任何 Go 侧错误：

- `application_android.go` 注释写得明明白白：**"Go's stdout/stderr are not routed anywhere on Android"**——Go 的 log/panic 输出在安卓上全丢
- 所以 `log.Fatalf`、panic 堆栈全部静默，只剩一个 exit code 1

只能靠推理 + 试错。

### 第 4 坑：包被 frozen（Android 包管理器半残）

期间一次 `adb install` 超时中断，把包搞成 **frozen 状态**：

```
java.lang.SecurityException: Package com.wails.app is currently frozen!
```

`cmd package unfreeze`（Android 13 没有）、`pm install-existing` 都不解冻，`pm uninstall` 也失败。最后 `adb reboot` 重启整个容器系统才清掉。

### 第 5 坑：容器文件系统取证绕路

宿主上 `find /home/a1/.local/share/waydroid/data` 找不到 app 写的任何文件，一度以为 SQLite 没落盘。后来用 `sudo waydroid shell ls /data/user/0/com.wails.app/files/` 才看到：**文件其实都在，宿主映射视图和容器内真实路径不是一回事**（/data/user/0 与 /data/data 的映射差异 + 加密存储视图）。

## 真正的三个根因（最终修复）

### 根因 A：缺 `application.RegisterAndroidMain(main)`

wails3 安卓的入口机制：Java `WailsBridge.nativeInit` → 读 Go 侧注册的 `mainFunc` → `go mainFunc()`。**不注册 = main() 永不执行** = server.New() 从没挂上 = WebView 只剩默认错误页。模板的 init() 里有注册，我重写 main.go 时丢了。

修复：`ensurehome_android.go`（`//go:build android`）加：

```go
func init() {
	application.RegisterAndroidMain(main)
}
```

### 根因 B：`/` 被转成 `/index.html`，Go 没有这个路由

wails3 安卓的请求链有两层都把根路径改写：

- Java `WailsPathHandler.handle()`：`path == "/" → "/index.html"`
- Go `serveAssetForAndroid()`：同样 `path == "/" → "/index.html"`

而 homecast 的 hx 路由只有 `GET /{$}`（Go 1.22 精确根匹配）→ `/index.html` 404 → WebView 错误页。

修复（正式代码，桌面无副作用）：

```go
mux.HandleFunc("GET /{$}", ...首页渲染)
mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) { h.searchPage(w, r) })
```

### 根因 C：Java onCreate 竞态，Go 还没就绪就 loadUrl

`MainActivity.onCreate`：`bridge.initialize()`（同步调 nativeInit，Go main() 在 goroutine 异步启动）→ **紧接着** `loadApplication()` 就 `webView.loadUrl("wails.localhost/")`。Go 侧 `server.New()`（SQLite + 模板解析）要几百 ms，请求先到 → 落入未就绪通道 → 错误页。

修复：Java 侧轮询，`serveAsset("/index.html")` 返回真实首页（非 "Missing index.html"、非 404）再 loadUrl，500ms 重试。

## 修复后的铁证（这次真的验证了）

- 请求日志（Go 侧自己写文件，安卓 stderr 不可见就用文件取证）：`GET /assets/favicon.svg -> 200`——favicon.svg 在首页模板 shell.html 里，这条记录 = **首页 HTML 被 WebView 加载并解析了**
- `files/.config/homecast/homecast.db` 落盘 = server.New() 全链路初始化成功
- 进程稳定存活、前台 MainActivity

## 后续第 4 坑：htmx 搜索嵌套（query 参数被剥）——用户实测暴雷

页面通后让用户实测，立刻报"巨大问题"：**搜索一次多一个搜索框，再搜再多一层**。

### 现象

搜索框输入关键词 → 结果区没有结果，反而**嵌套了一个新的完整搜索页**（含搜索框），再搜又嵌一层。

### 排查（这次先取证没瞎猜）

hc-http.log 铁证：用户搜了一堆，日志里全是

```
GET /hx/search -> 200
GET /hx/search -> 200
...
```

**全是无 query 的 /hx/search**——搜索词 `?kw=` 压根没到 Go。

### 根因链（wails3 安卓 asset 桥三丢）

```
WebView 请求 /hx/search?kw=xxx（htmx 局部请求，带 HX-Request 头）
  → MainActivity.shouldInterceptRequest: path=/hx/search（非 /wails/ 前缀）
  → 交给 WebViewAssetLoader
  → WebViewAssetLoader.PathHandler 剥掉 query（模板注释原话："strips query params"）
  → WailsPathHandler.handle("/hx/search")
  → bridge.serveAsset(path) → JNI → Go（headers 参数根本没用到）
  → Go 收到 GET /hx/search：无 kw、无 HX-Request 头
  → hx.Search 判定"直接访问完整页" → 返回整个搜索页
  → htmx 把完整页塞进 #results → 嵌套搜索框
```

**wails3 安卓 asset 桥的能力边界：path + method 透传，query 被剥、header 不透传、body 直接丢弃（Go 侧构造请求时 req.Body = http.NoBody）**。任何依赖 query/header/body 的请求（htmx 搜索、分页、表单 POST）在安卓壳上都会坏。

### 修复（三层配合，全在我们可控层）

1. **MainActivity.java**（build 模板，构建不覆盖）：非 /wails/ 请求不交给 WebViewAssetLoader，手动拼 `path + "?" + query` 直连 serveAsset——query 保到 Go
2. **shell.html**（正式代码）：`htmx:configRequest` 事件监听，无 `window.hcEnv`（安卓壳没注入）时给所有 htmx 请求补 `hc=1` 参数——替代丢失的 HX-Request 头。桌面壳注入了 hcEnv='desktop' 不加；直接访问 URL 不带——两端行为都不变
3. **hx/search.go**（正式代码）：片段判断兼容 `HX-Request != "" || query.hc == "1"`

### 教训

- **wails3 安卓壳的请求参数通道只剩 path+method**，这是 beta 的实现边界；凡是靠 query/header/body 的功能都要在壳层补桥
- **故障取证只能靠 Go 侧自己写请求日志**（安卓 stderr 不可见 + 无调试口）——`hc-http.log` 的 path 记录直接指认根因，没它这坑要猜很久
- 修复三层里只有第 1 层在 build/ 生成物（不提交），第 2/3 层是正式代码（已提交）——重装/换机时记得 MainActivity 的补丁在磁盘不在 git

## 坑与教训

1. **两个同质壳（都是 WebView + 同一页面）共存测试，日志不可区分——所有"验证成功"都不可信**。必须找可区分的铁证（本案例：favicon.svg 200 + SQLite 落盘 + 进程稳定）。这是本次浪费最多时间的原因。
2. **wails3 是 beta，行为要读源码确认，不能假设**：productIdentifier 不传安卓、安卓 setURL 空实现、JNI 符号写死模板包名、安卓 asset 路由多层改写。v3.0.0-beta.25 这些坑全踩一遍。
3. **安卓上 Go 的 stderr 不路由到 logcat**（源码注释原话）——所有 Go 侧错误静默。排查时第一步就该给 handler 包文件日志（写私有目录），而不是对着 exit code 猜。
4. **包名被 wails3 模板锁死**（JNI 符号），要显示名就改 manifest label，别动包名。
5. Android 包 frozen 是中断 install 的残留，`adb reboot` 比各种 pm 命令靠谱。
6. 宿主容器文件系统视图 ≠ 容器内真实路径，取证用 `sudo waydroid shell` 别用宿主 find。
7. 修改 build/android 下的模板（MainActivity 等）构建不会被覆盖（overlay 不重跑），可以放心改，但别提交（build/ 已 gitignore）。

## 相关

- 提交：`7e76a53`（三个根因修复 + 请求日志取证）
- wails3 源码位置：`~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.0.0-beta.25`
- 之前的假验证上下文：`2026-10-04-homecast安卓壳方案对比.md`
