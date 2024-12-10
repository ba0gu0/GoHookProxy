# GoHookProxy

一个用于透明代理网络操作的 Go 库。
A Go library for transparently hooking network operations and routing them through a proxy.

## 特性 | Features

- 支持所有网络操作的透明代理
- 支持 HTTP、HTTPS、HTTP2、SOCKS4A 和 SOCKS5 代理
- 内置连接池
- 自动重试与指数退避
- 详细的指标收集
- 无需修改代码
- 线程安全
- 易于使用

- Transparent proxy support for all network operations
- Support for HTTP, HTTPS, HTTP2, SOCKS4A, and SOCKS5 proxies
- Built-in connection pooling
- Automatic retry with exponential backoff
- Detailed metrics collection
- No code modification required
- Thread-safe
- Easy to use

## 指标 | Metrics

该库提供详细的指标,包括:
The library provides detailed metrics including:

- 活跃连接数 | Active connections
- 总连接数 | Total connections
- 失败连接数 | Failed connections
- 连接持续时间 | Connection duration
- 发送/接收字节数 | Bytes sent/received
- 连接延迟分布 | Connection latency distribution
- 错误分布 | Error distribution
- 协议统计 | Protocol statistics
- 带宽使用情况 | Bandwidth usage

## 重试机制 | Retry Mechanism

可配置的自动重试机制:
Automatic retry with configurable:

- 最大重试次数 | Maximum retries
- 初始延迟 | Initial delay
- 最大延迟 | Maximum delay
- 退避倍数 | Backoff multiplier

## 安装 | Installation

```bash
go get github.com/ba0gu0/GoHookProxy
```

## 快速开始 | Quick Start

```go
package main

import (
    "log"
    "time"
    "github.com/ba0gu0/GoHookProxy/config"
    "github.com/ba0gu0/GoHookProxy/proxy"
    "github.com/ba0gu0/GoHookProxy/hook"
)

func main() {
    // 创建代理配置 | Create proxy configuration
    cfg := config.DefaultConfig()
    
    cfg.Enable = true
    cfg.ProxyType = config.SOCKS5
    cfg.ProxyIP = "127.0.0.1"
    cfg.ProxyPort = 1080
    
    // 可选: 启用指标收集 | Optional: Enable metrics collection
    cfg.MetricsEnable = true
    
    // 可选: 设置连接池参数 | Optional: Set connection pool parameters
    cfg.MaxIdleConns = 100
    cfg.MaxTotalConns = 1000
    cfg.IdleTimeout = 90 * time.Second

    // 创建代理管理器 | Create proxy manager
    pm, err := proxy.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // 创建并启用 hook | Create and enable hook
    h := hook.New(pm)
    if err := h.Enable(); err != nil {
        log.Fatal(err)
    }
    defer h.Disable()

    // 所有网络操作现在都会通过代理 | All network operations will now go through the proxy
    // Your code here...
}
```

## 配置 | Configuration

代理配置支持以下选项:
The proxy configuration supports the following options:

