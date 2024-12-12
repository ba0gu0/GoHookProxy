package config

import (
	"fmt"
	"time"
)

// Default values
const (
	DefaultIdleTimeout = time.Minute * 5
	DefaultKeepAlive   = time.Minute * 5

	// HTTP proxy defaults
	DefaultHTTPTimeout    = time.Second * 30
	DefaultHTTPKeepAlive  = time.Second * 30
	DefaultHTTPSkipVerify = true
	DefaultHTTPCertFile   = ""
	DefaultHTTPKeyFile    = ""
	DefaultHTTPUser       = ""
	DefaultHTTPPass       = ""

	// SOCKS defaults
	DefaultSOCKSTimeout   = time.Second * 30
	DefaultSOCKSKeepAlive = time.Second * 30
	DefaultSOCKSUser      = ""
	DefaultSOCKSPass      = ""

	// Hook defaults
	DefaultDNSHook       = false
	DefaultTLSHook       = false
	DefaultMetricsEnable = false // 默认关闭指标收集
)

// ProxyType 代理类型
type ProxyType string

const (
	Direct  ProxyType = "direct"
	HTTP    ProxyType = "http"
	HTTPS   ProxyType = "https"
	HTTP2   ProxyType = "http2"
	SOCKS4  ProxyType = "socks4"
	SOCKS4A ProxyType = "socks4a"
	SOCKS5  ProxyType = "socks5"
)

type Config struct {
	IdleTimeout time.Duration
	KeepAlive   time.Duration

	// Proxy configurations
	HTTPConfig  *HTTPConfig
	SOCKSConfig *SOCKSConfig

	// Proxy settings
	ProxyType ProxyType
	ProxyIP   string
	ProxyPort int
	Enable    bool

	// Hook settings
	DNSHook       bool
	TLSHook       bool
	MetricsEnable bool
}

type HTTPConfig struct {
	Timeout       time.Duration
	KeepAlive     time.Duration
	User          string
	Pass          string
	TLSMinVersion uint16
	SkipVerify    bool
	CertFile      string
	KeyFile       string

	// HTTP2 特定配置
	MaxConcurrentStreams uint32 // 最大并发流数
	InitialWindowSize    uint32 // 初始窗口大小
	MaxFrameSize         uint32 // 最大帧大小
}

// SOCKSConfig 统一的SOCKS配置结构
type SOCKSConfig struct {
	EnableUDP  bool
	Timeout    time.Duration
	KeepAlive  time.Duration
	MaxRetries int
	RetryDelay time.Duration
	User       string // SOCKS5 专用
	Pass       string // SOCKS5 专用
}

// DefaultSOCKSConfig 返回默认SOCKS配置
func DefaultSOCKSConfig() *SOCKSConfig {
	return &SOCKSConfig{
		EnableUDP:  false,
		Timeout:    DefaultSOCKSTimeout,
		KeepAlive:  DefaultSOCKSKeepAlive,
		User:       DefaultSOCKSUser,
		Pass:       DefaultSOCKSPass,
		MaxRetries: 3,
		RetryDelay: time.Second * 5,
	}
}

func DefaultHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Timeout:    DefaultHTTPTimeout,
		KeepAlive:  DefaultHTTPKeepAlive,
		SkipVerify: DefaultHTTPSkipVerify,
		CertFile:   DefaultHTTPCertFile,
		KeyFile:    DefaultHTTPKeyFile,
		User:       DefaultHTTPUser,
		Pass:       DefaultHTTPPass,
	}
}

func DefaultConfig() *Config {
	return &Config{
		IdleTimeout: DefaultIdleTimeout,
		KeepAlive:   DefaultKeepAlive,
		HTTPConfig:  DefaultHTTPConfig(),
		SOCKSConfig: DefaultSOCKSConfig(), // 使用新的默认配置

		ProxyType:     Direct,
		ProxyIP:       "",
		ProxyPort:     0,
		Enable:        false,
		DNSHook:       DefaultDNSHook,
		TLSHook:       DefaultTLSHook,
		MetricsEnable: DefaultMetricsEnable, // 默认关闭
	}
}

// GetProxyAddr 返回完整的代理地址
func (c *Config) GetProxyAddr() string {
	return fmt.Sprintf("%s:%d", c.ProxyIP, c.ProxyPort)
}

// Validate 验证代理配置
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}

	// 验证IP
	if c.ProxyIP == "" {
		return fmt.Errorf("proxy IP cannot be empty")
	}

	// 验证端口
	if c.ProxyPort <= 0 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port: %d", c.ProxyPort)
	}

	switch c.ProxyType {
	case HTTP, HTTPS, HTTP2, SOCKS4, SOCKS4A, SOCKS5:
		return nil
	default:
		return fmt.Errorf("unsupported proxy type: %s", c.ProxyType)
	}
}
