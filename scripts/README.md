# homecast 自动化测试

## 0. git pre-commit hook（模板结构防残缺）
clone 后手动装一次（core.hooksPath 是本地配置，不进版本控制）：
```bash
git config core.hooksPath scripts/
```
之后每次 `git commit` 自动跑：模板涉及变更（`go/web/templates/`）→ `go test ./web/ -run TestTemplates`。失败→阻止提交。应急跳过：`git commit --no-verify`。

防什么：Go `html/template` 只校验 `{{}}` 语法，不检查 HTML 标签闭合。残缺模板（div 少 `</div>`）能被 template 解析通过、被浏览器自动修复掩盖（10月14日发现 speaker_devices.html 残缺跑了 12 天才发现）。hook + [templates_test.go](../go/web/templates_test.go) 双重挡住。

## 1. Go 单元/集成测试（无外网，秒级）
```bash
cd go && go test ./...
```
覆盖：标题清洗规则（web）、SQLite 持久化（store）、收藏/播放列表服务层
（service）、API 端点+参数校验（api）、路由注册表完备性（api/router_test，
AST 静态断言）、CORS（server）、全页面渲染+收藏 toggle 行为（hx）。

历史战果：第一轮跑出 1 个真 bug——收藏"新的在前"顺序反了（SQLite 无
AUTOINCREMENT 时 rowid 复用 + save 全表重插顺序问题），已修复并锁定。

## 2. 后端全链路 E2E（真实 HTTP + mock B站 + 临时 DB，零外网，进 CI）
```bash
cd go && go test ./server/ -run TestE2E -v
```
覆盖：启动真实 `server.New()`（假 B站 + 临时 DB，环境变量 `HC_BILI_BASE`/`HC_DATA_DIR`
注入）→ 真实 HTTP 全链路：首页渲染（htmx 骨架）、收藏增删闭环（真 DB）、挂件 SSE
（widget/events 收 lyric-state）、歌词偏移持久化、设置页渲染、音箱 SSE 订阅。
**不依赖外网、不污染用户数据**（临时 DB + 假 B站）。CI `backend-test` 的 `go test ./...` 自动覆盖。

mock B站 server：`go run ./cmd/mockbili`（:39001，提供 search/view/playurl/静音音频流），
前端/壳级 E2E 可 `HC_BILI_BASE=http://127.0.0.1:39001` 接它，无外网跑。

## 3. Linux 壳级 E2E 冒烟（Xvfb 启动真实 wails3 app，进 CI）
```bash
bash scripts/shell-e2e.sh [app_binary]   # 默认 wails3/bin/hcexp
```
覆盖（"有一点就够"）：Xvfb 下启动真实 wails3 app（GTK4 + WebKit + 内嵌后端）→
断言最小链路：进程不崩 + 后端端口起来 + 首页渲染（player-dock/搜索框） + 后端 API 通
（fav/list code=0）。CI `shell-e2e` job 复用 build-linux 产物跑。

> 注：wails3 官方没有壳级 GUI E2E 框架（CLI 无 test/e2e 子命令，tests/ 是编译/性能），
> 壳级 E2E = 启动真实 app（Linux Xvfb / Windows runner / macOS Xcode / 移动模拟器）。
> Windows/macOS/移动壳级冒烟按对应 runner 补，逻辑同 shell-e2e.sh（启动+最小链路断言）。

## 4. 真实浏览器 E2E（Playwright + 真外网 B 站）
前置：起服务 `go run ./cmd/server`（或已 build 的二进制），
用 Playwright MCP 的 filename 参数加载 `e2e-smoke.js` 运行。

覆盖：首页加载 → 真实搜索（B站）→ 标题短名清洗 → 真实播放（音频流
actually playing）→ 队列渲染+标题展开 → 收藏增删闭环（真实 API 幂等）→
主题切换持久化 → 移动端视口无横向溢出 → 控制台零报错。

历史战果：抓到 dock 标题对旧数据（全名）无清洗兜底，已修复。

## 5. 框架可测性结论
- Go 层：handler 全依赖注入（NewMux(handlers...)、service.NewX(db)），
  httptest + t.TempDir() SQLite 无痛隔离，不碰真实数据；`server.New()` 支持
  `HC_BILI_BASE`（mock B站）/`HC_DATA_DIR`（临时 DB）环境变量注入，E2E 零外网。
- 前端层：htmx 页面 = 纯 HTTP GET（无状态优先），真浏览器可直接断言 DOM。
- 业务逻辑（标题清洗）已从模板闭包提为导出纯函数 ShortTitle，可单测。
- 壳级：wails3 无 GUI E2E 框架，用 Xvfb 启动真实 app 断言最小链路（进程/端口/页面/API）。
