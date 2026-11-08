# mymusic 微信小程序壳（Go 后端）

**两个版本**：

| 版本 | 壳 | 观感 | 重写量 | 状态 |
|---|---|---|---|---|
| **A. web-view（默认）** | 一个 `<web-view>` 组件 | 网页 | **零** | ✅ 完美运行 |
| **B. 无 web-view（mp-html）** | mp-html 渲原生 | 原生 | 要重写导航/播放器/歌词 | ⏳ 攻坚 |

## A. web-view 版本（当前默认，零重写）

壳 = 一个 `<web-view src="http://127.0.0.1:28976/">`，加载 HomeCast **H5 整站**（go:embed 前端）。
**导航、播放器控制条、歌词、投屏全部原样运行，零重写。**

```
wails3/build/wechat/
├── app.js / app.json / app.wxss        # 壳入口
├── project.config.json                 # appid + urlCheck:false
├── sitemap.json
├── pages/index/
│   ├── index.wxml                      # <web-view>（就一行）
│   ├── index.js                        # url = BASE + '/'
│   └── index.json
└── utils/config.js                     # BASE 后端地址
```

## B. 无 web-view 版本（mp-html 原生，攻坚中）

用 mp-html 把后端 HTML 渲成**原生节点**（内容复用），但**交互必须重写**：
- 后端新增 `/hx/search-mp`（纯展示片段，卡片用 `<a href>` 包，`linktap` 事件触发播放）
- mp-html 关键点：事件名是 **`linktap`**（不是 `link`），默认行为是**复制链接**，需 `copy-link="false"` 关掉
- 待补齐：导航、底部播放器控制条、歌词面板（mp-html 不会自动有）

> mp-html 组件包在 `components/mp-html/`（零 npm，git 拷贝 `dist/mp-weixin`）。
> 播放已验证：`InnerAudioContext` 播 `/api/v1/music/stream/{bvid}` 能出声。

## 架构（共用）

- **业务逻辑一份住 Go 后端**（`go/cmd/server`），前后端走真实 HTTP `http://127.0.0.1:28976`
- 后端 `/hx/search-mp` 是 mp-html 版专用纯展示片段（web-view 版用整站 `/`，不需要它）
- 零 npm：壳无 node_modules；mp-html 组件直接拷贝；miniprogram-ci 仅预览用

## 前置（一次性）

1. Node + `npm install -g miniprogram-ci`（装到独立目录 `/home/a1/hc-mp-ci`，免全局权限）
2. 小程序 AppID：填 `project.config.json`
3. 代码上传密钥（公众平台→开发→开发管理→开发设置→小程序代码上传→生成）

## 起后端（局域网可达）

```bash
scripts/mp-backend.sh        # mockbili :39001 + server :28976
# 模拟器用 127.0.0.1（后端本机）；真机预览改 config.js 为局域网 IP
```

## 预览/调试

- **模拟器**（IDE）：微信开发者工具导入 `wails3/build/wechat` → 勾「不校验合法域名」→ 编译
- **真机扫码**（零 IDE）：`node scripts/mp-preview.js <私钥>` → 手机微信打开调试

## mp-html 边界（版本 B 用）

- ✅ 渲：标签/样式/图片/表格/音视频
- ❌ 不执行：`<script>`/`onclick`/`hx-post`；交互（播放/导航/歌词）需小程序逻辑层重接
- 事件名 **`linktap`**；默认复制链接，用 `copy-link="false"` 关闭

## 上线前（暂不做）

- `urlCheck:true` + 配置合法域名 / 业务域名（https+备案）
- `miniprogram-ci upload` 上传
