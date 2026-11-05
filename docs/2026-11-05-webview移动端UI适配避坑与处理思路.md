# WebView 移动端 UI 适配——避坑与处理思路

日期：2026-11-05

## 背景

HomeMusic（HomeCast）在安卓**真机**上首次跑通真实 HTTP 后，UI 出现明显偏差：搜索按钮消失、空态太大、滚动条冒出、元素挤在一块。而**安卓容器（waydroid）和桌面浏览器看都正常**，只有真机坏。

本文记录**为什么容器正常、真机坏**，以及这类 WebView UI 问题的**通用处理思路**，避免以后再踩。

---

## 一、根因：容器/桌面与真机的"屏幕密度"不同

同一份 CSS，在不同设备上**视觉大小不同**，本质是两件事：

| 环境 | 物理分辨率 | 密度 (dpi) | **CSS 逻辑宽** | 表现 |
|---|---|---|---|---|
| waydroid 容器 | 540×960 | 低 (~160) | **~540px** | 看着正常 |
| Redmi 真机 | 1080×2400 | 高 (~440) | **~393px** | 挤、按钮消失、滚动条 |
| 桌面 | 1920×1080 | 96 | ~1920px | 正常 |

**关键结论**：
1. **CSS 逻辑宽**决定布局（真机只有 ~393px，比容器窄 27%、比桌面窄更多）。前端若用桌面固定 `px`，窄屏必然挤。
2. **容器/模拟器 dpr 低**（逻辑宽大）会**掩盖** UI 问题——**容器正常 ≠ 真机正常**。测 UI 必须上真机。
3. 桌面用 `px` 没问题，因为屏幕宽；**移动端必须响应式**。

---

## 二、这轮实际踩的坑（现象 → 根因 → 修法）

### 坑 1：搜索按钮"消失"
- **现象**：真机搜索胶囊只有输入框，右侧粉色"搜索"按钮没了；容器正常。
- **根因**：胶囊是 `flex` 行，`<input>` 是 `flex-1`，但 `<button>` **没有 `flex-shrink-0`**。窄屏下 `flex-1` 把 button **压缩到 0 宽度**（按钮被挤没）。容器宽，button 有空间，所以正常。
- **修法**：button 加 `flex-shrink-0`；input 加 `min-w-0`（可收缩不撑破）。
  ```html
  <input class="flex-1 min-w-0 ...">
  <button class="flex-shrink-0 ...">搜索</button>
  ```
- **教训**：**所有 flex 行里的可点元素必须 `flex-shrink-0`**，否则窄屏被挤没。

### 坑 2：空态太大
- **现象**：空态页（"输入关键词开始搜索"等）上下留白巨大。
- **根因**：`py-20`/`py-24`（80/96px）是桌面值，窄屏太空。
- **修法**：`py-20`/`py-24` → `py-14`；空态图标 `w-16 h-16` → `w-12 h-12`；空态标题 `text-lg` → `text-base`。

### 坑 3：大按钮太宽
- **现象**：`px-8` 的按钮（搜索/去搜索/导入登录）在窄屏占太宽。
- **修法**：`px-8`/`px-6` → `px-5`（保留 `sm:` 桌面值）。

### 坑 4：滚动条冒出
- **现象**：真机出现横向/竖向滚动条，容器没有（窄屏内容溢出 + WebView 默认显示滚动条）。
- **修法**：
  - WebView 层：`webView.setVerticalScrollBarEnabled(false)`、`setHorizontalScrollBarEnabled(false)`、`setOverScrollMode(OVER_SCROLL_NEVER)`
  - CSS 层：`html, body { overflow-x: hidden; }` + `*::-webkit-scrollbar { display: none; }`

### 坑 5：桌面固定 px 在窄屏挤
- **现象**：列表项（封面/按钮/标题）用桌面尺寸，393px 挤。
- **修法**：只调**移动默认值**，`sm:`（≥640 桌面）保留——桌面零影响。例：
  - 封面 `w-16 h-12 sm:w-20 sm:h-14` → `w-12 h-9 sm:w-20 sm:h-14`
  - 标题 `text-base sm:text-lg` → `text-sm sm:text-lg`
  - 按钮 `w-9 h-9` → `w-8 h-8`

### 坑 6（关联）：viewport 不完整
- **修法**：`<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover">`（Wails 官方移动指南标配，`viewport-fit=cover` 处理刘海/安全区）。

### 坑 7（非 UI 但同源）：主线程网络
- **现象**：安卓启动白屏/黑屏，后端监听正常、curl 通，但 WebView 不加载。
- **根因**：`isBackendReady()` 的 `HttpURLConnection` 在**主线程**执行，安卓抛 `NetworkOnMainThreadException`（StrictMode 禁主线程网络），永远 false → 永不 `loadUrl`。
- **修法**：网络请求放**后台线程**（`new Thread(...)`），成功后再 `runOnUiThread` 切主线程 `loadUrl`。

---

## 三、社区结论（Android 官方 + Wails 官方 + 响应式实践）

1. **Android 官方**（不同屏幕适配）：WebView 缩放基于页面 **target density**（默认 medium）；`window.devicePixelRatio` 查密度；`-webkit-device-pixel-ratio` 可按密度写样式。
2. **Wails 官方移动指南**：移动端必须 `viewport meta width=device-width, initial-scale=1, viewport-fit=cover`。
3. **通用响应式实践**：桌面固定 `px` 在手机上偏大/挤 → 用**媒体查询（断点）+ 相对单位**（`rem`/`vw`/`min()`/`clamp()`）。

---

## 四、通用处理思路（方法论）

### 1. 断点策略：默认=移动，`sm:`=桌面（本项目模式）
项目用 Tailwind CDN，断点模式是：**默认值=移动（<640px），`sm:` 前缀=桌面（≥640px）**。
- 改移动端 UI = 只调**默认值**，`sm:` 值**全保留** → 桌面零影响。
- 这是最安全的响应式改法：**改移动不动桌面**。

### 2. flex 收缩三件套
任何 `flex` 行里：
- 可收缩元素：`min-w-0`（防止撑破）
- 可点元素：`flex-shrink-0`（防止被挤没）
- 占满元素：`flex-1`

### 3. 空态/大间距：用 `py-14` 级，不用 `py-20/24`
空态页留白适度，图标 `w-12`、标题 `text-base`。

### 4. 滚动条：WebView 层 + CSS 层双保险
- Java：`setVerticalScrollBarEnabled(false)` 等
- CSS：`overflow-x:hidden` + 隐藏 `::-webkit-scrollbar`

### 5. 验证环境：必须上真机
- **容器/模拟器 dpr 低会掩盖 UI 问题**——容器正常不代表真机正常。
- 每次 UI 改动，**最终以真机为准**（截图 or 真机看），容器/桌面只作快速预览。
- 真机逻辑宽 ~393px 是移动端"最窄"的基准，按它设计。

---

## 五、一句话总结

**WebView UI 问题 90% 是"桌面尺寸塞进窄屏"——用"默认=移动 / sm:=桌面"的断点模式只改移动值、给 flex 可点元素加 `flex-shrink-0`、空态收紧、隐藏滚动条，并在真机（非容器）上验收。** 容器正常≠真机正常，这是最容易踩的错觉。
