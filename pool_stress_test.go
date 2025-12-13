package netconnpool

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPool_StressTest 压力测试
func TestPool_StressTest(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 100
	config.MinConnections = 20
	config.MaxIdleConnections = 50
	config.ConnectionTimeout = 2 * time.Second
	config.GetConnectionTimeout = 2 * time.Second
	config.EnableHealthCheck = true
	config.EnableStats = true

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	concurrency := 200
	iterations := 50
	var successCount int64
	var errorCount int64

	start := time.Now()

	// 高并发压力测试
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				conn, err := pool.Get(ctx)
				cancel()
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}
				atomic.AddInt64(&successCount, 1)
				time.Sleep(1 * time.Millisecond)
				pool.Put(conn)
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	stats := pool.Stats()
	t.Logf("压力测试结果:")
	t.Logf("  并发数: %d", concurrency)
	t.Logf("  每并发迭代次数: %d", iterations)
	t.Logf("  总请求数: %d", concurrency*iterations)
	t.Logf("  成功数: %d", successCount)
	t.Logf("  失败数: %d", errorCount)
	t.Logf("  耗时: %v", duration)
	t.Logf("  QPS: %.2f", float64(successCount)/duration.Seconds())
	t.Logf("  当前连接数: %d", stats.CurrentConnections)
	t.Logf("  空闲连接数: %d", stats.CurrentIdleConnections)
	t.Logf("  活跃连接数: %d", stats.CurrentActiveConnections)
	t.Logf("  连接复用次数: %d", stats.TotalConnectionsReused)
	t.Logf("  平均复用率: %.2f%%", float64(stats.TotalConnectionsReused)/float64(stats.SuccessfulGets)*100)

	if successCount == 0 {
		t.Error("应该有成功的请求")
	}
}

// TestPool_MemoryLeak 测试内存泄漏
func TestPool_MemoryLeak(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 50
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 执行多轮操作
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
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
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
	}

	stats := pool.Stats()
	if stats.CurrentConnections > int64(config.MaxConnections) {
		t.Errorf("当前连接数 %d 不应该超过最大连接数 %d", stats.CurrentConnections, config.MaxConnections)
	}
}

// TestPool_TimeoutHandling 测试超时处理
func TestPool_TimeoutHandling(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 5
	config.ConnectionTimeout = 100 * time.Millisecond
	config.GetConnectionTimeout = 100 * time.Millisecond

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		// 模拟慢连接
		time.Sleep(50 * time.Millisecond)
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 快速获取连接，填满连接池
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get(context.Background())
			if err != nil {
				return
			}
			// 持有连接一段时间
			time.Sleep(200 * time.Millisecond)
			pool.Put(conn)
		}()
	}

	// 等待连接池填满
	time.Sleep(100 * time.Millisecond)

	// 尝试获取连接（应该超时）
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	conn, err := pool.GetWithTimeout(ctx, 50*time.Millisecond)
	cancel()

	if err == nil {
		if conn != nil {
			pool.Put(conn)
		}
		t.Error("应该超时")
	}

	wg.Wait()
}

// TestPool_MaxConnectionsLimit 测试最大连接数限制
func TestPool_MaxConnectionsLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	var createdCount int64
	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		atomic.AddInt64(&createdCount, 1)
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 创建超过最大连接数的连接
	conns := make([]*Connection, 0, 15)
	for i := 0; i < 15; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil {
			if err == ErrMaxConnectionsReached || err == ErrGetConnectionTimeout {
				break
			}
			t.Errorf("获取连接失败: %v", err)
			break
		}
		conns = append(conns, conn)
	}

	created := atomic.LoadInt64(&createdCount)
	if created > int64(config.MaxConnections) {
		t.Errorf("创建的连接数 %d 不应该超过最大连接数 %d", created, config.MaxConnections)
	}

	// 归还连接
	for _, conn := range conns {
		pool.Put(conn)
	}
}

// TestPool_GetWithProtocol_Stress 测试协议获取压力测试
func TestPool_GetWithProtocol_Stress(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 50
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	concurrency := 50

	// 并发获取TCP连接
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				conn, err := pool.GetTCP(context.Background())
				if err != nil {
					return
				}
				pool.Put(conn)
			}
		}()
	}

	wg.Wait()

	stats := pool.Stats()
	if stats.CurrentTCPConnections == 0 {
		t.Error("应该有TCP连接")
	}
}

// TestPool_GetWithIPVersion_Stress 测试IP版本获取压力测试
func TestPool_GetWithIPVersion_Stress(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 50
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	concurrency := 50

	// 并发获取IPv4连接
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				conn, err := pool.GetIPv4(context.Background())
				if err != nil {
					return
				}
				pool.Put(conn)
			}
		}()
	}

	wg.Wait()

	stats := pool.Stats()
	if stats.CurrentIPv4Connections == 0 {
		t.Error("应该有IPv4连接")
	}
}
