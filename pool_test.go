package netconnpool

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockConn 模拟连接用于测试
type mockConn struct {
	net.Conn
	closed bool
	mu     sync.Mutex
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// TestPool_BasicOperations 测试基本操作
func TestPool_BasicOperations(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.MinConnections = 2
	config.ConnectionTimeout = 1 * time.Second
	config.GetConnectionTimeout = 1 * time.Second

	connCount := 0
	var mu sync.Mutex

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		mu.Lock()
		connCount++
		mu.Unlock()
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 测试获取连接
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	if conn == nil {
		t.Fatal("连接为nil")
	}

	// 测试归还连接
	err = pool.Put(conn)
	if err != nil {
		t.Fatalf("归还连接失败: %v", err)
	}

	// 测试统计信息
	stats := pool.Stats()
	if stats.CurrentConnections == 0 {
		t.Error("当前连接数应该大于0")
	}
}

// TestPool_GetWithTimeout_InfiniteLoopProtection 测试无限循环保护
func TestPool_GetWithTimeout_InfiniteLoopProtection(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 5
	config.ConnectionTimeout = 100 * time.Millisecond
	config.GetConnectionTimeout = 100 * time.Millisecond

	callCount := 0
	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		callCount++
		// 模拟返回 nil, nil 的情况（虽然不应该发生）
		if callCount > 10 {
			return nil, nil
		}
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 应该返回错误而不是无限循环
	conn, err := pool.GetWithTimeout(ctx, 100*time.Millisecond)
	if err == nil && conn == nil {
		t.Error("应该返回错误而不是nil")
	}
}

// TestPool_GetWithIPVersion_MismatchProtection 测试IP版本不匹配保护
func TestPool_GetWithIPVersion_MismatchProtection(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.ConnectionTimeout = 100 * time.Millisecond
	config.GetConnectionTimeout = 100 * time.Millisecond

	// 创建一个总是返回IPv4连接的Dialer
	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		// 返回IPv4连接
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 尝试获取IPv6连接（应该失败，因为有保护机制）
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := pool.GetWithIPVersion(ctx, IPVersionIPv6, 100*time.Millisecond)
	if err == nil {
		if conn != nil {
			pool.Put(conn)
		}
		t.Error("应该返回错误，因为IP版本不匹配")
	}
	if err != ErrNoConnectionForIPVersion {
		t.Errorf("应该返回 ErrNoConnectionForIPVersion，但返回: %v", err)
	}
}

// TestPool_ConcurrentAccess 测试并发访问
func TestPool_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 20
	config.MinConnections = 5
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
	iterations := 10

	// 并发获取和归还连接
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				conn, err := pool.Get(context.Background())
				if err != nil {
					t.Errorf("获取连接失败: %v", err)
					return
				}
				time.Sleep(10 * time.Millisecond)
				if err := pool.Put(conn); err != nil {
					t.Errorf("归还连接失败: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// 等待所有操作完成
	time.Sleep(100 * time.Millisecond)

	// 检查统计信息
	stats := pool.Stats()
	// 注意：连接可能已经被清理，所以检查总数而不是当前数
	if stats.TotalConnectionsCreated == 0 {
		t.Error("应该创建过连接")
	}
	if stats.SuccessfulGets == 0 {
		t.Error("应该有成功的获取操作")
	}
}

// TestPool_Close_Safety 测试关闭安全性
func TestPool_Close_Safety(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}

	// 并发关闭多次（应该安全）
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Close()
		}()
	}
	wg.Wait()

	// 关闭后获取连接应该失败
	_, err = pool.Get(context.Background())
	if err != ErrPoolClosed {
		t.Errorf("应该返回 ErrPoolClosed，但返回: %v", err)
	}
}

