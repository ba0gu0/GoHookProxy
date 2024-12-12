package errors

import (
	"errors"
	"fmt"
)

var (
	// 基础错误
	ErrInvalidConfig    = errors.New("invalid proxy configuration")
	ErrUnsupportedProxy = errors.New("unsupported proxy type")
	ErrHookFailed       = errors.New("failed to hook network operations")
	ErrProxyDialFailed  = errors.New("proxy dial failed")

	// 代理特定错误
	ErrHTTPProxyAuth    = errors.New("http proxy authentication failed")
	ErrSOCKS5Auth       = errors.New("socks5 authentication failed")
	ErrSOCKS4AAuth      = errors.New("socks4a authentication failed")
	ErrProxyProtocol    = errors.New("proxy protocol error")
	ErrProxyNegotiation = errors.New("proxy negotiation failed")

	// 连接错误
	ErrConnectionTimeout = errors.New("connection timeout")
	ErrConnectionReset   = errors.New("connection reset by peer")
	ErrConnectionClosed  = errors.New("connection closed unexpectedly")

	// TLS 错误
	ErrTLSHandshake   = errors.New("TLS handshake failed")
	ErrCertValidation = errors.New("certificate validation failed")
	ErrTLSConfig      = errors.New("TLS configuration is missing")

	// Context 错误
	ErrContextCanceled         = errors.New("operation canceled by context")
	ErrContextDeadlineExceeded = errors.New("operation deadline exceeded")

	// 资源错误
	ErrPoolExhausted = errors.New("connection pool exhausted")
	ErrResourceLimit = errors.New("resource limit exceeded")

	// SOCKS 特定错误
	ErrSOCKSVersionNotSupported     = errors.New("socks: unsupported protocol version")
	ErrSOCKSCommandNotSupported     = errors.New("socks: unsupported command")
	ErrSOCKSAddressTypeNotSupported = errors.New("socks: unsupported address type")
	ErrSOCKSNetworkNotSupported     = errors.New("socks: unsupported network type")
	ErrSOCKSRequestFailed           = errors.New("socks request failed")
	ErrSOCKSHandshakeFailed         = errors.New("socks handshake failed")

	// SOCKS4 特定错误
	ErrSOCKS4RequestRejected = errors.New("socks4 request rejected")
	ErrSOCKS4IdentdFailed    = errors.New("socks4 identd failed")
	ErrSOCKS4IdentdMismatch  = errors.New("socks4 identd user mismatch")

	// SOCKS5 特定错误
	ErrSOCKS5NoAcceptableMethods     = errors.New("socks5 no acceptable auth methods")
	ErrSOCKS5GeneralFailure          = errors.New("socks5 general server failure")
	ErrSOCKS5NetworkUnreachable      = errors.New("socks5 network unreachable")
	ErrSOCKS5HostUnreachable         = errors.New("socks5 host unreachable")
	ErrSOCKS5ConnectionRefused       = errors.New("socks5 connection refused")
	ErrSOCKS5TTLExpired              = errors.New("socks5 ttl expired")
	ErrSOCKS5CommandNotSupported     = errors.New("socks5 command not supported")
	ErrSOCKS5AddressTypeNotSupported = errors.New("socks5 address type not supported")

	// SOCKS特定错误
	ErrSOCKSAuthMethodNotSupported = errors.New("socks: authentication method not supported")
	ErrSOCKSAuthFailed             = errors.New("socks: authentication failed")
	ErrSOCKSAuthTimeout            = errors.New("socks: authentication timeout")
	ErrSOCKSAuthRetryExceeded      = errors.New("socks: max authentication retries exceeded")

	// 连接相关错误
	ErrSOCKSConnectFailed    = errors.New("socks: connect to target failed")
	ErrSOCKSConnectTimeout   = errors.New("socks: connect timeout")
	ErrSOCKSProxyUnreachable = errors.New("socks: proxy server unreachable")
)

// WrapError 包装错误信息
func WrapError(err error, message string) error {
	return fmt.Errorf("%s: %w", message, err)
}
