package test

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	C "github.com/ba0gu0/GoHookProxy/config"
	"github.com/ba0gu0/GoHookProxy/hook"
	"github.com/ba0gu0/GoHookProxy/proxy"
)

func TestHookWithProxies(t *testing.T) {

	// 测试地址
	testURLs := []struct {
		url        string
		shouldWork bool
	}{
		{"https://www.baidu.com", true},
		{"https://www.bing.com", true},
		{"http://www.taobao.com", true},
		{"http://httpbin.org/ip", true},
	}

	// 添加TCP测试地址
	tcpAddr := "211.101.235.126:3306"

	// 测试不同的代理类型
	proxyTests := []struct {
		name       string
		proxyType  C.ProxyType
		proxyIP    string
		proxyPort  int
		shouldWork bool
	}{
		// http代理
		{
			name:       "HTTP代理",
			proxyType:  C.HTTP,
			proxyIP:    "127.0.0.1",
			proxyPort:  9001,
			shouldWork: true,
		},
		// https代理
		{
			name:       "HTTPS代理",
			proxyType:  C.HTTPS,
			proxyIP:    "127.0.0.1",
			proxyPort:  9002,
			shouldWork: true,
		},
		// http2代理
		{
			name:       "http2代理",
			proxyType:  C.HTTP2,
			proxyIP:    "127.0.0.1",
			proxyPort:  9003,
			shouldWork: true,
		},
		// socks4代理
		{
			name:       "SOCKS4代理",
			proxyType:  C.SOCKS4,
			proxyIP:    "127.0.0.1",
			proxyPort:  9004,
			shouldWork: true,
		},
		// socks5代理
		{
			name:       "SOCKS5代理",
			proxyType:  C.SOCKS5,
			proxyIP:    "127.0.0.1",
			proxyPort:  9005,
			shouldWork: true,
		},
		// 禁用代理
		{
			name:       "禁用代理",
			proxyType:  "",
			proxyIP:    "",
			proxyPort:  0,
			shouldWork: true,
		},
	}

	for _, pt := range proxyTests {
		t.Run(pt.name, func(t *testing.T) {
			// 修改代理配置，确保更安全的超时设置
			cfg := &C.Config{
				Enable:        pt.proxyType != "",
				ProxyType:     pt.proxyType,
				ProxyIP:       pt.proxyIP,
				ProxyPort:     pt.proxyPort,
				IdleTimeout:   time.Minute * 5,
				KeepAlive:     time.Minute * 5,
				MetricsEnable: true,
				HTTPConfig: &C.HTTPConfig{
					SkipVerify: true,
					Timeout:    time.Second * 30,
				},
				SOCKSConfig: &C.SOCKSConfig{
					Timeout:   time.Second * 30,
					KeepAlive: time.Second * 30,
				},
			}

			// 创建代理管理器
			pm, err := proxy.New(cfg)
			if err != nil {
				t.Fatalf("创建代理管理器失败: %v", err)
			}

			// 创建并启用hook
			h := hook.New(pm)
			if err := h.Enable(); err != nil {
				t.Fatalf("启用hook失败: %v", err)
			}
			defer h.Disable()

			// 测试HTTP请求
			for _, url := range testURLs {
				t.Run(url.url, func(t *testing.T) {
					testRequest(t, url.url, url.shouldWork)
				})
			}

			// 测试TCP连接
			t.Run("TCP连接测试", func(t *testing.T) {
				testTCPConnection(t, tcpAddr, pt.shouldWork)
			})

			// 检查指标
			metrics := pm.GetMetrics()
			t.Logf("代理类型 %s 的指标统计:\n", pt.name)
			t.Logf("活跃连接数: %d\n", metrics.ActiveConnections)
			t.Logf("总连接数: %d\n", metrics.TotalConnections)
			t.Logf("失败连接数: %d\n", metrics.FailedConnections)
			if metrics.TotalConnections > 0 {
				t.Logf("平均连接时间: %v\n", metrics.ConnectionDuration/time.Duration(metrics.TotalConnections))
			} else {
				t.Logf("平均连接时间: 0\n")
			}
		})
	}
}

func testRequest(t *testing.T, url string, shouldWork bool) {
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := client.Get(url)
	if shouldWork {
		if err != nil {
			t.Errorf("请求 %s 失败: %v", url, err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Errorf("读取响应失败: %v", err)
			return
		}

		t.Logf("成功请求 %s, 状态码: %d, 响应长度: %d", url, resp.StatusCode, len(body))
	} else {
		if err == nil {
			resp.Body.Close()
			t.Errorf("预期请求 %s 应该失败，但成功了", url)
		}
		// t.Logf("请求 %s 失败: %v", url, err)
	}
}

// testTCPConnection 测试TCP连接
func testTCPConnection(t *testing.T, addr string, shouldWork bool) {
	// 增加连接超时时间
	timeout := time.Second * 5
	conn, err := net.DialTimeout("tcp", addr, timeout)

	if shouldWork {
		if err != nil {
			t.Errorf("TCP连接失败 %s: %v", addr, err)
			return
		}
		defer conn.Close()

		// 发送测试数据
		testData := []byte("test")
		_, err = conn.Write(testData)
		if err != nil {
			t.Errorf("发送数据失败: %v", err)
			return
		}

		// 读取响应
		buffer := make([]byte, 1024)
		_, err = conn.Read(buffer)
		if err != nil {
			t.Errorf("读取响应失败: %v", err)
			return
		}

		t.Logf("TCP连接测试成功: %s, 响应: %s", addr, string(buffer))
	} else {
		if err == nil {
			conn.Close()
			t.Errorf("预期TCP连接 %s 应该失败，但成功了", addr)
		}
	}
}

// TestHookDisable 测试禁用hook功能
// func TestHookDisable(t *testing.T) {
// 	cfg := &C.Config{
// 		Enable:    true,
// 		ProxyType: C.SOCKS5,
// 		ProxyIP:   "127.0.0.1",
// 		ProxyPort: 1080,
// 	}

// 	pm, err := proxy.New(cfg)
// 	if err != nil {
// 		t.Fatalf("创建代理管理器失败: %v", err)
// 	}

// 	h := hook.New(pm)

// 	// 启用hook
// 	if err := h.Enable(); err != nil {
// 		t.Fatalf("启用hook失败: %v", err)
// 	}

// 	// 测试测试
// 	testRequest(t, "https://www.google.com", true)

// 	// 禁用hook
// 	if err := h.Disable(); err != nil {
// 		t.Fatalf("禁用hook失败: %v", err)
// 	}

// 	// 验证hook已被禁用
// 	testRequest(t, "https://www.youtube.com", false)
// }

// TestHookWithInvalidProxy 测试无效代理配置
func TestHookWithInvalidProxy(t *testing.T) {
	cfg := &C.Config{
		Enable:    true,
		ProxyType: C.HTTP,
		ProxyIP:   "invalid-ip",
		ProxyPort: -1,
	}

	_, err := proxy.New(cfg)
	if err == nil {
		t.Fatal("预期应该返回错误，但没有")
	}

	t.Logf("正确捕获无效配置错误: %v", err)
}