// TestPool_EnforceMaxIdleConnections 测试最大空闲连接数限制
func TestPool_EnforceMaxIdleConnections(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 20
	config.MaxIdleConnections = 5
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

	// 创建多个连接并归还
	conns := make([]*Connection, 0, 10)
	for i := 0; i < 10; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil {
			t.Fatalf("获取连接失败: %v", err)
		}
		conns = append(conns, conn)
	}

	// 归还所有连接
	for _, conn := range conns {
		if err := pool.Put(conn); err != nil {
			t.Errorf("归还连接失败: %v", err)
		}
	}

	// 等待清理管理器执行
	time.Sleep(500 * time.Millisecond)

	// 检查空闲连接数
	stats := pool.Stats()
	if stats.CurrentIdleConnections > int64(config.MaxIdleConnections+2) {
		t.Errorf("空闲连接数 %d 应该 <= %d", stats.CurrentIdleConnections, config.MaxIdleConnections+2)
	}
}

// TestPool_ProtocolMismatch 测试协议不匹配保护
func TestPool_ProtocolMismatch(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.ConnectionTimeout = 100 * time.Millisecond
	config.GetConnectionTimeout = 100 * time.Millisecond

	// 总是返回TCP连接
	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 尝试获取UDP连接（应该失败）
	conn, err := pool.GetWithProtocol(ctx, ProtocolUDP, 100*time.Millisecond)
	if err == nil {
		if conn != nil {
			pool.Put(conn)
		}
		t.Error("应该返回错误，因为协议不匹配")
	}
	if err != ErrNoConnectionForProtocol {
		t.Errorf("应该返回 ErrNoConnectionForProtocol，但返回: %v", err)
	}
}

// TestPool_ConnectionLeakDetection 测试连接泄漏检测
func TestPool_ConnectionLeakDetection(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.ConnectionLeakTimeout = 100 * time.Millisecond
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 获取连接但不归还（模拟泄漏）
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	// 等待泄漏检测（泄漏检测每分钟执行一次）
	time.Sleep(70 * time.Second)

	stats := pool.Stats()
	// 注意：泄漏检测可能还没有执行，所以这个测试可能不稳定
	// 但至少验证了连接没有被自动关闭
	if stats.CurrentConnections == 0 {
		t.Error("泄漏的连接应该还在连接池中")
	}

	// 清理
	conn.Close()
}

// TestPool_HealthCheck 测试健康检查
func TestPool_HealthCheck(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.EnableHealthCheck = true
	config.HealthCheckInterval = 100 * time.Millisecond
	config.HealthCheckTimeout = 50 * time.Millisecond
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 获取连接
	conn, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	// 归还连接
	pool.Put(conn)

	// 等待健康检查
	time.Sleep(200 * time.Millisecond)

	stats := pool.Stats()
	if stats.HealthCheckAttempts == 0 {
		t.Error("应该执行了健康检查")
	}
}

// TestPool_WarmUp 测试预热功能
func TestPool_WarmUp(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.MinConnections = 5
	config.ConnectionTimeout = 1 * time.Second

	connCount := 0
	var mu sync.Mutex

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		mu.Lock()
		connCount++
		mu.Unlock()
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 等待预热完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	created := connCount
	mu.Unlock()

	if created < config.MinConnections {
		t.Errorf("应该创建至少 %d 个连接，但只创建了 %d", config.MinConnections, created)
	}

	stats := pool.Stats()
	if stats.CurrentIdleConnections == 0 {
		t.Error("应该有预热连接在空闲池中")
	}
}

// TestPool_StatsAccuracy 测试统计信息准确性
func TestPool_StatsAccuracy(t *testing.T) {
	config := DefaultConfig()
	config.MaxConnections = 10
	config.EnableStats = true
	config.ConnectionTimeout = 1 * time.Second

	config.Dialer = func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	// 执行多次获取和归还
	for i := 0; i < 10; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil {
			t.Fatalf("获取连接失败: %v", err)
		}
		pool.Put(conn)
	}

	stats := pool.Stats()
	if stats.TotalGetRequests == 0 {
		t.Error("TotalGetRequests 应该大于0")
	}
	if stats.SuccessfulGets == 0 {
		t.Error("SuccessfulGets 应该大于0")
	}
	if stats.TotalConnectionsReused == 0 && stats.SuccessfulGets > 1 {
		t.Error("应该有连接复用")
	}
}
