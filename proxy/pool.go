package proxy

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ba0gu0/GoHookProxy/metrics"
)

// ConnPool 连接池
type ConnPool struct {
	mu sync.RWMutex
	// 空闲连接映射
	idle map[string][]connWithExpiry
	// 活跃连接计数
	active int64
	// 配置项
	maxIdle     int           // 每个目标地址的最大空闲连接数
	maxActive   int           // 最大总连接数(活跃+空闲)
	idleTimeout time.Duration // 空闲连接超时时间
	// 指标收集
	metrics *metrics.MetricsCollector
}

// connWithExpiry 带过期时间的连接
type connWithExpiry struct {
	conn       net.Conn
	expiryTime time.Time
}

// NewConnPool 创建连接池
func NewConnPool(maxIdle, maxTotal int, idleTimeout time.Duration, metrics *metrics.MetricsCollector) *ConnPool {
	return &ConnPool{
		idle:        make(map[string][]connWithExpiry),
		maxIdle:     maxIdle,
		maxActive:   maxTotal,
		idleTimeout: idleTimeout,
		metrics:     metrics,
	}
}

// Get 从连接池获取连接
func (p *ConnPool) Get(network, addr string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := network + ":" + addr

	// 检查是否达到最大连接数限制
	if p.active >= int64(p.maxActive) {
		if p.metrics != nil {
			p.metrics.RecordErrorType(errors.New("connection pool: max active connections reached"))
		}
		return nil, errors.New("connection pool: max active connections reached")
	}

	// 尝试复用空闲连接
	if conns := p.idle[key]; len(conns) > 0 {
		// 从后向前遍历,这样移除元素更高效
		for i := len(conns) - 1; i >= 0; i-- {
			connExp := conns[i]

			// 检查是否过期
			if time.Now().After(connExp.expiryTime) {
				// 移除并关闭过期连接
				p.removeConn(key, i)
				continue
			}

			// 检查连接是否可用
			if isConnAlive(connExp.conn) {
				// 移除该连接并更新计数
				p.idle[key] = append(conns[:i], conns[i+1:]...)
				p.active++

				if p.metrics != nil {
					p.metrics.IncrementActiveConnections()
				}

				// 记录活跃连接数
				atomic.AddInt64(&p.active, 1)
				if p.metrics != nil {
					p.metrics.RecordConnection(0)
				}

				return connExp.conn, nil
			}

			// 连接不可用,移除并关闭
			p.removeConn(key, i)
		}
	}

	return nil, nil
}

// removeConn 移除指定位置的连接
func (p *ConnPool) removeConn(key string, index int) {
	conns := p.idle[key]
	conn := conns[index].conn
	// 更新空闲连接列表
	p.idle[key] = append(conns[:index], conns[index+1:]...)
	// 关闭连接
	conn.Close()
}

// Put 将连接放回连接池
func (p *ConnPool) Put(network, addr string, conn net.Conn) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	key := network + ":" + addr
	conns := p.idle[key]

	// 检查是否超过每个地址的最大空闲连接数
	if len(conns) >= p.maxIdle {
		conn.Close()
		p.active--
		if p.metrics != nil {
			p.metrics.DecrementActiveConnections()
		}
		return
	}

	// 添加到空闲连接池
	p.idle[key] = append(conns, connWithExpiry{
		conn:       conn,
		expiryTime: time.Now().Add(p.idleTimeout),
	})
	p.active--

	if p.metrics != nil {
		p.metrics.DecrementActiveConnections()
	}
}

// CleanUp 清理过期和失效的连接
func (p *ConnPool) CleanUp() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cleaned := 0

	// 清理空闲连接
	for key, conns := range p.idle {
		var alive []connWithExpiry
		for _, connExp := range conns {
			if now.After(connExp.expiryTime) {
				connExp.conn.Close()
				atomic.AddInt64(&p.active, -1)
				cleaned++
				continue
			}

			// 验证连接是否真的活着
			if !isConnAlive(connExp.conn) {
				connExp.conn.Close()
				atomic.AddInt64(&p.active, -1)
				cleaned++
				continue
			}

			alive = append(alive, connExp)
		}

		if len(alive) == 0 {
			delete(p.idle, key)
		} else {
			p.idle[key] = alive
		}
	}

	// 记录清理数量
	if cleaned > 0 {
		log.Printf("Cleaned %d expired/dead connections", cleaned)
	}
}

// StartCleanup 启动定期清理
func (p *ConnPool) StartCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			p.CleanUp()
		}
	}()
}

// isConnAlive 检查连接是否可用
func isConnAlive(conn net.Conn) bool {
	if conn == nil {
		return false
	}

	// 设置1秒超时
	err := conn.SetReadDeadline(time.Now().Add(time.Second))
	if err != nil {
		return false
	}

	// 使用一个字节的缓冲区
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		if err == io.EOF {
			return false
		}
		// 如果是超时错误，说明连接还活着
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 重置读取超时
			conn.SetReadDeadline(time.Time{})
			return true
		}
		return false
	}

	// 重置读取超时
	conn.SetReadDeadline(time.Time{})
	return true
}

// poolConn 包装原始连接
type poolConn struct {
	net.Conn
	network string
	addr    string
	pool    *ConnPool
	once    sync.Once
}

// Close 重置关闭方法,实现连接复用
func (c *poolConn) Close() error {
	c.once.Do(func() {
		if isConnAlive(c.Conn) {
			c.pool.Put(c.network, c.addr, c.Conn)
		} else {
			c.Conn.Close()
		}
	})
	return nil
}

// Stats 连接池统计信息
type Stats struct {
	ActiveCount  int64
	IdleCount    int64
	TotalCount   int64
	IdleTimeout  time.Duration
	MaxIdleConns int
	MaxActive    int
}

// Stats 返回连接池统计信息
func (p *ConnPool) Stats() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var idleCount int64
	for _, conns := range p.idle {
		idleCount += int64(len(conns))
	}

	return Stats{
		ActiveCount:  p.active,
		IdleCount:    idleCount,
		TotalCount:   p.active + idleCount,
		IdleTimeout:  p.idleTimeout,
		MaxIdleConns: p.maxIdle,
		MaxActive:    p.maxActive,
	}
}
