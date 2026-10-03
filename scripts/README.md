# homecast 自动化测试

## 1. Go 单元/集成测试（无外网，秒级）
```bash
cd go && go test ./...
```
覆盖：标题清洗规则（web）、SQLite 持久化（store）、收藏/播放列表服务层
（service）、API 端点+参数校验（api）、路由注册表完备性（api/router_test，
AST 静态断言）、CORS（server）、全页面渲染+收藏 toggle 行为（hx）。

历史战果：第一轮跑出 1 个真 bug——收藏"新的在前"顺序反了（SQLite 无
AUTOINCREMENT 时 rowid 复用 + save 全表重插顺序问题），已修复并锁定。

## 2. 真实浏览器 E2E（Playwright + 真外网 B 站）
前置：起服务 `go run ./cmd/server`（或已 build 的二进制），
用 Playwright MCP 的 filename 参数加载 `e2e-smoke.js` 运行。

覆盖：首页加载 → 真实搜索（B站）→ 标题短名清洗 → 真实播放（音频流
actually playing）→ 队列渲染+标题展开 → 收藏增删闭环（真实 API 幂等）→
主题切换持久化 → 移动端视口无横向溢出 → 控制台零报错。

历史战果：抓到 dock 标题对旧数据（全名）无清洗兜底，已修复。

## 3. 框架可测性结论
- Go 层：handler 全依赖注入（NewMux(handlers...)、service.NewX(db)），
  httptest + t.TempDir() SQLite 无痛隔离，不碰真实数据。
- 前端层：htmx 页面 = 纯 HTTP GET（无状态优先），真浏览器可直接断言 DOM。
- 业务逻辑（标题清洗）已从模板闭包提为导出纯函数 ShortTitle，可单测。