```go
type Config struct {
    // 基础设置 | Basic settings
    Enable        bool      // 启用/禁用代理 | Enable/disable proxy
    ProxyType     string    // 代理类型 | Proxy type: "http", "https", "http2", "socks4a", "socks5"
    ProxyIP       string    // 代理服务器地址 | Proxy server address
    ProxyPort     int       // 代理服务器端口 | Proxy server port
    
    // 连接池设置 | Connection pool settings
    MaxIdleConns  int           // 最大空闲连接数 | Maximum idle connections
    MaxTotalConns int           // 最大总连接数 | Maximum total connections
    IdleTimeout   time.Duration // 空闲连接超时时间 | Idle connection timeout
    KeepAlive     time.Duration // TCP keepalive 间隔 | TCP keepalive interval
    
    // TLS 设置 | TLS settings
    SkipVerify    bool   // 是否跳过证书验证(默认为 true) | Skip certificate verification (default: true)
    CertFile      string // 可选的客户端证书文件 | Optional client certificate file
    KeyFile       string // 可选的客户端密钥文件 | Optional client key file
    
    // HTTP 代理设置 | HTTP proxy settings
    HTTPConfig    *HTTPConfig
    
    // SOCKS 代理设置 | SOCKS proxy settings
    SOCKSConfig   *SOCKSConfig
    
    // 指标收集设置 | Metrics collection settings
    MetricsEnable bool // 是否启用指标收集 | Enable metrics collection
}

type HTTPConfig struct {
    Username    string        // HTTP 代理用户名 | HTTP proxy username
    Password    string        // HTTP 代理密码 | HTTP proxy password
    Timeout     time.Duration // 请求超时时间 | Request timeout
    KeepAlive   time.Duration // Keep-alive 时间 | Keep-alive duration
}

type SOCKSConfig struct {
    User        string        // SOCKS 代理用户名 | SOCKS proxy username
    Pass        string        // SOCKS 代理密码 | SOCKS proxy password
    Timeout     time.Duration // 连接超时时间 | Connection timeout
    KeepAlive   time.Duration // Keep-alive 时间 | Keep-alive duration
}
```

## 支持的代理类型 | Supported Proxy Types

- HTTP
- HTTPS
- HTTP/2
- SOCKS4A
- SOCKS5

## TLS 设置 | TLS Settings

- HTTP/HTTPS/HTTP2 代理默认不验证证书(SkipVerify=true)
- HTTP/HTTPS/HTTP2 proxies skip certificate verification by default (SkipVerify=true)

如果需要启用证书验证:
To enable certificate verification:

```go
cfg := &config.Config{
    Enable: true,
    ProxyType: config.HTTP2,
    ProxyIP: "127.0.0.1",
    ProxyPort: 1080,
    SkipVerify: false,  // 启用证书验证 | Enable certificate verification
    CertFile: "/path/to/cert.pem",  // 可选 | Optional
    KeyFile: "/path/to/key.pem",    // 可选 | Optional
}
```

## 线程安全 | Thread Safety

该库完全线程安全。你可以:
The library is completely thread-safe. You can:

- 在运行时更新代理配置 | Update proxy configuration at runtime
- 随时启用/禁用代理 | Enable/disable proxy at any time
- 在并发环境中使用 | Use in concurrent environments

## 错误处理 | Error Handling

该库提供详细的错误类型以便更好地错误处理:
The library provides detailed error types for better error handling:

```go
var (
    // 基础错误 | Basic errors
    ErrInvalidConfig    // 无效的代理配置 | Invalid proxy configuration
    ErrUnsupportedProxy // 不支持的代理类型 | Unsupported proxy type
    ErrHookFailed      // hook 网络操作失败 | Failed to hook network operations
    ErrProxyDialFailed // 代理连接失败 | Proxy connection failed

    // 代理特定错误 | Proxy-specific errors
    ErrHTTPProxyAuth    // HTTP 代理认证失败 | HTTP proxy authentication failed
    ErrSOCKS5Auth       // SOCKS5 认证失败 | SOCKS5 authentication failed
    ErrSOCKS4AAuth      // SOCKS4A 认证失败 | SOCKS4A authentication failed
    ErrProxyProtocol    // 代理协议错误 | Proxy protocol error
    ErrProxyNegotiation // 代理协商失败 | Proxy negotiation failed

    // 连接错误 | Connection errors
    ErrConnectionTimeout // 连接超时 | Connection timeout
    ErrConnectionReset  // 连接被重置 | Connection reset by peer
    ErrConnectionClosed // 连接意外关闭 | Connection closed unexpectedly

    // TLS 错误 | TLS errors
    ErrTLSHandshake   // TLS 握手失败 | TLS handshake failed
    ErrCertValidation // 证书验证失败 | Certificate validation failed
)
```

## 示例 | Examples

查看 [examples](./example) 目录获取更多使用示例。
See the [examples](./example) directory for more usage examples.

## 贡献 | Contributing

欢迎贡献!请随时提交 Pull Request。
Contributions are welcome! Please feel free to submit a Pull Request.

## 许可证 | License

该项目基于 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
