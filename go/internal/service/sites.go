package service

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Site 嗅探站点（对齐 Python Site）
type Site struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	SiteType string `json:"site_type"`
}

// SitesData 文件结构（sites + preset）
type SitesData struct {
	Sites  []Site `json:"sites"`
	Preset []Site `json:"preset"`
}

// SitesService 站点管理 + 详情页集缓存（对齐 api/sites.py）
type SitesService struct {
	mu          sync.Mutex
	sitesPath   string
	cachePath   string
	defaultData SitesData
}

// NewSitesService 构造（~/.config/homecast/sites.json）
func NewSitesService(dataDir string) *SitesService {
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".config", "homecast")
	}
	return &SitesService{
		sitesPath: filepath.Join(dataDir, "sites.json"),
		cachePath: filepath.Join(dataDir, "episodes_cache.json"),
		defaultData: SitesData{
			Sites: []Site{},
			Preset: []Site{
				{Name: "MoMoVOD-真情", URL: "https://momovod.app/vod/466165.html", SiteType: "detail"},
			},
		},
	}
}

func (s *SitesService) load() SitesData {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.sitesPath)
	if err != nil {
		return s.defaultData
	}
	var d SitesData
	if json.Unmarshal(data, &d) != nil {
		return s.defaultData
	}
	return d
}

func (s *SitesService) save(d SitesData) {
	_ = os.MkdirAll(filepath.Dir(s.sitesPath), 0o755)
	if b, err := json.MarshalIndent(d, "", "  "); err == nil {
		_ = os.WriteFile(s.sitesPath, b, 0o644)
	}
}

// List 站点列表
func (s *SitesService) List() SitesData {
	return s.load()
}

// Add 添加（URL 去重）
func (s *SitesService) Add(site Site) error {
	d := s.load()
	for _, x := range d.Sites {
		if x.URL == site.URL {
			return errors.New("网站已存在")
		}
	}
	d.Sites = append(d.Sites, site)
	s.save(d)
	return nil
}

// Remove 按 URL 移除
func (s *SitesService) Remove(url string) {
	d := s.load()
	out := d.Sites[:0]
	for _, x := range d.Sites {
		if x.URL != url {
			out = append(out, x)
		}
	}
	d.Sites = out
	s.save(d)
}

// ── 详情页集缓存（投屏嗅探缓存） ──

type episodesCache struct {
	DetailURL     string      `json:"detail_url"`
	DetailURLHash string      `json:"detail_url_hash"`
	Title         string      `json:"title"`
	EpisodesList  []Episode   `json:"episodes_list"`
	CachedAt      float64     `json:"cached_at"`
	_             interface{} `json:"-"`
}

// CacheEpisodes 缓存详情页集数列表
func (s *SitesService) CacheEpisodes(detailURL, title string, list []Episode) {
	cache := s.loadCache()
	hash := urlHash(detailURL)
	cache[hash] = map[string]any{
		"detail_url":      detailURL,
		"detail_url_hash": hash,
		"title":           title,
		"episodes_list":   list,
		"cached_at":       float64(time.Now().UnixNano()) / 1e9,
	}
	b, _ := json.Marshal(cache)
	_ = os.MkdirAll(filepath.Dir(s.cachePath), 0o755)
	_ = os.WriteFile(s.cachePath, b, 0o644)
}

// GetCachedEpisodes 查询缓存
func (s *SitesService) GetCachedEpisodes(detailURL string) (map[string]any, bool) {
	cache := s.loadCache()
	hash := urlHash(detailURL)
	v, ok := cache[hash].(map[string]any)
	return v, ok
}

func (s *SitesService) loadCache() map[string]any {
	data, err := os.ReadFile(s.cachePath)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return map[string]any{}
	}
	return m
}

// urlHash MD5
func urlHash(url string) string {
	sum := md5.Sum([]byte(url))
	return hex.EncodeToString(sum[:])
}