# HomeCast iOS 版中文字体豆腐块排查与根治

## 背景

HomeCast（B站音乐播放器）用 wails3 v3.1.0 做壳，前后端全走真实 HTTP（`http://127.0.0.1:28976`），前端 go:embed 内嵌。最近给 macOS/iOS 都建了版，macOS 中文正常，**iOS 版一启动界面布局/图标全对，但所有文字（中英数）全是豆腐块（❓/方块）**。用户截图确认，系统设置 App / Safari 自己 UI 中文正常，说明不是模拟器系统字体坏了，是我们页面问题。

## 问题现象

- iOS 26.3 模拟器（iPhone 17 Pro，x86_64 Intel Mac 上跑）里 HomeCast 界面：布局、SVG 图标、渐变按钮全正常，但**搜索框 placeholder、导航、按钮、所有文字**都是豆腐块。
- 系统 App（设置）、Safari 原生 UI 中文正常。
- **Safari 打开我们后端 `http://127.0.0.1:28976` 也豆腐** → 问题在我们后端返回的 HTML，不是 wails3 WebView 配置，不是模拟器字体。

## 排查过程

### 1. 先排除 wails3 WebView 配置差异（用户怀疑 macOS 有优化、iOS 没有）

对比 wails3 v3.1.0 的 `webview_window_darwin.go` 和 `webview_window_ios.m`：

- 两者配置**完全一致**：`suppressesIncrementalRendering`、`applicationNameForUserAgent`、URL scheme handler、message handler。
- **wails3 没对 macOS 做字体/编码优化**，iOS 同源。**排除 wails3 层差异**。

### 2. 排除后端响应编码

curl 我们后端首页，`Content-Type: text/html; charset=utf-8` 正确，shell.html 有 `<meta charset="UTF-8">`。**charset 正确**。排除编码问题。

### 3. 决定性测试：Safari 加载百度 vs 我们页面

- Safari 打开百度 → **中文完全正常**。
- Safari 打开我们后端 → **豆腐**。

结论：**iOS 模拟器 WKWebView 字体没问题，是我们前端页面的问题**。

### 4. 决定性测试：纯内联 HTML（无 Tailwind）看字体

在后端临时加 `/test-font` 路由，返回纯内联 HTML（无 Tailwind、无外部资源）：

```html
<body style="font-family:-apple-system">
  <h1>测试中文字体 abc123</h1>
  <p style="font-family:-apple-system">苹方 abc123 中文渲染</p>
  <p style="font-family:ui-sans-serif,system-ui,sans-serif">ui-sans-serif 测试中文</p>
</body>
```

Safari 打开 → **`-apple-system` 段：英文 `abc123` 正常、中文豆腐；`ui-sans-serif,system-ui` 段：全豆腐**。

**关键发现**：`-apple-system` 英文（SF）正常、中文（fallback 苹方）豆腐；`ui-sans-serif` 全豆腐。而系统 UI / 百度（默认字体栈）中文正常。

### 5. 用 JS 查实际生效的 font-family（决定性）

在 shell.html 临时加调试脚本，页面加载后把 body/input/导航元素的实际 `getComputedStyle().fontFamily` 大字显示在页面上：

```
BODY_FF:-apple-system, BlinkMacSystemFont, "PingFang SC", ...
INPUT_FF:-apple-system, BlinkMacSystemFont, "PingFang SC", ...
EL_FF(a.flex...):-apple-system, BlinkMacSystemFont, "PingFang SC", ...
```

- **调试面板自己文字正常**（-apple-system 英文能渲染）。
- **页面背景文字仍豆腐**（同样的 -apple-system 栈）。

**确诊**：`-apple-system`/`BlinkMacSystemFont` 在 iOS 26.3 模拟器 WKWebView 里**中文 fallback 失败**——英文走 SF 正常，中文要 fallback 到苹方却失败，渲染成豆腐。**问题不在 font-family 栈本身生效与否，而是 `-apple-system` 的中文 fallback 在 iOS 这个 WebKit 上不工作**。

## 解决方案

**全局字体栈把 `PingFang SC` 放最前**，中文直接走苹方，不依赖 `-apple-system` 的中文 fallback（英文苹方也有，不会丢英文）：

`go/web/assets/style.css`：

```css
html, body, button, input, select, textarea, div, span, p, a, li, h1, h2, h3, h4, h5, h6 {
  font-family: "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Segoe UI", -apple-system, sans-serif !important;
}
```

`go/web/templates/shell.html` 的 `tailwind.config` 同步：

```js
tailwind.config = { darkMode: 'class', theme: { extend: { fontFamily: { sans: ['"PingFang SC"', '-apple-system', '"Hiragino Sans GB"', '"Microsoft YaHei"', '"Segoe UI"', 'sans-serif'] } } } }
```

重编 iOS → **全中文正常**："搜索音乐"、搜索框 placeholder、"搜索"按钮、"输入关键词开始搜索"全部正常渲染。用户确认"彻底跑通了"。

## 坑与教训

1. **`-apple-system` 中文 fallback 在 iOS 26.3 模拟器 WKWebView 失败**——这是 iOS 模拟器 WebKit 的一个坑，不是编码问题，不是 Tailwind 问题。排查时如果只看 font-family 是否生效（computed 是 -apple-system）会误判——**字体栈生效 ≠ 中文能渲染**，得靠"英文正常/中文豆腐"这个分叉来判断。
2. **Tailwind CDN 的 `ui-sans-serif` 也是豆腐元凶之一**（`font-sans` 默认栈首项），但它只是表象；真正根因是 iOS WebKit 对这类"泛型/系统字体名"的中文 fallback 不给力。**最稳的做法是显式中文字体置顶**。
3. **别只靠 CSS font-family 判断**——测试页证明 `-apple-system` 英文正常中文豆腐，单看 computed font-family 会以为没问题。
4. **调试脚本别留产品代码**——临时 `/test-font` 路由、页面大字调试面板，修完都要清干净（已移除/还原）。
5. 排查顺序建议：先纯内联 HTML 测试页（无框架）区分"框架问题"vs"字体问题"，再查 computed font-family，最后针对性改。
