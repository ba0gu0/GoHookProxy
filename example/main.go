package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/ba0gu0/GoHookProxy/config"
	"github.com/ba0gu0/GoHookProxy/hook"
	"github.com/ba0gu0/GoHookProxy/proxy"
)

func main() {
	// 使用默认配置
	cfg := config.DefaultConfig()

	// 根据需要修改配置
	cfg.Enable = true
	cfg.ProxyType = config.SOCKS5
	cfg.ProxyIP = "127.0.0.1"
	cfg.ProxyPort = 1080
	cfg.SOCKSConfig.User = "user"
	cfg.SOCKSConfig.Pass = "password"

	// 启用指标收集
	cfg.MetricsEnable = true

	// 设置连接池参数
	cfg.IdleTimeout = 90 * time.Second

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatal("Invalid config:", err)
	}

	// 创建代理管理器
	pm, err := proxy.New(cfg)
	if err != nil {
		log.Fatal("Failed to create proxy manager:", err)
	}

	patcher := gomonkey.NewPatches()
	// 创建并启用hook
	h := hook.New(pm, patcher)
	if err := h.Enable(); err != nil {
		log.Fatal("Failed to enable hook:", err)
	}
	defer h.Disable()

	// Start metrics reporting
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			metrics := pm.GetMetrics()
			log.Printf("Active connections: %d", metrics.ActiveConnections)
			log.Printf("Total connections: %d", metrics.TotalConnections)
			log.Printf("Failed connections: %d", metrics.FailedConnections)
			log.Printf("Average duration: %v",
				metrics.ConnectionDuration/time.Duration(metrics.TotalConnections))
		}
	}()

	// 运行测试
	if err := runTests(); err != nil {
		log.Printf("Tests failed: %v", err)
		os.Exit(1)
	}
}

func runTests() error {
	tests := []struct {
		name string
		url  string
	}{
		{"HTTP Test", "http://example.com"},
		{"HTTPS Test", "https://api.github.com"},
	}

	for _, test := range tests {
		log.Printf("Running %s...", test.name)
		if err := testRequest(test.url); err != nil {
			return fmt.Errorf("%s failed: %w", test.name, err)
		}
		log.Printf("%s succeeded", test.name)
	}

	return nil
}

func testRequest(url string) error {
	// 创建带超时的客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 发送请求
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// 打印响应摘要
	fmt.Printf("Response from %s (%d bytes):\n%s\n",
		url,
		len(body),
		string(body[:min(len(body), 100)]), // 只显示前100个字符
	)

	return nil
}

// min returns the smaller of x or y
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
