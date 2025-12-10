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

func mainServer() {
	fmt.Println("=== TLS服务器端连接池示例 ===")

	// 注意：这只是一个演示示例，生产环境需要真实的证书和私钥
	// 可以使用 self-signed 证书进行测试

	// 加载TLS证书（示例，实际使用时需要真实的证书文件）
	// cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	// if err != nil {
	//     log.Fatalf("加载证书失败: %v", err)
	// }

	// 为演示目的，创建一个自签名证书配置
	// 实际生产环境应该使用真实的证书
	tlsConfig := &tls.Config{
		// Certificates: []tls.Certificate{cert}, // 实际使用时取消注释
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			// 返回服务器证书
			// 实际使用时应该从配置中加载证书
			log.Printf("客户端请求连接: %s", hello.ServerName)
			// 这里应该返回真实的证书
			return nil, fmt.Errorf("未实现证书加载")
		},
		MinVersion: tls.VersionTLS12,
	}

	// 创建TLS监听器
	listener, err := tls.Listen("tcp", ":8443", tlsConfig)
	if err != nil {
		log.Fatalf("创建TLS监听器失败: %v", err)
	}
	defer listener.Close()

	fmt.Println("TLS服务器监听在 :8443")

	// 创建服务器端连接池配置
	config := netconnpool.DefaultServerConfig()
	config.Listener = listener
	config.MaxConnections = 100

	// 自定义Acceptor（可选，默认即可使用）
	config.Acceptor = func(ctx context.Context, l net.Listener) (any, error) {
		// 对于TLS服务器，Listener已经是tls.Listener
		// 直接Accept即可获得TLS连接
		conn, err := l.Accept()
		if err != nil {
			return nil, err
		}

		// 类型断言为TLS连接
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			conn.Close()
			return nil, fmt.Errorf("接受的连接不是TLS连接")
		}

		// TLS握手在Accept时自动完成
		state := tlsConn.ConnectionState()
		fmt.Printf("接受TLS连接: %s, TLS版本=%s\n",
			tlsConn.RemoteAddr(),
			tlsVersionStringServer(state.Version))

		return tlsConn, nil
	}

	// 自定义健康检查
	config.HealthChecker = func(conn any) bool {
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			return false
		}

		state := tlsConn.ConnectionState()
		if !state.HandshakeComplete {
			return false
		}

		// 简单的心跳检测
		tlsConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		one := make([]byte, 1)
		_, err := tlsConn.Read(one)
		tlsConn.SetReadDeadline(time.Time{})

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}

		return false
	}

	// 创建连接池
	pool, err := netconnpool.NewPool(config)
	if err != nil {
		log.Fatalf("创建TLS服务器连接池失败: %v", err)
	}
	defer pool.Close()

	fmt.Println("TLS服务器连接池创建成功")
	fmt.Println("等待客户端连接...")

	// 服务器运行循环
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, err := pool.Get(ctx)
		cancel()

		if err != nil {
			log.Printf("获取连接失败: %v", err)
			continue
		}

		// 处理连接（在实际应用中，这里应该启动goroutine处理）
		go handleTLSConnection(conn, pool)
	}
}

func handleTLSConnection(conn *netconnpool.Connection, pool *netconnpool.Pool) {
	defer pool.Put(conn)

	tlsConn := conn.GetConn().(*tls.Conn)
	buf := make([]byte, 1024)

	for {
		tlsConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := tlsConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("读取数据失败: %v", err)
			return
		}

		// 回显数据
		_, err = tlsConn.Write(buf[:n])
		if err != nil {
			log.Printf("写入数据失败: %v", err)
			return
		}
	}
}

func tlsVersionStringServer(version uint16) string {
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

