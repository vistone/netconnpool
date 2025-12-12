package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/vistone/netconnpool"
)

func main() {
	fmt.Println("=== TLS服务器端连接池示例 ===")

	// 生成自签名证书
	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("生成证书失败: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// 创建TLS监听器
	listener, err := tls.Listen("tcp", "127.0.0.1:8443", tlsConfig)
	if err != nil {
		log.Fatalf("创建TLS监听器失败: %v", err)
	}
	defer listener.Close()

	fmt.Println("TLS服务器监听在 127.0.0.1:8443")

	// 创建服务器端连接池配置
	config := netconnpool.DefaultServerConfig()
	config.Listener = listener
	config.MaxConnections = 100

	// 自定义Acceptor（可选，默认即可使用）
	config.Acceptor = func(ctx context.Context, l net.Listener) (net.Conn, error) {
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

		// TLS握手在Accept时自动完成（或者在第一次Read/Write时）
		// 这里我们强制握手以获取连接状态
		// 注意：在实际高并发场景中，最好不要在Acceptor中做耗时操作
		// 这里为了演示目的
		// err = tlsConn.Handshake()
		// 实际上 tls.Listener.Accept() 返回的连接在 Read/Write 时会自动握手
		// 如果想立即握手，可以在这里调用，但要注意超时

		return tlsConn, nil
	}

	// 自定义健康检查
	config.HealthChecker = func(conn net.Conn) bool {
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			return false
		}

		state := tlsConn.ConnectionState()
		if !state.HandshakeComplete {
			// 如果还没握手，可能是不健康的，或者还没开始使用
			// 对于服务器端被动接受的连接，如果还没握手完成，可能不应该视为健康
			// 但这里简单起见，如果没握手，我们认为它还在初始化中，或者...
			// 实际上，服务器端连接池通常用于复用已经建立的连接
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

	// 启动客户端测试
	go runClient(cert)

	// 服务器运行循环
	// 运行一段时间后退出，以便测试结束
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("测试结束")
			return
		default:
			// 非阻塞尝试获取连接（模拟服务器处理循环）
			// 注意：Get() 在服务器模式下会阻塞等待新连接
			// 我们使用带超时的Get
			getCtx, getCancel := context.WithTimeout(context.Background(), 1*time.Second)
			conn, err := pool.Get(getCtx)
			getCancel()

			if err != nil {
				// log.Printf("获取连接超时/失败: %v", err)
				continue
			}

			// 处理连接
			go handleTLSConnection(conn, pool)
		}
	}
}

func handleTLSConnection(conn *netconnpool.Connection, pool *netconnpool.Pool) {
	defer pool.Put(conn)

	tlsConn := conn.GetConn().(*tls.Conn)
	buf := make([]byte, 1024)

	tlsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := tlsConn.Read(buf)
	if err != nil {
		// 客户端关闭或超时
		return
	}

	// 回显数据
	tlsConn.Write(buf[:n])
	fmt.Printf("服务器收到并回显: %s\n", string(buf[:n]))
}

func runClient(serverCert tls.Certificate) {
	time.Sleep(1 * time.Second) // 等待服务器启动

	// 客户端配置，信任服务器证书
	// 由于是自签名，我们需要将证书添加到RootCAs，或者InsecureSkipVerify
	// 这里简单起见使用 InsecureSkipVerify，因为我们知道是自签名的
	clientConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", "127.0.0.1:8443", clientConfig)
	if err != nil {
		log.Printf("客户端连接失败: %v", err)
		return
	}
	defer conn.Close()

	msg := "Hello TLS"
	conn.Write([]byte(msg))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("客户端读取失败: %v", err)
		return
	}
	fmt.Printf("客户端收到: %s\n", string(buf[:n]))
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

// 生成自签名证书
func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}
