# mymusic 微信小程序壳（Go 后端 + mp-html 渲原生）

**零 npm 架构**：壳内**没有 node_modules、没有构建链、没有依赖安装**——mp-html 组件是直接拷贝进来的（`components/mp-html/`），页面是手写 WXML/JS。唯一用 npm 的是**预览工具** `miniprogram-ci`（微信官方 CLI，只在预览时用，不是壳的依赖）。

```
wails3/build/wechat/
├── app.js / app.json / app.wxss      # 壳入口（业务全在后端，壳空）
├── project.config.json               # appid + urlCheck:false（开发期不校验域名）
├── sitemap.json
├── components/mp-html/               # mp-html 组件包（git 拷 dist/mp-weixin，零 npm）
├── pages/index/                      # 搜索框 + mp-html 渲染 + 播放
└── utils/config.js                   # BASE = 后端地址（真机要能访问，填内网 IP）
```

## 架构（沿用项目原则）

- **业务逻辑一份住 Go 后端**，壳只做：搜索 → 后端吐 HTML → mp-html 渲原生 → 点击播放
- **前后端走真实 HTTP**（`http://<内网IP>:28976`）
- **内容复用 go:embed 模板**：后端新增 `/hx/search-mp`（纯展示片段，无 onclick/hx-post），mp-html 渲成小程序原生节点
- **交互重接**：mp-html 不执行 JS，但支持 `<a>` 的 `link` 事件——后端片段用 `<a href="hc-play://play?bvid=...">` 包卡片，小程序 `bindlink` 拿 href 解析 bvid → 播放

## 前置

1. **Node**（仅预览工具用）+ `npm install -g miniprogram-ci`
2. **小程序 AppID**：mp.weixin.qq.com 免费注册（个人主体）→ 填进 `project.config.json`
3. **代码上传密钥**：微信公众平台 → 开发管理 → 开发设置 →「小程序代码上传」→ 下载私钥

## 起后端（局域网可达）

```bash
scripts/mp-backend.sh
# 输出内网 IP，把它填进 wails3/build/wechat/utils/config.js 的 BASE
# 手机先浏览器访问 http://<内网IP>:28976 确认能通
```

## 真机预览（零 IDE）

```bash
node scripts/mp-preview.js <代码上传私钥路径>
# 终端显示二维码 → 手机微信扫码 → 真机打开 mymusic
```

**域名校验**：`project.config.json` 已设 `urlCheck:false`；若真机仍拦，手机微信打开小程序后 **右上角菜单 → 打开调试**（官方途径，免 IDE）即可访问 http://内网IP。

## 自测分层

| 层 | 方式 | 需 IDE/真机 |
|---|---|---|
| 后端逻辑 | `go test ./...` + CI E2E（已有） | 否 |
| 搜索片段 | `curl 'http://127.0.0.1:28976/hx/search-mp?kw=test'` | 否 |
| 壳逻辑层 | `node --check pages/index/index.js`（语法）+ 手动走查 | 否 |
| 真机体验 | `node scripts/mp-preview.js` 扫码 | 仅真机 |

## mp-html 边界（重要）

- ✅ 渲：标签/样式/图片/表格/音频视频
- ❌ 不执行：`<script>`、`onclick`、`hx-post` —— 所以后端片段是**纯展示**（`/hx/search-mp`），交互全部由壳逻辑层重接（`bindlink`/`bindtap`）
- 播放走 `InnerAudioContext` 播 `/api/v1/music/stream/{bvid}`（后端代理流，mockbili 返回真实可播 WAV）

## 上线前（暂不做）

- `urlCheck:true` + 微信后台配置合法域名（**https + 备案域名**）
- `miniprogram-ci upload` 上传 → 体验版 → 正式版
