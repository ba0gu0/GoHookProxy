package test

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	C "github.com/ba0gu0/GoHookProxy/config"
	PM "github.com/ba0gu0/GoHookProxy/proxy"
)

func TestProxyConnections(t *testing.T) {
	// 定义要测试的目标URL
	targets := []struct {
		name    string
		url     string
		isTCP   bool
		tcpHost string
		tcpPort string
	}{
		{"HTTPBin", "http://httpbin.org/get?id=1", false, "", ""},
		{"Baidu", "https://www.baidu.com", false, "", ""},
		{"CustomServer", "", true, "211.101.235.126", "3306"},
	}

	// 定义要测试的代理配置
	proxies := []struct {
		name      string
		ip        string
		port      int
		proxyType C.ProxyType
	}{
		{"HTTP", "127.0.0.1", 9001, C.HTTP},
		{"HTTPS", "127.0.0.1", 9002, C.HTTPS},
		{"HTTP2", "127.0.0.1", 9003, C.HTTP2},
		{"SOCKS4a", "127.0.0.1", 9004, C.SOCKS4},
		{"SOCKS5", "127.0.0.1", 9005, C.SOCKS5},
	}

	for _, proxy := range proxies {
		t.Run(proxy.name, func(t *testing.T) {
			// 创建代理配置
			cfg := C.DefaultConfig()
			cfg.Enable = true
			cfg.ProxyType = proxy.proxyType
			cfg.ProxyIP = proxy.ip
			cfg.ProxyPort = proxy.port
			cfg.HTTPConfig.SkipVerify = true
			cfg.MetricsEnable = true

			// 创建代理管理器
			pm, err := PM.New(cfg)
			if err != nil {
				t.Fatalf("创建代理管理器失败: %v", err)
			}

			// 测试每个目标URL
			for _, target := range targets {
				t.Run(target.name, func(t *testing.T) {
					dialer := pm.GetDialer()
					if dialer == nil {
						t.Fatal("获取代理拨号器失败")
					}

					if target.isTCP {
						// TCP 连接测试
						conn, err := dialer.Dial("tcp", net.JoinHostPort(target.tcpHost, target.tcpPort))
						if err != nil {
							t.Errorf("TCP连接失败 [%s:%s]: %v", target.tcpHost, target.tcpPort, err)
							return
						}
						defer conn.Close()

						// 发送 "hello" 消息
						_, err = conn.Write([]byte("hello"))
						if err != nil {
							t.Errorf("发送数据失败: %v", err)
							return
						}

						// 读取响应
						buffer := make([]byte, 1024)
						n, err := conn.Read(buffer)
						if err != nil {
							t.Errorf("读取响应失败: %v", err)
							return
						}

						t.Logf("成功建立TCP连接到 [%s:%s]，收到响应: %s",
							target.tcpHost, target.tcpPort, string(buffer[:n]))
					} else {
						// HTTP/HTTPS 请求测试
						client := &http.Client{
							Transport: &http.Transport{
								DialContext: pm.DialContext,
							},
							Timeout: 10 * time.Second,
						}

						// 发送请求
						resp, err := client.Get(target.url)
						if err != nil {
							t.Errorf("请求失败 [%s]: %v", target.url, err)
							return
						}
						defer resp.Body.Close()

						// 读取响应
						body, err := io.ReadAll(resp.Body)
						if err != nil {
							t.Errorf("读取响应失败: %v", err)
							return
						}

						// 验证响应状态码
						if resp.StatusCode != http.StatusOK {
							t.Errorf("预期状态码 200, 实际获得: %d", resp.StatusCode)
							return
						}

						t.Logf("成功访问 [%s]，响应长度: %d bytes", target.url, len(body))
					}
				})
			}
		})
	}
}
