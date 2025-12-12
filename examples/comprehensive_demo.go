package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/vistone/netconnpool"
)

func main() {
	// 启动一个简单的 TCP 服务器用于测试
	go startTCPServer()
	// 启动一个简单的 UDP 服务器用于测试
	go startUDPServer()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	fmt.Println("=== 全面功能测试示例 ===")

	// 1. 创建配置
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 20
	config.MinConnections = 2
	config.MaxIdleConnections = 10
	config.EnableStats = true

	// 设置新的生命周期钩子
	config.OnCreated = func(conn net.Conn) error {
		log.Printf("[Hook] 新连接已创建: %s -> %s", conn.LocalAddr(), conn.RemoteAddr())
		return nil
	}

	config.OnBorrow = func(conn net.Conn) {
		log.Printf("[Hook] 连接被借出: %s", conn.RemoteAddr())
	}

	config.OnReturn = func(conn net.Conn) {
		log.Printf("[Hook] 连接已归还: %s", conn.RemoteAddr())
	}

	// 混合协议 Dialer
	// 这里模拟一个能够根据需要创建 TCP 或 UDP 连接的 Dialer
	// 在实际应用中，通常会根据业务逻辑决定连接到哪里
	// 为了演示，我们简单地交替创建 TCP 和 UDP 连接
	var counter int
	var mu sync.Mutex

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		mu.Lock()
		count := counter
		counter++
		mu.Unlock()

		if count%2 == 0 {
			// 创建 TCP 连接
			return net.DialTimeout("tcp", "127.0.0.1:8080", 5*time.Second)
		} else {
			// 创建 UDP 连接
			return net.DialTimeout("udp", "127.0.0.1:8081", 5*time.Second)
		}
	}

	// 2. 创建连接池
	pool, err := netconnpool.NewPool(config)
	if err != nil {
		log.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 3. 测试并发获取 TCP 和 UDP 连接
	var wg sync.WaitGroup
	ctx := context.Background()

	// 启动 10 个 goroutine 并发获取连接
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 随机决定获取 TCP 还是 UDP
			isTCP := id%2 == 0
			var conn *netconnpool.Connection
			var err error

			if isTCP {
				fmt.Printf("Goroutine %d 尝试获取 TCP 连接...\n", id)
				conn, err = pool.GetTCP(ctx)
			} else {
				fmt.Printf("Goroutine %d 尝试获取 UDP 连接...\n", id)
				conn, err = pool.GetUDP(ctx)
			}

			if err != nil {
				log.Printf("Goroutine %d 获取连接失败: %v", id, err)
				return
			}

			// 验证获取到的连接类型
			protocol := conn.GetProtocol()
			fmt.Printf("Goroutine %d 成功获取连接: ID=%d, Protocol=%s, Remote=%s\n",
				id, conn.ID, protocol, conn.GetConn().RemoteAddr())

			if isTCP && protocol != netconnpool.ProtocolTCP {
				log.Printf("错误: Goroutine %d 期望 TCP 但获取到了 %s", id, protocol)
			}
			if !isTCP && protocol != netconnpool.ProtocolUDP {
				log.Printf("错误: Goroutine %d 期望 UDP 但获取到了 %s", id, protocol)
			}

			// 模拟使用连接
			time.Sleep(100 * time.Millisecond)

			// 归还连接
			pool.Put(conn)
		}(i)
	}

	wg.Wait()

	// 4. 查看统计信息
	stats := pool.Stats()
	fmt.Printf("\n=== 连接池统计信息 ===\n")
	fmt.Printf("总创建连接数: %d\n", stats.TotalConnectionsCreated)
	fmt.Printf("总复用次数: %d\n", stats.TotalConnectionsReused)
	fmt.Printf("当前 TCP 连接数: %d (空闲: %d)\n", stats.CurrentTCPConnections, stats.CurrentTCPIdleConnections)
	fmt.Printf("当前 UDP 连接数: %d (空闲: %d)\n", stats.CurrentUDPConnections, stats.CurrentUDPIdleConnections)

	// 5. 测试 UDP 缓冲区清理
	fmt.Println("\n=== 测试 UDP 缓冲区清理 ===")
	// 获取一个 UDP 连接
	udpConn, err := pool.GetUDP(ctx)
	if err == nil {
		// 向服务器发送数据，服务器会回显
		conn := udpConn.GetConn()
		conn.Write([]byte("hello"))

		// 不读取响应，直接归还
		// 此时缓冲区中有数据
		fmt.Println("归还带有未读数据的 UDP 连接...")
		pool.Put(udpConn)

		// 再次获取 UDP 连接（应该是同一个，因为被复用了）
		udpConn2, _ := pool.GetUDP(ctx)
		fmt.Printf("再次获取 UDP 连接: ID=%d (复用次数: %d)\n", udpConn2.ID, udpConn2.GetReuseCount())

		// 尝试读取，应该读不到旧数据（因为被清理了）
		// 注意：这里需要设置超时，否则如果没有新数据会阻塞
		conn2 := udpConn2.GetConn()
		conn2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		buf := make([]byte, 1024)
		n, err := conn2.Read(buf)
		if err != nil {
			fmt.Printf("读取结果: %v (符合预期，缓冲区已被清理)\n", err)
		} else {
			fmt.Printf("读取到数据: %s (不符合预期，缓冲区未清理)\n", string(buf[:n]))
		}
		pool.Put(udpConn2)
	}
}

func startTCPServer() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Printf("TCP Server error: %v", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			// 简单回显
			io.Copy(c, c)
		}(conn)
	}
}

func startUDPServer() {
	addr, _ := net.ResolveUDPAddr("udp", ":8081")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("UDP Server error: %v", err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		// 回显
		conn.WriteToUDP(buf[:n], remote)
	}
}
