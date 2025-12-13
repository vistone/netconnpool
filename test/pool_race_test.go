package netconnpool_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/vistone/netconnpool"
)

// TestPool_RaceCondition 使用race detector测试竞态条件
func TestPool_RaceCondition(t *testing.T) {
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 50
	config.MinConnections = 10
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second
	config.EnableHealthCheck = true
	config.EnableStats = true

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	concurrency := 100

	// 并发获取连接
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				conn, err := pool.Get(context.Background())
				if err != nil {
					return
				}
				// 模拟使用连接
				time.Sleep(1 * time.Millisecond)
				pool.Put(conn)
			}
		}(i)
	}

	// 并发获取统计信息
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				stats := pool.Stats()
				_ = stats
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

// TestPool_ConcurrentClose 测试并发关闭
func TestPool_ConcurrentClose(t *testing.T) {
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 20
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}

	var wg sync.WaitGroup

	// 并发获取连接
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get(context.Background())
			if err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			pool.Put(conn)
		}()
	}

	// 并发关闭
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Close()
		}()
	}

	wg.Wait()
}

// TestPool_ConcurrentGetPut 测试并发获取和归还
func TestPool_ConcurrentGetPut(t *testing.T) {
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 30
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	concurrency := 50

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				conn, err := pool.Get(context.Background())
				if err != nil {
					return
				}
				pool.Put(conn)
			}
		}()
	}

	wg.Wait()

	// 验证统计信息
	stats := pool.Stats()
	if stats.CurrentConnections == 0 {
		t.Error("应该有连接存在")
	}
}

// TestPool_ConnectionStateRace 测试连接状态竞态条件
func TestPool_ConnectionStateRace(t *testing.T) {
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 20
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	var wg sync.WaitGroup

	// 并发访问连接状态
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.IsInUse()
			conn.GetHealthStatus()
			conn.GetAge()
			conn.GetIdleTime()
			conn.GetReuseCount()
		}()
	}

	wg.Wait()
	pool.Put(conn)
}

// TestPool_StatsRace 测试统计信息竞态条件
func TestPool_StatsRace(t *testing.T) {
	config := netconnpool.DefaultConfig()
	config.MaxConnections = 20
	config.EnableStats = true
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := netconnpool.NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup

	// 并发操作和读取统计信息
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get(context.Background())
			if err != nil {
				return
			}
			stats := pool.Stats()
			_ = stats
			pool.Put(conn)
		}()
	}

	wg.Wait()
}
