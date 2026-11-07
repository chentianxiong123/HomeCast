//go:build ios

package main

import (
	"os"
	"path/filepath"
)

// ensureHome iOS：数据目录用沙盒可写区（Application Support / Documents）
func ensureHome() {}

// hcDataPath 应用数据目录（日志/配置落盘处）——iOS 沙盒内
func hcDataPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "homecast")
}