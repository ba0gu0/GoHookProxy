package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	C "github.com/ba0gu0/GoHookProxy/config"
	"github.com/ba0gu0/GoHookProxy/errors"
	"github.com/ba0gu0/GoHookProxy/metrics"

	"golang.org/x/net/http2"
)

// HTTPProxyDialer HTTP代理拨号器
type HTTPProxyDialer struct {
	proxyURL  *url.URL
	proxyType C.ProxyType
	dialer    *net.Dialer
	tlsConfig *tls.Config
	Config    *C.HTTPConfig
	metrics   *metrics.MetricsCollector
}

// Dial 实现 ProxyDialer 接口
func (d *HTTPProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// DialContext 实现 ProxyDialer 接口
func (d *HTTPProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	start := time.Now()

	// 记录总连接数
	if d.metrics != nil {
		d.metrics.RecordConnection(0) // 先记录连接,duration后面再更新
	}

	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.WrapError(errors.ErrUnsupportedProxy, fmt.Sprintf("unsupported network type: %s", network))
	}

	var conn net.Conn
	var err error

	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.ErrContextDeadlineExceeded
		}
		return nil, errors.ErrContextCanceled
	default:
		switch d.proxyType {
		case C.HTTP:
			conn, err = d.dialHTTP(ctx, addr)
		case C.HTTPS:
			conn, err = d.dialHTTPS(ctx, addr)
		case C.HTTP2:
			conn, err = d.dialHTTP2(ctx, addr)
		default:
			return nil, errors.WrapError(errors.ErrUnsupportedProxy, string(d.proxyType))
		}
	}

	if err != nil {
		if d.metrics != nil {
			d.metrics.RecordFailure(err)
		}
		return nil, err
	}

	if d.metrics != nil {
		d.metrics.IncrementActiveConnections()
		d.metrics.RecordConnection(time.Since(start))
	}

	return conn, nil
}

// dialHTTP 处理普通 HTTP 代理连接
func (d *HTTPProxyDialer) dialHTTP(ctx context.Context, addr string) (net.Conn, error) {
	// 建立 TCP 连接
	conn, err := d.dialer.DialContext(ctx, "tcp", d.proxyURL.Host)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.ErrConnectionTimeout
		}
		return nil, errors.WrapError(errors.ErrProxyDialFailed, err.Error())
	}

	// 发送 CONNECT 请求
	if err := d.sendConnectRequest(conn, addr); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// dialHTTPS 处理 HTTPS 代理连接
func (d *HTTPProxyDialer) dialHTTPS(ctx context.Context, addr string) (net.Conn, error) {
	// 检查必要的配置
	if d.tlsConfig == nil {
		return nil, errors.WrapError(errors.ErrTLSConfig, "TLS configuration is missing")
	}

	// 建立 TCP 连接
	conn, err := d.dialer.DialContext(ctx, "tcp", d.proxyURL.Host)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.ErrConnectionTimeout
		}
		return nil, errors.WrapError(errors.ErrProxyDialFailed, err.Error())
	}

	// 确保连接在出错时被关闭
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	// 克隆 TLS 配置以避免并发问题
	tlsConfig := d.tlsConfig.Clone()

	// 升级到 TLS
	tlsConn := tls.Client(conn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, errors.WrapError(errors.ErrTLSHandshake, err.Error())
	}

	// 发送 CONNECT 请求
	if err := d.sendConnectRequest(tlsConn, addr); err != nil {
		return nil, err
	}

	return tlsConn, nil
}

type http2Conn struct {
	reader     *io.PipeReader
	writer     *io.PipeWriter
	stream     io.ReadCloser
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     chan struct{}
	closeOnce  sync.Once
	err        error
}

func (c *http2Conn) closeWithError(err error) {
	c.err = err
	c.Close()
}

func (c *http2Conn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *http2Conn) Write(b []byte) (n int, err error) {
	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
		return c.writer.Write(b)
	}
}

func (c *http2Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.stream != nil {
			c.stream.Close()
		}
		c.reader.Close()
		c.writer.Close()
	})
	return nil
}

