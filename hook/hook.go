package hook

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/ba0gu0/GoHookProxy/proxy"

	"crypto/tls"
	"crypto/x509"
	"errors"

	"github.com/agiledragon/gomonkey/v2"
)

type Hook struct {
	proxyManager *proxy.ProxyManager
	patches      *gomonkey.Patches

	// 新增DNS解析缓存
	dnsCache sync.Map
	dnsTTL   time.Duration
}

func New(pm *proxy.ProxyManager) *Hook {
	return &Hook{
		proxyManager: pm,
		patches:      gomonkey.NewPatches(),
		dnsTTL:       5 * time.Minute, // 默认DNS缓存5分钟
	}
}

func directDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// 支持 TCP 和 UDP
	switch network {
	case "tcp", "tcp4", "tcp6":
		addr, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			return nil, err
		}
		conn, err := net.DialTCP(network, nil, addr)
		if err != nil {
			return nil, err
		}

		go func() {
			<-ctx.Done()
			conn.Close()
		}()

		return conn, nil

	case "udp", "udp4", "udp6":
		addr, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			return nil, err
		}
		conn, err := net.DialUDP(network, nil, addr)
		if err != nil {
			return nil, err
		}

		go func() {
			<-ctx.Done()
			conn.Close()
		}()

		return conn, nil

	default:
		return nil, fmt.Errorf("不支持的网络类型: %s", network)
	}
}

func (h *Hook) Enable() error {
	if !h.proxyManager.Config.Enable {
		return nil
	}

	// Hook DialContext
	h.patches.ApplyMethod(reflect.TypeOf(&net.Dialer{}), "DialContext",
		func(d *net.Dialer, ctx context.Context, network, addr string) (net.Conn, error) {
			start := time.Now()
			defer func() {
				if h.proxyManager.Config.MetricsEnable && h.proxyManager.Metrics != nil {
					h.proxyManager.Metrics.RecordLatency(time.Since(start))
				}
			}()

			if h.proxyManager.IsProxyAddress(addr) {
				return directDialContext(ctx, network, addr)
			}
			return h.proxyManager.DialContext(ctx, network, addr)
		})

	if !h.proxyManager.Config.DNSHook {
		return nil
	}

	// Hook DNS解析
	h.patches.ApplyFunc(net.ResolveIPAddr, func(network, address string) (*net.IPAddr, error) {
		// 实际解析
		ipAddr, err := net.ResolveIPAddr(network, address)
		if err != nil {
			// 只在启用指标收集时记录错误
			if h.proxyManager.Config.MetricsEnable && h.proxyManager.Metrics != nil {
				h.proxyManager.Metrics.RecordErrorType(err)
			}
			return nil, err
		}

		return ipAddr, nil
	})

	if !h.proxyManager.Config.TLSHook {
		return nil
	}

	// Hook TLS配置
	h.patches.ApplyMethod(reflect.TypeOf(&tls.Config{}), "Clone",
		func(c *tls.Config) *tls.Config {
			clone := c.Clone()

			// 注入自定义验证
			if clone.VerifyPeerCertificate == nil {
				clone.VerifyPeerCertificate = h.verifyPeerCertificate
			}

			return clone
		})

	return nil
}

type dnsCacheEntry struct {
	ipAddr    *net.IPAddr
	timestamp time.Time
}

// 自定义证书验证
func (h *Hook) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// 在这里添加自定义的证书验证逻辑
	if len(rawCerts) == 0 {
		return errors.New("no certificates provided")
	}

	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}

	// 检查证书是否过期
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not valid before %v", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate expired on %v", cert.NotAfter)
	}

	// 可以添加更多自定义验证...

	return nil
}

func (h *Hook) Disable() error {
	h.patches.Reset()
	return nil
}
