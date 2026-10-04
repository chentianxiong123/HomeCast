//go:build !android

package main

import (
	"os"
	"path/filepath"
)

// ensureHome 桌面等平台：HOME 本就存在，无需处理
func ensureHome() {}

// hcDataPath 应用数据目录（日志/配置落盘处）
func hcDataPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "homecast")
}
