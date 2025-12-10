package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/vistone/netconnpool"
)

func main() {
	// 创建支持IPv6的配置
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 20
	config.MinConnections = 2
	config.EnableStats = true

	// IPv6连接创建函数
	config.Dialer = func(ctx context.Context) (any, error) {
		// 连接到IPv6地址（使用::1表示本地IPv6地址）
		return net.DialTimeout("tcp6", "[::1]:8080", 5*time.Second)
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		log.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	fmt.Println("=== IPv6连接池示例 ===")

	// 获取IPv6连接
	ctx := context.Background()
	conn, err := pool.GetIPv6(ctx)
	if err != nil {
		log.Printf("获取IPv6连接失败: %v", err)
	} else {
		fmt.Printf("成功获取IPv6连接: %s\n", conn.GetIPVersion().String())
		if netConn, ok := conn.GetConn().(net.Conn); ok {
			fmt.Printf("远程地址: %s\n", netConn.RemoteAddr())
		}
		pool.Put(conn)
	}

	// 查看统计信息
	stats := pool.Stats()
	fmt.Printf("\n连接池统计信息:\n")
	fmt.Printf("  当前IPv4连接数: %d\n", stats.CurrentIPv4Connections)
	fmt.Printf("  当前IPv6连接数: %d\n", stats.CurrentIPv6Connections)
	fmt.Printf("  当前IPv4空闲连接数: %d\n", stats.CurrentIPv4IdleConnections)
	fmt.Printf("  当前IPv6空闲连接数: %d\n", stats.CurrentIPv6IdleConnections)
}

