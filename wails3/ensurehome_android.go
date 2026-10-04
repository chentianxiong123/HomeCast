//go:build android

package main

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ensureHome 安卓 app 进程无 HOME：指向私有 filesDir，homecast 数据目录 ~/.config/homecast 落盘
func ensureHome() {
	if os.Getenv("HOME") == "" {
		os.Setenv("HOME", application.Android.StoragePath())
	}
}
