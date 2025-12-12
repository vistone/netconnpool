# NetConnPool - 网络连接池管理库

[![Go Reference](https://pkg.go.dev/badge/github.com/vistone/netconnpool.svg)](https://pkg.go.dev/github.com/vistone/netconnpool)
[![Go Report Card](https://goreportcard.com/badge/github.com/vistone/netconnpool)](https://goreportcard.com/report/github.com/vistone/netconnpool)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

一个功能全面、高性能的 Go 语言网络连接池管理库，提供了完善的连接生命周期管理、健康检查、统计监控等功能。

**核心特性**：
- 🚀 **高性能**：连接复用率 > 95%，显著提升性能
- 🔒 **并发安全**：完全线程安全，支持高并发场景
- 🎯 **灵活配置**：支持客户端/服务器端两种模式
- 📊 **详细统计**：提供丰富的统计信息，便于监控和优化
- 🛡️ **自动管理**：健康检查、泄漏检测、自动清理
- 🌐 **协议支持**：支持TCP/UDP，IPv4/IPv6
- 🔄 **智能空闲池**：TCP/UDP 独立空闲池，避免协议混淆带来的性能抖动
- 🪝 **生命周期钩子**：支持 Created/Borrow/Return 阶段的自定义回调

## 目录

- [特性](#特性)
- [安装](#安装)
- [快速开始](#快速开始)
- [使用指南](#使用指南)
- [工作流程](#工作流程)
- [功能要点](#功能要点)
- [配置选项](#配置选项)
- [高级功能](#高级功能)
- [注意事项](#注意事项)
- [错误处理](#错误处理)
- [性能优化](#性能优化)
- [架构设计](#架构设计)
- [示例代码](#示例代码)
- [测试](#测试)

## 特性

- ✅ **客户端/服务器端模式**：支持客户端主动连接和服务器端被动接受两种模式
- ✅ **完整的连接生命周期管理**：创建、获取、归还、销毁
- ✅ **连接复用机制**：自动复用已建立的连接，显著提升性能
- ✅ **TCP/UDP 协议支持**：自动识别和区分TCP/UDP连接，支持指定协议类型获取连接
- ✅ **IPv4/IPv6 支持**：自动识别和区分IPv4/IPv6连接，支持指定IP版本获取连接
- ✅ **UDP缓冲区管理**：自动清理UDP读取缓冲区，防止数据混淆
- ✅ **灵活的连接池配置**：最大/最小连接数、空闲连接数等
- ✅ **连接健康检查**：自动检测和清理不健康的连接
- ✅ **连接泄漏检测**：及时发现长时间未归还的连接
- ✅ **连接超时管理**：创建超时、空闲超时、生命周期限制
- ✅ **统计和监控**：详细的连接池使用统计信息（包括TCP/UDP和IPv4/IPv6分别统计，连接复用统计）
- ✅ **并发安全**：完全支持并发访问
- ✅ **优雅关闭**：支持连接池的优雅关闭和资源清理
- ✅ **等待队列**：连接池耗尽时自动排队等待（支持TCP/UDP和IPv4/IPv6分别排队）
- ✅ **类型安全**：基于 `net.Conn` 接口，提供更强的类型检查

## 安装

```bash
go get github.com/vistone/netconnpool
```

## 快速开始

### 客户端模式（默认）

客户端模式用于主动连接到服务器的场景，适用于HTTP客户端、数据库客户端、RPC客户端等。

```go
package main

import (
    "context"
    "net"
    "time"
    "github.com/vistone/netconnpool"
)

func main() {
    // 创建客户端连接池配置
    config := netconnpool.DefaultConfig()
    config.MaxConnections = 10
    config.MinConnections = 2 // 预热2个连接
    
    // 设置连接创建函数
    config.Dialer = func(ctx context.Context) (net.Conn, error) {
        return net.DialTimeout("tcp", "127.0.0.1:8080", 5*time.Second)
    }
    
    // 创建连接池
    pool, err := netconnpool.NewPool(config)
    if err != nil {
        panic(err)
    }
    defer pool.Close()
    
    // 获取连接
    conn, err := pool.Get(context.Background())
    if err != nil {
        panic(err)
    }
    defer pool.Put(conn)
    
    // 使用连接
    netConn := conn.GetConn()
    // ... 使用连接进行网络操作 ...
}
```

### 服务器端模式

服务器端模式用于接受客户端连接的场景，适用于HTTP服务器、TCP服务器等。

```go
package main

import (
    "context"
    "net"
    "time"
    "github.com/vistone/netconnpool"
)

func main() {
    // 创建监听器
    listener, _ := net.Listen("tcp", ":8080")

    // 创建服务器端连接池配置
    config := netconnpool.DefaultServerConfig()
    config.Listener = listener
    config.MaxConnections = 100

    // 创建连接池
    pool, err := netconnpool.NewPool(config)
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // 获取连接（等待接受客户端连接）
    conn, err := pool.Get(context.Background())
    if err != nil {
        panic(err)
    }
    defer pool.Put(conn)

    // 使用连接处理客户端请求
    netConn := conn.GetConn()
    // ... 处理客户端请求 ...
}
```

## 使用指南

### 1. 客户端与服务器端模式

连接池支持两种使用模式，以适应不同的应用场景：

| 特性 | 客户端模式 | 服务器端模式 |
|------|-----------|-------------|
| **连接创建方式** | 主动Dial | 被动Accept |
| **必需配置** | `Dialer` | `Listener` |
| **可选配置** | - | `Acceptor` |
| **预热支持** | ✅ 支持 | ❌ 不支持 |
| **典型场景** | HTTP客户端、数据库客户端 | HTTP服务器、TCP服务器 |
| **连接来源** | 主动创建 | 客户端连接 |
| **配置函数** | `DefaultConfig()` | `DefaultServerConfig()` |

#### 客户端模式配置

```go
config := netconnpool.DefaultConfig()
config.Mode = netconnpool.PoolModeClient // 显式设置（默认值）
config.Dialer = func(ctx context.Context) (net.Conn, error) {
    return net.DialTimeout("tcp", "server:port", timeout)
}
```

#### 服务器端模式配置

```go
config := netconnpool.DefaultServerConfig()
listener, _ := net.Listen("tcp", ":8080")
config.Listener = listener

// 可选：自定义连接接受逻辑
config.Acceptor = func(ctx context.Context, l net.Listener) (net.Conn, error) {
    // 自定义Accept逻辑，例如添加日志、限流等
    conn, err := l.Accept()
    if err == nil {
        log.Printf("接受新连接: %s", conn.RemoteAddr())
    }
    return conn, err
}
```

### 2. 获取连接的方式

连接池提供了多种获取连接的方法：

```go
// 基本方法：自动选择IP版本和协议
conn, err := pool.Get(ctx)

// 指定IP版本
ipv4Conn, err := pool.GetIPv4(ctx)
ipv6Conn, err := pool.GetIPv6(ctx)

// 指定协议
tcpConn, err := pool.GetTCP(ctx)
udpConn, err := pool.GetUDP(ctx)

// 使用超时控制
conn, err := pool.GetWithTimeout(ctx, 5*time.Second)

// 指定IP版本和超时
conn, err := pool.GetWithIPVersion(ctx, netconnpool.IPVersionIPv6, 5*time.Second)

// 指定协议和超时
conn, err := pool.GetWithProtocol(ctx, netconnpool.ProtocolUDP, 5*time.Second)
```

### 3. 查看连接信息

Connection 对象提供了丰富的连接信息查询方法：

```go
conn, _ := pool.Get(ctx)
defer pool.Put(conn)

// 基本属性
id := conn.ID  // 连接唯一ID

// 协议和IP版本信息
ipVersion := conn.GetIPVersion()
fmt.Println(ipVersion.String()) // "IPv4" 或 "IPv6"

protocol := conn.GetProtocol()
fmt.Println(protocol.String()) // "TCP" 或 "UDP"

// 复用信息
reuseCount := conn.GetReuseCount()  // 获取连接复用次数

// 连接状态（线程安全方法）
isInUse := conn.IsInUse()  // 检查连接是否正在使用中
isHealthy := conn.GetHealthStatus()  // 获取连接健康状态

// 时间信息
age := conn.GetAge()  // 获取连接年龄（从创建到现在的时间）
idleTime := conn.GetIdleTime()  // 获取连接空闲时间（如果正在使用则返回0）

// 获取原始连接对象
netConn := conn.GetConn()
```

**Connection 方法说明**：

| 方法 | 说明 | 线程安全 |
|------|------|---------|
| `GetProtocol()` | 获取协议类型（TCP/UDP） | ✅ 是 |
| `GetIPVersion()` | 获取IP版本（IPv4/IPv6） | ✅ 是 |
| `GetConn()` | 获取底层连接对象 | ✅ 是 |
| `GetReuseCount()` | 获取连接复用次数 | ✅ 是 |
| `IsInUse()` | 检查连接是否正在使用中 | ✅ 是 |
| `GetHealthStatus()` | 获取连接健康状态 | ✅ 是 |
| `GetAge()` | 获取连接年龄 | ✅ 是 |
| `GetIdleTime()` | 获取连接空闲时间 | ✅ 是 |
| `Close()` | 关闭连接 | ✅ 是 |

**重要提示**：
- `conn.ID` 是公共字段，可以直接访问（只读，线程安全）
- 其他信息应使用提供的getter方法，这些方法是线程安全的
- `GetAge()` 和 `GetIdleTime()` 可以用于监控和调试
- `IsInUse()` 和 `GetHealthStatus()` 是新增的线程安全方法，推荐使用
- 不要直接访问 `conn.InUse` 或 `conn.IsHealthy` 字段，应使用对应的方法

### 4. 归还连接

连接使用完毕后必须归还，否则会导致连接泄漏：

```go
// 方式1：使用defer确保归还（推荐）
conn, err := pool.Get(ctx)
if err != nil {
    return err
}
defer pool.Put(conn)  // 确保归还

// 使用连接...
```

```go
// 方式2：正常归还
conn, err := pool.Get(ctx)
if err != nil {
    return err
}

// 使用连接...
err = pool.Put(conn)
if err != nil {
    // 处理归还错误
}
```

```go
// 方式3：连接出错时关闭而不是归还
conn, err := pool.Get(ctx)
if err != nil {
    return err
}

if err := useConnection(conn); err != nil {
    // 连接出错，关闭连接而不是归还
    conn.Close()  // 关闭错误连接
    return err
}

// 正常归还
pool.Put(conn)
```

**重要注意事项**：
1. **必须归还连接**：使用完连接后，必须调用 `Put()` 归还，或者调用 `Close()` 关闭
2. **使用defer**：推荐使用 `defer pool.Put(conn)` 确保连接被归还
3. **错误连接处理**：如果连接在使用过程中出错，应该调用 `conn.Close()` 关闭而不是归还
4. **泄漏检测**：未归还的连接会被泄漏检测器标记为泄漏，超过 `ConnectionLeakTimeout` 时间会被记录
5. **连接池关闭**：如果连接池已关闭，`Put()` 会自动关闭连接
6. **空闲池满**：如果空闲连接池已满，`Put()` 会自动关闭连接

### 5. TCP/UDP 协议支持

连接池自动识别连接的协议类型，并提供专门的方法获取指定协议的连接：

```go
// 获取TCP连接
tcpConn, err := pool.GetTCP(context.Background())

// 获取UDP连接
udpConn, err := pool.GetUDP(context.Background())

// 获取指定协议的连接
udpConn, err := pool.GetWithProtocol(context.Background(), netconnpool.ProtocolUDP, 5*time.Second)

// 查看连接的协议类型
protocol := conn.GetProtocol()
fmt.Println(protocol.String()) // 输出: "TCP" 或 "UDP"
```

### 6. IPv4/IPv6 支持

连接池自动识别连接的IP版本，并提供专门的方法获取指定IP版本的连接：

```go
// 获取IPv4连接
ipv4Conn, err := pool.GetIPv4(context.Background())

// 获取IPv6连接
ipv6Conn, err := pool.GetIPv6(context.Background())

// 获取指定IP版本的连接
ipv6Conn, err := pool.GetWithIPVersion(context.Background(), netconnpool.IPVersionIPv6, 5*time.Second)

// 查看连接的IP版本
ipVersion := conn.GetIPVersion()
fmt.Println(ipVersion.String()) // 输出: "IPv4" 或 "IPv6"
```

### 7. UDP 缓冲区管理

UDP连接在使用后可能会在读取缓冲区中残留数据包，这会导致连接复用时的数据混淆。连接池提供了自动缓冲区清理功能：

#### 问题描述

当UDP连接被使用后归还到连接池时，读取缓冲区中可能残留未读取的数据包。如果这个连接被其他goroutine复用，新的使用者可能会错误地读取到之前残留的数据包，导致：
- **数据混淆**：读取到不属于当前请求的旧数据
- **协议错误**：响应数据与请求不匹配
- **应用逻辑错误**：基于错误数据做出错误的业务判断

#### 解决方案

```go
config := netconnpool.DefaultConfig()
config.ClearUDPBufferOnReturn = true  // 启用缓冲区清理（默认启用）
config.UDPBufferClearTimeout = 100 * time.Millisecond  // 清理超时时间
config.MaxBufferClearPackets = 100 // 最大清理包数

pool, _ := netconnpool.NewPool(config)
// 连接归还时会自动清空UDP读取缓冲区
```

#### 工作原理

1. **连接归还时清理**：
   - 当UDP连接被归还到连接池时，自动清空读取缓冲区
   - 使用带超时的读取操作，避免永久阻塞
   - 持续读取直到缓冲区为空或超时
   - 使用非阻塞方式（goroutine异步清理），不影响连接归还的性能

2. **连接创建时清理**：
   - 新创建的UDP连接也会进行缓冲区清理
   - 防止新连接包含之前的残留数据
   - 使用后台清理，不阻塞连接创建

3. **健康检查优化**：
   - UDP连接的健康检查使用极短的超时（1ms）
   - 通过超时错误判断连接是否正常（超时表示连接正常但无数据）
   - 避免UDP健康检查时的阻塞问题

**重要说明**：
- 默认情况下，UDP连接在归还时会自动清空读取缓冲区
- 这可以防止连接复用时的数据混淆问题
- 如果不需要此功能，可以设置 `ClearUDPBufferOnReturn = false`
- 清理操作使用非阻塞方式，不会影响连接归还的性能
- 建议在生产环境中保持启用状态

### 8. 连接复用机制

连接复用是连接池的核心功能，通过重复使用已创建的连接而不是每次请求都创建新连接，大大减少了连接创建和销毁的开销。

#### 工作原理

**连接获取流程：**

1. **从空闲连接池获取**：优先从 `idleTCPConnections` 或 `idleUDPConnections` 通道获取已归还的空闲连接
2. **创建新连接**：如果空闲连接池为空，且未达到最大连接数，创建新连接
3. **等待可用连接**：如果已达到最大连接数，加入等待队列，等待其他连接被归还

**连接归还流程：**

1. **验证连接有效性**：检查连接是否仍然有效
2. **清理连接状态**：对于UDP连接，清空读取缓冲区（如果启用）
3. **归还到空闲池**：将连接放回对应的空闲通道
4. **通知等待队列**：如果有等待的请求，立即通知

#### 复用统计

```go
stats := pool.Stats()
fmt.Printf("累计创建连接数: %d\n", stats.TotalConnectionsCreated)
fmt.Printf("累计复用次数: %d\n", stats.TotalConnectionsReused)
fmt.Printf("平均复用次数: %.2f\n", stats.AverageReuseCount)
fmt.Printf("复用率: %.2f%%\n", 
    float64(stats.TotalConnectionsReused) / float64(stats.SuccessfulGets) * 100)

// 查看单个连接的复用次数
conn, _ := pool.Get(ctx)
reuseCount := conn.GetReuseCount()
fmt.Printf("连接ID %d 已被复用 %d 次\n", conn.ID, reuseCount)
```

#### 性能优势

1. **减少连接创建开销**：
   - TCP三次握手
   - TLS握手（如果使用）
   - 连接初始化

2. **降低系统资源消耗**：
   - 减少文件描述符使用
   - 降低内存分配
   - 减少网络带宽

3. **提高响应速度**：
   - 复用连接可立即使用
   - 无需等待连接建立

4. **避免协议混淆**：
   - 独立的 TCP/UDP 空闲池，避免了从通用池中取出错误协议连接导致的重试开销

典型性能提升：使用连接池可达到40倍以上的性能提升（相比每次创建新连接）。

## 工作流程

### 连接池初始化流程

```
1. 创建配置（DefaultConfig 或 DefaultServerConfig）
   ↓
2. 设置必需配置（Dialer 或 Listener）
   ↓
3. 调用 NewPool(config)
   ↓
4. 验证配置（Validate）
   ↓
5. 初始化连接池结构（空闲连接通道、等待队列等）
   ↓
6. 初始化后台管理器（健康检查、清理、泄漏检测、统计）
   ↓
7. 预热连接（仅客户端模式，创建 MinConnections 个连接）
   ↓
8. 启动后台任务（健康检查循环、清理循环、泄漏检测循环）
   ↓
9. 返回连接池实例
```

### 获取连接流程

```
1. 调用 Get() / GetIPv4() / GetTCP() 等方法
   ↓
2. 检查连接池是否已关闭
   ↓
3. 尝试从对应的空闲连接池获取连接 (TCP/UDP 分离)
   ├─→ 找到匹配的连接
   │   ├─→ 检查连接有效性
   │   ├─→ 检查协议/IP版本是否匹配（如果指定）
   │   ├─→ 标记为使用中
   │   ├─→ 增加复用计数
   │   └─→ 返回连接
   └─→ 空闲连接池为空
       ↓
4. 检查是否达到最大连接数
   ├─→ 未达到：创建新连接
   │   ├─→ 获取创建信号量（限制并发创建数）
   │   ├─→ 调用 Dialer 或 Acceptor 创建连接
   │   ├─→ 包装为 Connection 对象
   │   ├─→ 添加到连接映射表
   │   ├─→ 更新统计信息
   │   └─→ 返回连接
   └─→ 已达到：加入等待队列
       ├─→ 等待可用连接（等待队列通知）
       └─→ 超时返回错误
```

### 归还连接流程

```
1. 调用 Put(conn)
   ↓
2. 检查连接是否有效
   ├─→ 无效：关闭连接
   └─→ 有效：继续
       ↓
3. 清理连接状态
   ├─→ UDP连接：清空读取缓冲区（如果启用，非阻塞方式）
   └─→ 标记为空闲
       ↓
4. 尝试归还到对应的空闲连接池
   ├─→ 成功：更新统计信息，通知等待队列
   └─→ 失败（空闲池已满）：关闭连接
```

### 后台任务流程

#### 健康检查循环

```
1. 每 HealthCheckInterval 执行一次（默认30秒，可配置）
   ↓
2. 获取所有连接
   ↓
3. 跳过正在使用中的连接（只检查空闲连接）
   ↓
4. 使用信号量限制并发检查数（最多10个并发）
   ↓
5. 对每个连接执行健康检查
   ├─→ TCP连接：尝试读取（超时 HealthCheckTimeout，默认3秒）
   ├─→ UDP连接：尝试读取（超时1ms），超时表示正常
   └─→ 自定义检查：调用 HealthChecker 函数
       ↓
6. 不健康的连接被标记为不健康
   ↓
7. 不健康的连接在下次获取时会被验证并关闭
```

**健康检查说明**：
- 健康检查只检查空闲连接，不会检查正在使用中的连接
- 使用信号量限制最多10个并发检查，避免资源消耗过大
- TCP连接使用配置的 `HealthCheckTimeout`（默认3秒）进行健康检查
- UDP连接使用极短超时（1ms），超时表示连接正常（只是没有数据）
- 如果提供了自定义 `HealthChecker`，会使用自定义函数进行检查
- 不健康的连接会被标记，在下次从空闲池获取时会验证并关闭
- 健康检查在后台执行，不会阻塞正常操作
- 健康检查失败会记录到统计信息（`HealthCheckFailures`、`UnhealthyConnections`）

#### 清理循环

```
1. 每 CleanupInterval 执行一次
   ↓
2. 检查空闲连接
   ├─→ 超过 IdleTimeout：关闭
   └─→ 超过 MaxLifetime：关闭
       ↓
3. 检查空闲连接数
   └─→ 超过 MaxIdleConnections：关闭多余的连接（优先关闭空闲时间最长的）
```

#### 泄漏检测循环

```
1. 每 1 分钟执行一次（固定间隔）
   ↓
2. 检查所有使用中的连接
   ↓
3. 连接使用时间超过 ConnectionLeakTimeout
   └─→ 标记为泄漏，记录到统计信息
```

**泄漏检测说明**：
- 检测间隔固定为 1 分钟
- 只检测使用中的连接（`InUse = true`）
- 泄漏检测不会自动关闭连接，只记录统计信息
- 可以通过 `stats.LeakedConnections` 查看泄漏的连接数
- 实际应用中可以根据统计信息进行告警或日志记录

## 功能要点

### 1. 连接生命周期管理

连接池自动管理连接的完整生命周期：

- **创建**：按需创建或预热创建
- **使用**：获取后标记为使用中
- **归还**：使用完毕后归还到空闲池
- **复用**：优先复用空闲连接
- **清理**：自动清理过期、不健康的连接
- **销毁**：连接池关闭时销毁所有连接

### 2. 并发安全

所有操作都是并发安全的：
- 使用互斥锁保护共享状态
- 使用通道进行goroutine间通信
- 原子操作更新统计信息
- 信号量控制并发创建数

### 3. 连接有效性检查

在复用连接前会检查：
- 连接是否已关闭
- 连接是否健康（健康检查）
- 连接是否过期（超过MaxLifetime）
- 连接是否空闲太久（超过IdleTimeout）

### 4. 协议和IP版本匹配

获取连接时，会根据指定的协议或IP版本从空闲池中选择匹配的连接：
- `GetTCP()` 只会复用TCP连接
- `GetUDP()` 只会复用UDP连接
- `GetIPv4()` 只会复用IPv4连接
- `GetIPv6()` 只会复用IPv6连接

### 5. 等待队列机制

当连接池达到最大连接数时，新的获取请求会：
1. 加入等待队列（支持按协议/IP版本分别排队）
   - 通用等待队列：`Get()` 使用
   - TCP等待队列：`GetTCP()` 使用
   - UDP等待队列：`GetUDP()` 使用
   - IPv4等待队列：`GetIPv4()` 使用
   - IPv6等待队列：`GetIPv6()` 使用
2. 等待其他连接被归还
3. 连接归还时，优先通知对应协议/IP版本的等待队列
4. 如果超时，返回 `ErrGetConnectionTimeout`

**等待队列特性**：
- 每个等待队列缓冲区大小为 100
- 连接归还时会优先通知匹配的等待队列
- 如果匹配的等待队列为空，会通知通用等待队列
- 等待队列使用通道实现，性能优秀
- 连接池关闭时，所有等待的goroutine会被通知并返回 `ErrPoolClosed`

### 6. 优雅关闭

`Close()` 方法确保：
1. 停止接受新的获取请求
2. 通知所有等待的goroutine
3. 停止所有后台任务
4. 关闭所有连接
5. 清理所有资源

## 配置选项

### Config 结构

```go
type Config struct {
    // Mode 连接池模式：客户端或服务器端
    // 默认值为PoolModeClient（客户端模式）
    Mode PoolMode
    
    // MaxConnections 最大连接数，0表示无限制
    MaxConnections int
    
    // MinConnections 最小连接数（预热连接数，仅客户端模式）
    MinConnections int
    
    // MaxIdleConnections 最大空闲连接数
    MaxIdleConnections int
    
    // ConnectionTimeout 连接创建超时时间
    ConnectionTimeout time.Duration
    
    // IdleTimeout 空闲连接超时时间，超过此时间的空闲连接将被关闭
    IdleTimeout time.Duration
    
    // MaxLifetime 连接最大生命周期，超过此时间的连接将被关闭
    MaxLifetime time.Duration
    
    // GetConnectionTimeout 获取连接的超时时间
    GetConnectionTimeout time.Duration
    
    // HealthCheckInterval 健康检查间隔
    HealthCheckInterval time.Duration
    
    // HealthCheckTimeout 健康检查超时时间
    HealthCheckTimeout time.Duration
    
    // ConnectionLeakTimeout 连接泄漏检测超时时间
    ConnectionLeakTimeout time.Duration
    
    // Dialer 连接创建函数（客户端模式必需）
    Dialer Dialer
    
    // Listener 网络监听器（服务器端模式必需）
    Listener net.Listener
    
    // Acceptor 连接接受函数（服务器端模式可选）
    Acceptor Acceptor
    
    // HealthChecker 健康检查函数（可选）
    HealthChecker HealthChecker
    
    // CloseConn 连接关闭函数（可选）
    CloseConn func(conn net.Conn) error
    
    // OnCreated 连接创建后调用
    OnCreated func(conn net.Conn) error

    // OnBorrow 连接从池中取出前调用
    OnBorrow func(conn net.Conn)

    // OnReturn 连接归还池中前调用
    OnReturn func(conn net.Conn)
    
    // EnableStats 是否启用统计信息
    EnableStats bool
    
    // EnableHealthCheck 是否启用健康检查
    EnableHealthCheck bool
    
    // ClearUDPBufferOnReturn 是否在归还UDP连接时清空读取缓冲区
    // 默认值: true
    ClearUDPBufferOnReturn bool
    
    // UDPBufferClearTimeout UDP缓冲区清理超时时间
    // 默认值: 100ms
    UDPBufferClearTimeout time.Duration

    // MaxBufferClearPackets UDP缓冲区清理最大包数
    // 默认值: 100
    MaxBufferClearPackets int
}
```

### 默认配置

#### 客户端模式

```go
config := netconnpool.DefaultConfig()
// Mode: PoolModeClient
// MaxConnections: 10
// MinConnections: 2
// MaxIdleConnections: 10
// ConnectionTimeout: 10s
// IdleTimeout: 5m
// MaxLifetime: 30m
// GetConnectionTimeout: 5s
// HealthCheckInterval: 30s
// HealthCheckTimeout: 3s
// ConnectionLeakTimeout: 5m
// EnableStats: true
// EnableHealthCheck: true
// ClearUDPBufferOnReturn: true
// UDPBufferClearTimeout: 100ms
// MaxBufferClearPackets: 100
```

#### 服务器端模式

```go
config := netconnpool.DefaultServerConfig()
// Mode: PoolModeServer
// MaxConnections: 100
// MinConnections: 0
// MaxIdleConnections: 50
// 其他配置与客户端模式相同
```

### 配置验证

配置在创建连接池时会自动验证：

- 客户端模式必须提供 `Dialer`
- 服务器端模式必须提供 `Listener`
- `MinConnections` 不能小于0
- `MinConnections` 不能大于 `MaxConnections`（如果MaxConnections > 0）
- `MaxIdleConnections` 不能小于等于0
- `ConnectionTimeout` 必须大于0
- `MaxIdleConnections` 不应超过 `MaxConnections`（会自动修正）
- 健康检查超时不应超过检查间隔（会自动修正）

## 高级功能

### 自定义健康检查

如果默认的健康检查不满足需求，可以自定义健康检查函数：

```go
config.HealthChecker = func(conn net.Conn) bool {
    // 执行自定义健康检查逻辑
    // 例如：发送ping消息、检查特定状态等
    
    // 示例：检查连接是否可写
    conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
    _, err := conn.Write([]byte{})
    if err != nil {
        return false
    }
    return true
}
```
