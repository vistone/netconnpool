package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/vistone/netconnpool"
)

func main() {
	fmt.Println("=== TLS连接池示例 ===")

	// 创建TLS连接池配置
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 10
	config.MinConnections = 2
	config.ConnectionTimeout = 10 * time.Second

	// 配置TLS Dialer
	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		// 创建TCP连接
		tcpConn, err := net.DialTimeout("tcp", "example.com:443", 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("TCP连接失败: %w", err)
		}

		// 配置TLS客户端
		tlsConfig := &tls.Config{
			ServerName:         "example.com",    // SNI（服务器名称指示）
			InsecureSkipVerify: false,            // 在生产环境中应该为false，验证服务器证书
			MinVersion:         tls.VersionTLS12, // 最小TLS版本
		}

		// 执行TLS握手，将TCP连接升级为TLS连接
		tlsConn := tls.Client(tcpConn, tlsConfig)

		// 执行TLS握手（必须在返回前完成）
		handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// 使用goroutine执行握手，以便支持超时
		handshakeDone := make(chan error, 1)
		go func() {
			handshakeDone <- tlsConn.Handshake()
		}()

		select {
		case err := <-handshakeDone:
			if err != nil {
				tcpConn.Close() // 握手失败，关闭TCP连接
				return nil, fmt.Errorf("TLS握手失败: %w", err)
			}
		case <-handshakeCtx.Done():
			tcpConn.Close() // 超时，关闭TCP连接
			return nil, fmt.Errorf("TLS握手超时: %w", handshakeCtx.Err())
		}

		fmt.Printf("TLS连接建立成功: %s\n", tlsConn.ConnectionState().ServerName)
		return tlsConn, nil
	}

	// 自定义健康检查（TLS连接）
	config.HealthChecker = func(conn net.Conn) bool {
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			return false
		}

		// 检查连接状态
		state := tlsConn.ConnectionState()
		if !state.HandshakeComplete {
			return false
		}

		// 使用SetReadDeadline进行简单的心跳检测
		tlsConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		one := make([]byte, 1)
		_, err := tlsConn.Read(one)
		tlsConn.SetReadDeadline(time.Time{})

		// 超时表示连接还活着（没有数据但连接正常）
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}

		// EOF或其他错误表示连接已关闭
		return false
	}

	// 创建连接池
	pool, err := netconnpool.NewPool(config)
	if err != nil {
		log.Fatalf("创建TLS连接池失败: %v", err)
	}
	defer pool.Close()

	fmt.Println("TLS连接池创建成功")

	// 使用连接池
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := pool.Get(ctx)
		cancel()

		if err != nil {
			log.Printf("获取连接失败: %v", err)
			continue
		}

		// 使用TLS连接
		tlsConn := conn.GetConn().(*tls.Conn)
		state := tlsConn.ConnectionState()
		fmt.Printf("连接 %d: TLS版本=%s, 服务器=%s, 复用次数=%d\n",
			conn.ID,
			tlsVersionString(state.Version),
			state.ServerName,
			conn.GetReuseCount())

		// 归还连接
		if err := pool.Put(conn); err != nil {
			log.Printf("归还连接失败: %v", err)
		}
	}

	// 打印统计信息
	stats := pool.Stats()
	fmt.Printf("\n=== TLS连接池统计 ===\n")
	fmt.Printf("当前连接数: %d\n", stats.CurrentConnections)
	fmt.Printf("累计创建: %d\n", stats.TotalConnectionsCreated)
	fmt.Printf("累计复用: %d\n", stats.TotalConnectionsReused)
	if stats.TotalConnectionsCreated > 0 {
		reuseRate := float64(stats.TotalConnectionsReused) / float64(stats.TotalConnectionsCreated+stats.TotalConnectionsReused) * 100
		fmt.Printf("连接复用率: %.2f%%\n", reuseRate)
	}
}

// tlsVersionString 将TLS版本号转换为字符串
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("未知版本 (0x%04x)", version)
	}
}
