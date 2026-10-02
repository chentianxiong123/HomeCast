// homecast Go 后端入口（复用 internal/server 全部端点）
package main

import (
	"log"

	"homecast/server"
)

func main() {
	if err := server.ListenAndServe(""); err != nil {
		log.Fatal(err)
	}
}