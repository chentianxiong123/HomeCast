package token

import (
	"testing"
	"time"
)

func TestStoreGetLifecycle(t *testing.T) {
	ts := NewTokenStore()
	tk := ts.Store("http://x", map[string]string{"mime": "audio/mp4"})
	e, ok := ts.Get(tk)
	if !ok || e.URL != "http://x" || e.Metadata["mime"] != "audio/mp4" {
		t.Fatalf("Get after Store failed: %+v %v", e, ok)
	}
	if _, ok := ts.Get("nosuch"); ok {
		t.Fatal("Get unknown should be false")
	}
}

func TestStoreTTLExpires(t *testing.T) {
	ts := NewTokenStoreWith(100, 50*time.Millisecond)
	tk := ts.Store("http://x", nil)
	if _, ok := ts.Get(tk); !ok {
		t.Fatal("should be valid before TTL")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := ts.Get(tk); ok {
		t.Fatal("expired token should be gone")
	}
	// 惰性删除：Get 已清掉
	if _, ok := ts.m[tk]; ok {
		t.Fatal("expired entry should be deleted from map")
	}
}

func TestStoreCapacityEviction(t *testing.T) {
	ts := NewTokenStoreWith(3, time.Hour)
	ta := ts.Store("a", nil)
	_ = ts.Store("b", nil)
	_ = ts.Store("c", nil)
	td := ts.Store("d", nil) // 超上限 → purge 过期（无）→ dropOldest（最早过期的一项）
	if len(ts.m) != 3 {
		t.Fatalf("capacity should be 3, have %d", len(ts.m))
	}
	if _, ok := ts.Get(ta); ok {
		t.Fatal("oldest (a) should be evicted")
	}
	if _, ok := ts.Get(td); !ok {
		t.Fatal("newest (d) should be present")
	}
}