package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	C "github.com/ba0gu0/GoHookProxy/config"
	E "github.com/ba0gu0/GoHookProxy/errors"
	"github.com/ba0gu0/GoHookProxy/metrics"
)

// SOCKS4请求格式:
// +----+----+----+----+----+----+----+----+----+----+....+----+
// | VN | CD | DSTPORT |      DSTIP        | USERID       |NULL|
// +----+----+----+----+----+----+----+----+----+----+....+----+
//    1    1      2              4           variable       1
//
// VN: SOCKS版本号(0x04)
// CD: 命令码
//     0x01 = CONNECT
//     0x02 = BIND
// DSTPORT: 目标端口(2字节)
// DSTIP: 目标IP地址(4字节)
// USERID: 用户ID字符串(可变长度)
// NULL: 结束符(0x00)

// SOCKS4响应格式:
// +----+----+----+----+----+----+----+----+
// | VN | CD | DSTPORT |      DSTIP        |
// +----+----+----+----+----+----+----+----+
//   1    1      2              4
//
// CD: 返回码
//     0x5A = 请求granted
//     0x5B = 请求rejected
//     0x5C = 请求failed(无法连接到identd)
//     0x5D = 请求failed(identd用户ID不匹配)

// SOCKS5认证请求:
// +----+----------+----------+
// |VER | NMETHODS | METHODS  |
// +----+----------+----------+
// | 1  |    1     | 1 to 255 |
// +----+----------+----------+
//
// VER: SOCKS版本号(0x05)
// NMETHODS: 认证方法数量
// METHODS: 认证方法列表
//     0x00 = 无认证
//     0x01 = GSSAPI
//     0x02 = 用户名/密码
//     0x03-0x7F = IANA分配
//     0x80-0xFE = 私有方法

// SOCKS5认证响应:
// +----+--------+
// |VER | METHOD |
// +----+--------+
// | 1  |   1    |
// +----+--------+
//
// METHOD: 选择的认证方法
//     0x00 = 无认证
//     0xFF = 无可接受的方法

// SOCKS5用户名/密码认证:
// +----+------+----------+------+----------+
// |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
// +----+------+----------+------+----------+
// | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
// +----+------+----------+------+----------+

// SOCKS5认证响应:
// +----+--------+
// |VER | STATUS |
// +----+--------+
// | 1  |   1    |
// +----+--------+
//
// STATUS: 0x00 = 成功, 其他 = 失败

// SOCKS5请求:
// +----+-----+-------+------+----------+----------+
// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
// +----+-----+-------+------+----------+----------+
// | 1  |  1  | X'00' |  1   | Variable |    2     |
// +----+-----+-------+------+----------+----------+
//
// VER: SOCKS版本号(0x05)
// CMD: 命令码
//     0x01 = CONNECT
//     0x02 = BIND
//     0x03 = UDP ASSOCIATE
// RSV: 保留字段(0x00)
// ATYP: 地址类型
//     0x01 = IPv4
//     0x03 = 域名
//     0x04 = IPv6
// DST.ADDR: 目标地址(变长)
// DST.PORT: 目标端口(2字节)

// SOCKS5响应:
// +----+-----+-------+------+----------+----------+
// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
// +----+-----+-------+------+----------+----------+
// | 1  |  1  | X'00' |  1   | Variable |    2     |
// +----+-----+-------+------+----------+----------+
//
// REP: 响应码
//     0x00 = 成功
//     0x01 = 常规SOCKS服务器连接失败
//     0x02 = 现有规则不允许连接
//     0x03 = 网络不可达
//     0x04 = 主机不可达
//     0x05 = 连接被拒绝
//     0x06 = TTL过期
//     0x07 = 命令不支持
//     0x08 = 地址类型不支持
//     0x09-0xFF = 未分配

// SocksDialer SOCKS代理拨号器
type SocksDialer struct {
	proxyURL  string
	proxyType C.ProxyType // SOCKS4 或 SOCKS5
	Config    *C.SOCKSConfig
	metrics   *metrics.MetricsCollector
}

