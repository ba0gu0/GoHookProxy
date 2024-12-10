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
	pool    *ConnPool
	Metrics *metrics.MetricsCollector
}

// ProxyDialer 代理拨号器接口
type ProxyDialer interface {
	Dial(network, addr string) (net.Conn, error)
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// New 创建代理管理器
func New(config *C.Config) (*ProxyManager, error) {
	// 添加config验证
	if err := config.Validate(); err != nil {
		return nil, err
	}

	pm := &ProxyManager{}

	// 只在启用指标收集时创建 MetricsCollector
	if config.MetricsEnable {
		pm.Metrics = metrics.NewMetricsCollector()
	}

	// 先更新配置
	if err := pm.UpdateConfig(config); err != nil {
		return nil, err
	}

	// 创建连接池时传入 metrics
	pm.pool = NewConnPool(
		config.MaxIdleConns,
		config.MaxTotalConns,
		config.IdleTimeout,
		pm.Metrics, // 可以为 nil
	)

	// 启动定期清理
	go pm.startCleanup()

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

func (pm *ProxyManager) startCleanup() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		pm.pool.CleanUp()
	}
}

// GetMetrics 获取指标
func (pm *ProxyManager) GetMetrics() *metrics.Metrics {
	// 检查是否启用了指标收集
	if !pm.Config.MetricsEnable || pm.Metrics == nil {
		return &metrics.Metrics{} // 返回空指标
	}
	return pm.Metrics.GetSnapshot()
}

// IsProxyAddress 判断给定地址是否为代理地址
func (pm *ProxyManager) IsProxyAddress(addr string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 如果配置为空或代理未启用，则不是代理地址
	if pm.Config == nil || !pm.Config.Enable {
		return false
	}

	// 构造代理地址
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

	// 只在启用指标收集时记录协议类型
	if pm.Config.MetricsEnable && pm.Metrics != nil {
		pm.Metrics.RecordProtocol(network)
	}

	// 1. 尝试从连接池获取
	if conn, err := pm.pool.Get(network, addr); err == nil && conn != nil {
		return &poolConn{
			Conn:    conn,
			network: network,
			addr:    addr,
			pool:    pm.pool,
		}, nil
	}

	// 2. 创建新连接
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

	// 只在启用指标收集时记录延迟
	if pm.Config.MetricsEnable && pm.Metrics != nil {
		pm.Metrics.RecordLatency(time.Since(start))
	}

	// 3. 包装连接
	return &poolConn{
		Conn:    conn,
		network: network,
		addr:    addr,
		pool:    pm.pool,
	}, nil
}
