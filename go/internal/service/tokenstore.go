package service

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// TokenEntry 代理 token 数据（对齐 Python token_store）
type TokenEntry struct {
	URL      string
	Metadata map[string]string // content_type / original_url / referer / hardware / mime_type
}

// TokenStore 内存 token 存储（音箱/DLNA 播放用代理 URL）
type TokenStore struct {
	mu sync.Mutex
	m  map[string]TokenEntry
}

// NewTokenStore 构造
func NewTokenStore() *TokenStore {
	return &TokenStore{m: map[string]TokenEntry{}}
}

// Store 存 URL → 返回随机 token
func (t *TokenStore) Store(url string, metadata map[string]string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	t.mu.Lock()
	t.m[token] = TokenEntry{URL: url, Metadata: metadata}
	t.mu.Unlock()
	return token
}

// Get 取 token 数据
func (t *TokenStore) Get(token string) (TokenEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.m[token]
	return e, ok
}