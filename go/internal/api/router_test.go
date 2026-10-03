package api

import (
	"strings"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// TestRouterRegisteredPaths 路由注册表完备性：从源码静态断言
// 每个 mux.HandleFunc("METHOD /path") 的模式：方法合法 + 路径非空 + 不与表外重复
// （框架可测性验证：路由表是"代码即契约"，不联网也能锁死）
func TestRouterRegisteredPaths(t *testing.T) {
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("读 router.go: %v", err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "router.go", src, 0)
	if err != nil {
		t.Fatalf("解析 router.go: %v", err)
	}

	type route struct{ method, pattern string }
	var routes []route
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// 注意：Go 1.22 路由写法 mux.HandleFunc("METHOD /path", handler)
		// pattern 是 args[0] 字符串（"GET /api/v1/x"），handler 是 args[1] 表达式
		if len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		// 方法 3 字（GET/DELETE）或 4 字（POST），不能按固定下标切 → Fields 拆
		fs := strings.Fields(strings.Trim(lit.Value, `"`))
		if len(fs) != 2 || len(fs[1]) < 3 {
			return true
		}
		routes = append(routes, route{fs[0], fs[1]})
		return true
	})

	if len(routes) < 30 {
		t.Fatalf("注册端点过少: %d, want >= 30", len(routes))
	}
	seen := map[string]bool{}
	validMethods := map[string]bool{"GET": true, "POST": true, "DELETE": true, "OPTIONS": true}
	for _, r := range routes {
		if !validMethods[r.method] {
			t.Errorf("非法方法 %s for %s", r.method, r.pattern)
		}
		if len(r.pattern) < 3 {
			t.Errorf("非法路径模式 %s", r.pattern)
		}
		key := r.method + " " + r.pattern
		if seen[key] {
			t.Errorf("重复路由: %s", key)
		}
		seen[key] = true
	}

	// 关键端点必须在表内
	for _, want := range []string{
		"/api/v1/music/search", "/api/v1/music/stream/{bvid}",
		"/api/v1/music/lyric", "/api/v1/fav/list", "/api/v1/fav/add",
		"/api/v1/fav/{bvid}", "/api/v1/fav/clear",
		"/api/v1/playlist", "/api/v1/playlist/add/{bvid}",
		"/api/v1/cast/devices", "/api/v1/widget/state",
	} {
		found := false
		for k := range seen {
			if len(k) > len(want) && k[len(k)-len(want):] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("关键端点缺失: %s", want)
		}
	}
}

// TestRoutesStableAndMatch 实际起最小 fav mux：未注册路径 404、已注册路径方法匹配
func TestRoutesStableAndMatch(t *testing.T) {
	_, mux := newTestFavHandler(t)

	// 未知路径 → 404
	if w := doReq(t, mux, "GET", "/api/v1/nonexistent", nil); w.Code != 404 {
		t.Errorf("未知路径 code=%d, want 404", w.Code)
	}
	// 未注册路径（前缀像但不存在）→ 404
	if w := doReq(t, mux, "GET", "/api/v1/fav/", nil); w.Code == 200 {
		t.Errorf("/api/v1/fav/ 不应 200")
	}
}