package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	C "github.com/ba0gu0/GoHookProxy/config"
	"github.com/ba0gu0/GoHookProxy/errors"
	"github.com/ba0gu0/GoHookProxy/metrics"
)

// ProxyManager 代理管理器
type ProxyManager struct {
	mu      sync.RWMutex
	Config  *C.Config
	dialer  ProxyDialer
	Metrics *metrics.MetricsCollector
}

// ProxyDialer 代理拨号器接口
type ProxyDialer interface {
	Dial(network, addr string) (net.Conn, error)
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// New 创建代理管理器
func New(config *C.Config) (*ProxyManager, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	pm := &ProxyManager{}

	// 只在启用指标收集时创建 MetricsCollector
	if config.MetricsEnable {
		pm.Metrics = metrics.NewMetricsCollector()
	}

	// 更新配置
	if err := pm.UpdateConfig(config); err != nil {
		return nil, err
	}

	return pm, nil
}

// UpdateConfig 更新代理配置
func (pm *ProxyManager) UpdateConfig(config *C.Config) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if config == nil {
		pm.Config = nil
		pm.dialer = nil
		return nil
	}

	if err := config.Validate(); err != nil {
		return err
	}

	dialer, err := createProxyDialer(config, pm.Metrics)
	if err != nil {
		return err
	}

	pm.Config = config
	pm.dialer = dialer
	return nil
}

// GetDialer 获取代理拨号器
func (pm *ProxyManager) GetDialer() ProxyDialer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.dialer
}

// createProxyDialer 创建代理拨号器
func createProxyDialer(config *C.Config, metrics *metrics.MetricsCollector) (ProxyDialer, error) {
	if !config.Enable {
		return &net.Dialer{
			Timeout:   config.IdleTimeout,
			KeepAlive: config.KeepAlive,
		}, nil
	}

	switch config.ProxyType {
	case C.HTTP, C.HTTPS, C.HTTP2:
		return createHTTPProxyDialer(config.ProxyType, config.ProxyIP, config.ProxyPort, config.HTTPConfig, metrics)
	case C.SOCKS4, C.SOCKS5:
		return createSocksDialer(config.ProxyType, config.ProxyIP, config.ProxyPort, config.SOCKSConfig, metrics)
	case C.Direct:
		return &net.Dialer{
			Timeout:   config.IdleTimeout,
			KeepAlive: config.KeepAlive,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", config.ProxyType)
	}
}

// GetMetrics 获取指标
func (pm *ProxyManager) GetMetrics() *metrics.Metrics {
	if !pm.Config.MetricsEnable || pm.Metrics == nil {
		return &metrics.Metrics{}
	}
	return pm.Metrics.GetSnapshot()
}

// IsProxyAddress 判断给定地址是否为代理地址
func (pm *ProxyManager) IsProxyAddress(addr string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.Config == nil || !pm.Config.Enable {
		return false
	}

	proxyAddr := fmt.Sprintf("%s:%d", pm.Config.ProxyIP, pm.Config.ProxyPort)
	return addr == proxyAddr
}

// Dial 实现 ProxyDialer 接口
func (pm *ProxyManager) Dial(network, addr string) (net.Conn, error) {
	return pm.DialContext(context.Background(), network, addr)
}

// DialContext 实现 ProxyDialer 接口
func (pm *ProxyManager) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	start := time.Now()

	if pm.Config.MetricsEnable && pm.Metrics != nil {
		pm.Metrics.RecordProtocol(network)
	}

	dialer := pm.GetDialer()
	if dialer == nil {
		return nil, errors.ErrUnsupportedProxy
	}

	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		if pm.Metrics != nil {
			pm.Metrics.RecordFailure(err)
		}
		return nil, err
	}

	if pm.Config.MetricsEnable && pm.Metrics != nil {
		pm.Metrics.RecordLatency(time.Since(start))
	}

	return conn, nil
}
