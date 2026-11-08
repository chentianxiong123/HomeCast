//go:build openharmony

// homecast Go 后端鸿蒙入口（c-shared .so，Native 加载）
// 导出 StartServer 供 ArkTS 壳调用（dlopen .so → 调 StartServer → 后端起 :28976）
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"log"
	"os"
	"path/filepath"

	"homecast/server"
)

// StartServer 启动 HomeCast HTTP 后端。dataDir 必须是正式 App 的沙盒 filesDir；
// 显式设置 HOME / HC_DATA_DIR，不依赖 hdc shell 的环境变量。
// 在独立 goroutine 运行（后端是长驻服务，不占调用线程）。
//
//export StartServer
func StartServer(dataDir *C.char) {
	if dir := C.GoString(dataDir); dir != "" {
		_ = os.Setenv("HOME", dir)
		_ = os.Setenv("HC_DATA_DIR", dir)
		_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
		_ = os.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
		_ = os.Setenv("TMPDIR", filepath.Join(dir, "tmp"))
		_ = os.MkdirAll(filepath.Join(dir, "tmp"), 0o700)
	}
	go func() {
		// 不吞掉启动错误：让 hilog 能看到真实失败原因。
		if err := server.ListenAndServe("127.0.0.1:" + server.Port); err != nil {
			log.Printf("HomeCast backend stopped: %v", err)
		}
	}()
}

func main() {} // c-shared 下 main 不执行（由导出函数驱动）
