# 微信小程序壳：web-view 与 mp-html 双版本探索

> 2026-11-09 · HomeCast（mymusic）· 微信小程序壳路线探索
> 核心：业务逻辑一份住 Go 后端，小程序壳只做"显示层"。探索 mp-html 能否胜任"原生渲染 + 交互"，最终得出 web-view 与 mp-html 两条路。

---

## ① 目标与动机

把 HomeCast（音乐搜索/播放/歌词/投屏）以**最薄壳**跑进微信小程序，且**不重写前端**：
- 业务逻辑一份住 Go 后端（`go/cmd/server`，端口 28976）
- 前端复用 go:embed 的 htmx 整站
- 小程序壳只负责"显示"，零 npm、零构建

想验证两条显示路线：
- **web-view**：壳 = 一个 `<web-view>` 加载 H5 整站（完整但网页观感）
- **mp-html**：把后端吐的 HTML 渲成小程序**原生节点**（原生观感，但 mp-html 不执行 JS）

## ② 资源拓扑

### mp-html 库（jin-yufeng/mp-html，微信/QQ/百度/支付宝/头条/uni-app 多平台包）

```
components/mp-html/            # 从 git 拷贝 dist/mp-weixin（零 npm，源码方式）
├── index.js                   # 根组件：setContent(html) 解析 → nodes → 渲染
├── index.wxml
├── node/node.js               # 每个 HTML 节点一个自定义组件，递归自引用
├── parser/                    # html → 节点树解析器
└── ...
```

**关键机制**：
- `content` 属性收 HTML 字符串 → parser 解析成节点树 → 递归渲染成小程序原生节点
- 事件：节点 `<a>` 点击触发 **`linktap`**（注意**不是 `link`**！）；图片 `imgtap`
- 默认行为：`href` 含 `://` → **`wx.setClipboardData`（复制链接）**；`copy-link` 属性控制（默认 `true`）
- **不执行** `<script>`/`onclick`/`hx-post`——安全边界 + 小程序无 DOM

### HomeCast web 前端（go:embed，htmx）

```
go/web/assets/   player.js（audio 播放+控制条）、lyric.js（SSE 歌词）、subtitle.js、settings.js、style.css
go/web/templates/  shell.html（导航+播放条骨架）、content_search.html、results.html、fav_list.html、results_mp.html（mp-html 专用纯展示片段）
go/hx/mp.go       /hx/search-mp 端点（纯展示 HTML，卡片用 <a href> 包）
```

**前端交互构成**（"JS 少不少"的关键）：
- 搜索：htmx `hx-get` → 后端返回 HTML 片段 → `innerHTML` 插入
- 播放：`player.js` 的 `<audio>` + 控制条（进度/播放暂停/上一首下一首）
- 歌词：`lyric.js` 走 SSE（`/api/v1/lyric/stream`）实时刷新
- 收藏/投屏/控制条共享：htmx + 少量 JS

### 小程序壳（wails3/build/wechat/）

```
project.config.json     appid + urlCheck:false
app.json / app.js / app.wxss
pages/index/            index.wxml（web-view 或 mp-html 切换）+ index.js
utils/config.js         BASE（后端地址）
components/mp-html/     B 版本用
```

## ③ 工具链清单

| 工具 | 版本 | 用途 | 装法 |
|---|---|---|---|
| Go | 1.26.5 | 后端 | 官网 tar.gz → `~/.local/go`（Mac） |
| mockbili | go run | 假 B站（E2E 数据） | `go/cmd/mock/bili` |
| server | go run | 后端 28976 | `go/cmd/server` |
| miniprogram-ci | 2.1.31 | 真机扫码预览 | `/home/a1/hc-mp-ci`（免全局） |
| 微信开发者工具 | 2.02.2609250 | 模拟器调试 | Mac `/Applications/wechatwebdevtools.app` |
| mp-html | git 拷贝 | 原生渲染 | `components/mp-html` |

**起后端**：`scripts/mp-backend.sh`（mockbili :39001 + server :28976）
**后端地址**：模拟器用 `127.0.0.1:28976`（后端本机）；真机用局域网 IP。

## ④ 成功路径

### A. web-view（当前默认，已验证完美）

```
wails3/build/wechat/pages/index/index.wxml:
  <web-view src="{{url}}" />          # 就一行

index.js:
  Page({ data: { url: BASE + '/' } })
```

微信开发者工具导入 `wails3/build/wechat` → 勾「不校验合法域名」→ 编译 → **HomeCast 整站完整显示（导航/播放器/歌词全有，与网页版一模一样）**。

### B. mp-html（原生渲染，播放已通，UI 残缺）

```
后端 /hx/search-mp?kw=  → 纯展示 HTML 片段（卡片 <a href="https://play.homecast/?bvid=..."> 包）
壳 index.wxml:  <mp-html content="{{html}}" bindlinktap="onLink" copy-link="{{false}}" />
index.js onLink: 解析 bvid → InnerAudioContext 播 /api/v1/music/stream/{bvid}  → 出声 ✅
```