func createSocksDialer(proxyType C.ProxyType, proxyIP string, proxyPort int, config *C.SOCKSConfig, metrics *metrics.MetricsCollector) (ProxyDialer, error) {
	// 确保配置不为空
	if config == nil {
		config = &C.SOCKSConfig{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
	}

	proxyURL := fmt.Sprintf("%s:%d", proxyIP, proxyPort)
	dialer := NewSocksDialer(proxyURL, proxyType, config, metrics)
	return dialer, nil
}

// NewSocksDialer 创建SOCKS拨号器
func NewSocksDialer(proxyURL string, proxyType C.ProxyType, config *C.SOCKSConfig, metrics *metrics.MetricsCollector) *SocksDialer {
	// 确保配置不为空
	if config == nil {
		config = &C.SOCKSConfig{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
	}

	return &SocksDialer{
		proxyURL:  proxyURL,
		proxyType: proxyType,
		Config:    config,
		metrics:   metrics,
	}
}

// Dial 实现 ProxyDialer 接口
func (d *SocksDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// DialContext 实现SOCKS连接
func (d *SocksDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	start := time.Now()

	if d.metrics != nil {
		d.metrics.RecordConnection(0)
	}

	// 验证网络类型
	if err := d.validateNetwork(network); err != nil {
		if d.metrics != nil {
			d.metrics.RecordFailure(err)
		}
		return nil, err
	}

	// 处理UDP连接
	if network == "udp" || network == "udp4" || network == "udp6" {
		if d.proxyType != C.SOCKS5 {
			return nil, E.ErrSOCKSNetworkNotSupported
		}

		// 解析UDP地址
		raddr, err := net.ResolveUDPAddr(network, addr)
		if err != nil {
			return nil, err
		}

		// 创建UDP连接
		conn, err := d.dialUDPSocks5(network, nil, raddr)
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

	// TCP连接处理
	conn, err := d.dialWithTimeout(ctx, addr)
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

func (d *SocksDialer) validateNetwork(network string) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
		return nil
	case "udp", "udp4", "udp6":
		if !d.Config.EnableUDP {
			return E.ErrSOCKSNetworkNotSupported
		}
		return nil
	default:
		return E.ErrSOCKSNetworkNotSupported
	}
}

func (d *SocksDialer) dialWithTimeout(ctx context.Context, addr string) (net.Conn, error) {
	switch d.proxyType {
	case C.SOCKS4:
		return d.dialSocks4(ctx, addr)
	case C.SOCKS5:
		return d.dialSocks5(ctx, addr)
	default:
		return nil, E.ErrSOCKSVersionNotSupported
	}
}

func (d *SocksDialer) dialSocks4(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}

	proxyConn, err := net.DialTimeout("tcp", d.proxyURL, d.Config.Timeout)
	if err != nil {
		return nil, E.ErrSOCKSProxyUnreachable
	}

	if deadline, ok := ctx.Deadline(); ok {
		proxyConn.SetDeadline(deadline)
	}

	// SOCKS4/4a请求
	req := []byte{
		0x04,                                     // VN: SOCKS4版本
		0x01,                                     // CD: CONNECT命令
		byte(portNum >> 8), byte(portNum & 0xff), // DSTPORT
	}

	ip := net.ParseIP(host)
	if ip != nil {
		// SOCKS4: 使用IP地址
		ip4 := ip.To4()
		if ip4 == nil {
			proxyConn.Close()
			return nil, E.ErrSOCKSAddressTypeNotSupported
		}
		req = append(req, ip4...)
	} else {
		// SOCKS4a: 使用域名
		req = append(req, []byte{0, 0, 0, 1}...) // 特殊IP表示SOCKS4a
	}

	// 添加用户ID (如果有)
	if d.Config.User != "" {
		req = append(req, []byte(d.Config.User)...)
	}
	req = append(req, 0x00) // NULL结束符

	// SOCKS4a时添加域名
	if ip == nil {
		req = append(req, []byte(host)...)
		req = append(req, 0x00) // NULL结束符
	}

	// 发送请求
	if _, err := proxyConn.Write(req); err != nil {
		proxyConn.Close()
		return nil, err
	}

	// 读取响应
	resp := make([]byte, 8)
	if _, err := io.ReadFull(proxyConn, resp); err != nil {
		proxyConn.Close()
		return nil, err
	}

	// 检查响应
	if resp[1] != 0x5A {
		proxyConn.Close()
		switch resp[1] {
		case 0x5B:
			return nil, E.ErrSOCKSConnectFailed
		case 0x5C:
			return nil, E.ErrSOCKSAuthFailed
		case 0x5D:
			return nil, E.ErrSOCKSAuthFailed
		default:
			return nil, E.ErrSOCKSConnectFailed
		}
	}

	return proxyConn, nil
}

func (d *SocksDialer) dialSocks5(ctx context.Context, addr string) (net.Conn, error) {
	proxyConn, err := net.DialTimeout("tcp", d.proxyURL, d.Config.Timeout)
	if err != nil {
		return nil, E.ErrSOCKSProxyUnreachable
	}

	if deadline, ok := ctx.Deadline(); ok {
		proxyConn.SetDeadline(deadline)
	}

	// 认证协商
	methods := []byte{0x00} // 无认证
	if d.Config.User != "" && d.Config.Pass != "" {
		methods = []byte{0x02} // 用户名/密码认证
	}

	authReq := []byte{0x05, byte(len(methods))}
	authReq = append(authReq, methods...)

	if _, err := proxyConn.Write(authReq); err != nil {
		proxyConn.Close()
		return nil, err
	}

	authResp := make([]byte, 2)
	if _, err := io.ReadFull(proxyConn, authResp); err != nil {
		proxyConn.Close()
		return nil, err
	}

	if authResp[0] != 0x05 {
		proxyConn.Close()
		return nil, E.ErrSOCKSVersionNotSupported
	}

	if authResp[1] == 0x02 {
		if err := d.authenticateSocks5(proxyConn); err != nil {
			proxyConn.Close()
			return nil, err
		}
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	req := []byte{0x05, 0x01, 0x00}

	ip := net.ParseIP(host)
	if ip == nil {
		req = append(req, 0x03)
		req = append(req, byte(len(host)))
		req = append(req, []byte(host)...)
	} else if ip4 := ip.To4(); ip4 != nil {
		req = append(req, 0x01)
		req = append(req, ip4...)
	} else {
		req = append(req, 0x04)
		req = append(req, ip.To16()...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(portNum))
	req = append(req, portBytes...)

	if _, err := proxyConn.Write(req); err != nil {
		proxyConn.Close()
		return nil, err
	}

	resp := make([]byte, 4)
	if _, err := io.ReadFull(proxyConn, resp); err != nil {
		proxyConn.Close()
		return nil, err
	}

	if resp[1] != 0x00 {
		proxyConn.Close()
		return nil, E.ErrSOCKSConnectFailed
	}

	// 跳过绑定地址和端口
	switch resp[3] {
	case 0x01:
		_, err = io.CopyN(io.Discard, proxyConn, 4+2) // IPv4 + Port
	case 0x03:
		var length [1]byte
		_, err = io.ReadFull(proxyConn, length[:])
		if err == nil {
			_, err = io.CopyN(io.Discard, proxyConn, int64(length[0])+2)
		}
	case 0x04:
		_, err = io.CopyN(io.Discard, proxyConn, 16+2) // IPv6 + Port
	}

	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	return proxyConn, nil
}

func (d *SocksDialer) authenticateSocks5(conn net.Conn) error {
	username := []byte(d.Config.User)
	password := []byte(d.Config.Pass)

	req := []byte{0x01, byte(len(username))}
	req = append(req, username...)
	req = append(req, byte(len(password)))
	req = append(req, password...)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[1] != 0x00 {
		return E.ErrSOCKSAuthFailed
	}

	return nil
}

// DialUDP 创建UDP连接
func (d *SocksDialer) DialUDP(network string, laddr, raddr *net.UDPAddr) (*SocksUDPConn, error) {
	if !d.Config.EnableUDP {
		return nil, E.ErrSOCKSNetworkNotSupported
	}

	switch d.proxyType {
	case C.SOCKS5:
		return d.dialUDPSocks5(network, laddr, raddr)
	default:
		return nil, E.ErrSOCKSNetworkNotSupported
	}
}

// SocksUDPConn UDP连接封装
type SocksUDPConn struct {
	*net.UDPConn
	proxyConn  net.Conn     // TCP连接到代理服务器
	udpAddr    *net.UDPAddr // UDP中继地址
	targetAddr *net.UDPAddr // 目标地址
	closed     chan struct{}
}

// dialUDPSocks5 通过SOCKS5代理建立UDP连接
func (d *SocksDialer) dialUDPSocks5(network string, laddr, raddr *net.UDPAddr) (*SocksUDPConn, error) {
	// 1. 建立到代理服务器的TCP连接
	proxyConn, err := net.DialTimeout("tcp", d.proxyURL, d.Config.Timeout)
	if err != nil {
		return nil, err
	}

	// 2. 进行SOCKS5认证
	if err := d.authenticateSocks5(proxyConn); err != nil {
		proxyConn.Close()
		return nil, err
	}

	// 3. 发送UDP ASSOCIATE请求
	req := []byte{
		0x05,                   // VER
		0x03,                   // CMD: UDP ASSOCIATE
		0x00,                   // RSV
		0x01,                   // ATYP: IPv4
		0x00, 0x00, 0x00, 0x00, // IP: 0.0.0.0
		0x00, 0x00, // PORT: 0
	}

	if _, err := proxyConn.Write(req); err != nil {
		proxyConn.Close()
		return nil, err
	}

	// 4. 读取响应
	resp := make([]byte, 4)
	if _, err := io.ReadFull(proxyConn, resp); err != nil {
		proxyConn.Close()
		return nil, err
	}

	if resp[1] != 0x00 {
		proxyConn.Close()
		return nil, E.ErrSOCKSConnectFailed
	}

	// 5. 解析UDP中继地址
	var udpAddr *net.UDPAddr
	switch resp[3] {
	case 0x01: // IPv4
		addr := make([]byte, 4+2)
		if _, err := io.ReadFull(proxyConn, addr); err != nil {
			proxyConn.Close()
			return nil, err
		}
		udpAddr = &net.UDPAddr{
			IP:   net.IPv4(addr[0], addr[1], addr[2], addr[3]),
			Port: int(binary.BigEndian.Uint16(addr[4:])),
		}
	case 0x04: // IPv6
		addr := make([]byte, 16+2)
		if _, err := io.ReadFull(proxyConn, addr); err != nil {
			proxyConn.Close()
			return nil, err
		}
		udpAddr = &net.UDPAddr{
			IP:   addr[:16],
			Port: int(binary.BigEndian.Uint16(addr[16:])),
		}
	default:
		proxyConn.Close()
		return nil, E.ErrSOCKSAddressTypeNotSupported
	}

	// 6. 创建本地UDP连接
	udpConn, err := net.ListenUDP(network, laddr)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	return &SocksUDPConn{
		UDPConn:    udpConn,
		proxyConn:  proxyConn,
		udpAddr:    udpAddr,
		targetAddr: raddr,
		closed:     make(chan struct{}),
	}, nil
}

// Write 实现UDP写入
func (c *SocksUDPConn) Write(b []byte) (n int, err error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
		// SOCKS5 UDP请求头
		header := []byte{
			0x00, 0x00, 0x00, // RSV
			0x01, // FRAG: 0
			0x01, // ATYP: IPv4
		}
		header = append(header, c.targetAddr.IP.To4()...)
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, uint16(c.targetAddr.Port))
		header = append(header, port...)

		// 组合数据
		data := append(header, b...)
		return c.UDPConn.WriteToUDP(data, c.udpAddr)
	}
}

// Read 实现UDP读取
func (c *SocksUDPConn) Read(b []byte) (n int, err error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
		buf := make([]byte, len(b)+10+net.IPv4len+2) // 预留UDP头空间
		n, _, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return 0, err
		}

		// 跳过SOCKS5 UDP响应头
		if n < 10 {
			return 0, io.ErrShortBuffer
		}

		copy(b, buf[10:n])
		return n - 10, nil
	}
}

// Close 关闭所有连接
func (c *SocksUDPConn) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
		c.proxyConn.Close()
		return c.UDPConn.Close()
	}
}
