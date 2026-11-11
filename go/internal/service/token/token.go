// Package token 代理 token 域：临时 URL 化（音箱/DLNA 播放代理用），内存存储。
// 全链路：使用方 internal/api/proxy.go · speaker 域 · cast 域；表：无
package token

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// TokenEntry 代理 token 数据（对齐 Python token_store）
type TokenEntry struct {
	URL      string
	Metadata map[string]string // content_type / original_url / referer / hardware / mime_type
	expires  time.Time         // 过期时间（短命 token：到期即失效删除）
}

// TokenStore 内存 token 存储（音箱/DLNA 播放用代理 URL）
// 过期策略：Store 时定 TTL（默认 1h），Get 惰性删过期项，超上限顺带整体清扫。
type TokenStore struct {
	mu      sync.Mutex
	m       map[string]TokenEntry
	max     int           // 容量上限（0=不限）
	ttl     time.Duration // 过期时长
}

// NewTokenStore 构造
func NewTokenStore() *TokenStore {
	return NewTokenStoreWith(4096, time.Hour)
}

// NewTokenStoreWith 构造（显式容量/过期，测试注入短 TTL 用）
func NewTokenStoreWith(max int, ttl time.Duration) *TokenStore {
	return &TokenStore{m: map[string]TokenEntry{}, max: max, ttl: ttl}
}

// Store 存 URL → 返回随机 token（带 TTL；超上限时清扫过期项，防只增不删内存泄漏）
func (t *TokenStore) Store(url string, metadata map[string]string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	now := time.Now()
	t.mu.Lock()
	t.m[token] = TokenEntry{URL: url, Metadata: metadata, expires: now.Add(t.ttl)}
	// 容量保护：只增不删会内存泄漏（review 低能 #1）。满了先清过期项，仍满则删
	// 最旧（按 expires 排，等同最早写入）。
	if t.max > 0 && len(t.m) > t.max {
		t.purgeLocked(now)
		if len(t.m) > t.max {
			dropOldestLocked(t.m)
		}
	}
	t.mu.Unlock()
	return token
}

// Get 取 token 数据（惰性：命中但过期 → 删除返回 false）
func (t *TokenStore) Get(token string) (TokenEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[token]
	if !ok {
		return TokenEntry{}, false
	}
	if !e.expires.IsZero() && time.Now().After(e.expires) {
		delete(t.m, token)
		return TokenEntry{}, false
	}
	return e, true
}

// purgeLocked 清掉全部过期项（调用方持锁）
func (t *TokenStore) purgeLocked(now time.Time) {
	for k, e := range t.m {
		if !e.expires.IsZero() && now.After(e.expires) {
			delete(t.m, k)
		}
	}
}

// dropOldestLocked 删除最旧（最早过期）的一项（调用方持锁；map 无序遍历安全）
func dropOldestLocked(m map[string]TokenEntry) {
	var (
		oldK  string
		oldT  time.Time
		first = true
	)
	for k, e := range m {
		if first || e.expires.Before(oldT) {
			oldK, oldT, first = k, e.expires, false
		}
	}
	if oldK != "" {
		delete(m, oldK)
	}
}