搜索卡片**能显示**，点击**能出声**，但**无导航/播放器控制条/歌词**（mp-html 只渲静态 HTML，JS 动态 UI 不出现）。

## ⑤ 踩坑记录（现象 → 根因 → 解法）

1. **搜索一直"加载中"** → `config.js` 的 `BASE` 是 Linux 占位 `192.168.1.100`，Mac 模拟器访问不到 → 改成 `127.0.0.1:28976`（Mac BSD sed 用 `sed -i ''`，不是 GNU `-i`）。
2. **点卡片"复制链接"（非播放）** → mp-html 源码 `node/node.js` 的 `linkTap`：事件名是 **`linktap`**（非 `link`），且 `href` 含 `://` 默认 `wx.setClipboardData` 复制 → 绑 `bindlinktap` + `copy-link="{{false}}"`。
3. **`hc-play://` 自定义协议不被 mp-html 当链接** → 改 `href="https://play.homecast/?bvid=..."`（http(s) 形式触发 linktap）。
4. **服务端模板改了不生效** → `results_mp.html` 是 go:embed 内嵌，必须**重启 server** 才生效。
5. **kill server 杀不干净** → `go run` 的编译子进程仍监听 28976 → `kill -9 $(lsof -tiTCP:28976 -sTCP:LISTEN)`。
6. **miniprogram-ci 需服务端口** → CLI 调用报 "IDE service port disabled" → 需工具「设置→安全设置→开启服务端口」。
7. **Mac 副本落后且脏** → `git fetch` + `git reset --hard origin/master`（本地改动先 `git diff > patch` 备份）。

## ⑥ 验证成果

- **web-view**：模拟器里 HomeCast 整站**一模一样**（导航/播放器/歌词全有），播放能听到声音 ✅
- **mp-html**：搜索卡片渲出真实 B站数据 ✅；点卡片 InnerAudioContext 播放**能出声** ✅；但**无导航/播放器控制条/歌词**（残缺）❌
- 后端：`GET /hx/search-mp?kw=` HTTP 200；`GET /api/v1/music/stream/{bvid}` 返回真实可播 WAV；mockbili 返回 5 条 E2E 测试数据
- go test：`hx`、`web` 全过

## ⑦ 决策记录

| 方案 | 结论 | 原因 |
|---|---|---|
| **web-view（A）** | ✅ **默认，完美** | 零重写、JS 全跑、H5 整站完整、已验证 |
| mp-html（B） | ⏳ 备选/攻坚 | 只渲静态 HTML，导航/播放器/歌词要重写 |
| Towxml / rich-text | ❌ 不通 | 同 mp-html：不执行 JS |
| kbone | ❌ 不用 | 要 webpack 构建链（违背零 npm）+ 面向 vue/react，htmx 的 DOM/fetch 在模拟 DOM 里风险大 |
| Taro / uni-app | ❌ 不用 | 构建链 + 框架重写（用户明确拒绝 uni-app） |
| 正式账号/测试号 | 测试号不能 miniprogram-ci | 正式账号注册即实名；邮箱互斥（2036680567@qq.com 已绑微信） |

**物理事实**：小程序无 DOM，mp-html/Towxml/rich-text 不执行 JS 是设计边界，非 bug；能跑 JS 的（kbone/Taro）都要构建链。**web-view 是"JS 全跑 + 零重写 + 无构建"的唯一解。**

## ⑧ 未解之谜 / 下一步

**【实测结论 2026-11-09】mp-html 高频刷新思路成立 ✅（播放条+歌词都过）**
- 实验：后端 `/hx/player-bar`（进度随时间变）+ `/hx/lyric-panel`（12 行歌词当前行滚动高亮），壳 mp-html 每 **500ms** setData 刷新 → 模拟器实测**播放条 + 12 行歌词同时刷，均流畅不卡**。
- 结论：**mp-html + 后端吐动态 HTML + 高频 setData 刷新 = 完整可行**，是 web-view 与纯重写之间的**第三条路**（原生感 + 轻重写，逻辑全在 Go 后端）。
- 用户不服判断成立：前端 JS 少、服务端多，把"动态 UI"也下放后端吐 HTML，mp-html 就能覆盖。

1. **【下一步】搭 B 完整版**：把实验换成真实播放器状态——后端用真实播放状态渲染 `/hx/player-bar` + `/hx/lyric-panel`（接 widget/player 服务，非模拟），壳多块 mp-html（搜索 + 播放条 + 歌词）+ 500ms 轮询 + linktap 分发 action（播放/暂停/上一首/下一首 → 调真实 API）。
2. 真机扫码预览（需正式账号+密钥，邮箱卡着）
3. 多端扩展（支付宝/抖音/QQ，抄 web-view 壳）
4. Mac 后端起停脚本（一键 `scripts/mp-backend.sh` 的 Mac 版）
2. 真机扫码预览（需正式账号+密钥，邮箱卡着）
3. 多端扩展（支付宝/抖音/QQ，抄 web-view 壳）
4. Mac 后端起停脚本（一键 `scripts/mp-backend.sh` 的 Mac 版）