// 添加缺失的接口方法
func (c *http2Conn) LocalAddr() net.Addr  { return c.localAddr }
func (c *http2Conn) RemoteAddr() net.Addr { return c.remoteAddr }
func (c *http2Conn) SetDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "http2", Err: errors.ErrUnsupportedProxy}
}
func (c *http2Conn) SetReadDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "http2", Err: errors.ErrUnsupportedProxy}
}
func (c *http2Conn) SetWriteDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "http2", Err: errors.ErrUnsupportedProxy}
}

// dialHTTP2 处理 HTTP2 代理连接
func (d *HTTPProxyDialer) dialHTTP2(ctx context.Context, addr string) (net.Conn, error) {
	transport := &http2.Transport{
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			conn, err := d.dialer.DialContext(ctx, network, d.proxyURL.Host)
			if err != nil {
				return nil, errors.WrapError(errors.ErrProxyDialFailed, err.Error())
			}

			tlsConfig := cfg.Clone()
			tlsConfig.NextProtos = []string{"h2"}
			tlsConn := tls.Client(conn, tlsConfig)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, errors.WrapError(errors.ErrTLSHandshake, err.Error())
			}
			return tlsConn, nil
		},
		TLSClientConfig: d.tlsConfig,
	}

	client := &http.Client{
		Transport: transport,
	}

	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect,
		fmt.Sprintf("https://%s", d.proxyURL.Host), pr)
	if err != nil {
		return nil, errors.WrapError(errors.ErrProxyNegotiation, err.Error())
	}

	req.Host = addr
	if d.Config.User != "" {
		req.SetBasicAuth(d.Config.User, d.Config.Pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WrapError(errors.ErrProxyNegotiation, err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.WrapError(errors.ErrProxyProtocol, resp.Status)
	}

	return &http2Conn{
		reader:     pr,
		writer:     pw,
		stream:     resp.Body,
		localAddr:  &net.TCPAddr{IP: net.IPv4zero, Port: 0},
		remoteAddr: &net.TCPAddr{IP: net.IPv4zero, Port: 0},
		closed:     make(chan struct{}),
	}, nil
}

// sendConnectRequest 发送 CONNECT 请并处理响应
func (d *HTTPProxyDialer) sendConnectRequest(conn net.Conn, addr string) error {
	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Host: addr},
		Host:   addr,
		Header: make(http.Header),
	}

	if d.Config.User != "" {
		req.SetBasicAuth(d.Config.User, d.Config.Pass)
	}

	if err := req.Write(conn); err != nil {
		return errors.WrapError(errors.ErrProxyNegotiation, err.Error())
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return errors.WrapError(errors.ErrProxyNegotiation, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusProxyAuthRequired {
		return errors.ErrHTTPProxyAuth
	}

	if resp.StatusCode != http.StatusOK {
		return errors.WrapError(errors.ErrProxyProtocol, resp.Status)
	}

	return nil
}

// createHTTPProxyDialer 创建 HTTP 代理拨号器
func createHTTPProxyDialer(proxyType C.ProxyType, ip string, port int, config *C.HTTPConfig, metrics *metrics.MetricsCollector) (ProxyDialer, error) {
	if config == nil {
		config = C.DefaultHTTPConfig()
	}

	proxyURL := &url.URL{
		Scheme: string(proxyType),
		Host:   fmt.Sprintf("%s:%d", ip, port),
	}

	// 设置认证信息
	if config.User != "" {
		proxyURL.User = url.UserPassword(config.User, config.Pass)
	}

	// 配置 TLS
	tlsConfig := &tls.Config{
		MinVersion:         config.TLSMinVersion,
		InsecureSkipVerify: config.SkipVerify,
		NextProtos:         []string{"h2", "http/1.1"}, // 支持 HTTP2
	}

	// 加载证书
	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, errors.WrapError(errors.ErrCertValidation, err.Error())
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return &HTTPProxyDialer{
		proxyURL:  proxyURL,
		proxyType: proxyType,
		dialer: &net.Dialer{
			Timeout:   config.Timeout,
			KeepAlive: config.KeepAlive,
		},
		tlsConfig: tlsConfig,
		Config:    config,
		metrics:   metrics,
	}, nil
}
