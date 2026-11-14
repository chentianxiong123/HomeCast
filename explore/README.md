# explore/ —— 探索区（历史只读，不维护）

**主次定位**：正主是 `go/`（业务核心）+ `wails3/`（三端壳），本目录只收"历史版本 / 探索原型"，
不参与任何构建、测试、维护。

| 子目录 | 是什么 | 价值 |
|--------|--------|------|
| `legacy/python-backend/` | **老 HomeCast 后端**（Python/FastAPI）：B站音乐、DLNA、音箱、嗅探 | 历史参考，不跑 |
| `legacy/vue-frontend/` | **老 HomeCast 前端**（Vue 3）：htmx 重构前的旧界面 | 历史参考，不跑 |
| `legacy/test_fav.json` | 老收藏接口的测试数据 | 历史 fixture |
| `android/` | **手搓安卓 WebView 壳**（Java）：wails3 安卓壳早期实现 | 兜底参考（可读） |
| `wasm-mp/` | **微信小程序 WASM 同进程方案**（已放弃）：Go 后端编译 wasm 跑在小程序进程 | 探索留档，不跑 |
| `mp-html/` | **微信小程序 mp-html 渲染方案**（已废弃）：后端吐 HTML + mp-html 渲 | 探索留档，不跑 |
| `speaker-latency-mock/` | **音箱拉流延迟黑盒测试台**（独立 mock）：真实音箱 token 下发播放，定位"点播放到出声三四秒"根因 = fMP4 容器，软转码 mp3 秒开 | 根因已定，可跑（独立 :28777） |

## 血缘

- `legacy/python-backend + legacy/vue-frontend` = HomeCast 的 **python/Vue 时代**（git 早期提交）
- `android/` = 安卓壳的手搓原型（后被 wails3 统一壳取代）
- 参考项目不放在这，统一在 `/mnt/shared/ref-project/`（bili-music / musicxx / xiaomusic / MiAir）

## 纪律

- 不要往这里写"新探索"——新探索去独立目录（如 `explore/` 下的新名字或单独仓库）
- 不要引用这里的代码到 `go/` `wails3/`（复制代码后即归档）
