//go:build android

package main

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	// 安卓入口注册：nativeInit 会 go main()；不注册则 main() 永不执行（WebView 只剩默认错误页）
	application.RegisterAndroidMain(main)
}

// ensureHome 安卓 app 进程 HOME 不可靠（可能为空/不可写）：无条件指向私有 filesDir，
// 保证 server.New() 的 ~/.config/homecast 落盘。必须在 nativeInit 之后（JNI bridge 可用）调用。
func ensureHome() {
	os.Setenv("HOME", application.Android.StoragePath())
}
