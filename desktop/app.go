package main

import "fmt"

// App Wails 绑定结构（前端通过 window.go.main.App 访问）
type App struct {
	backendURL string
}

// NewApp 构造；port 为内嵌后端端口
func NewApp(port string) *App {
	return &App{backendURL: fmt.Sprintf("http://127.0.0.1:%s", port)}
}

// GetBackendURL 返回内嵌后端的地址（生产模式前端用它做 API baseURL）
func (a *App) GetBackendURL() string {
	return a.backendURL
}