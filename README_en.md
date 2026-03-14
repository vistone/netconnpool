# NetConnPool - Network Connection Pool Management Library

[![Go Reference](https://pkg.go.dev/badge/github.com/vistone/netconnpool.svg)](https://pkg.go.dev/github.com/vistone/netconnpool)
[![Go Report Card](https://goreportcard.com/badge/github.com/vistone/netconnpool)](https://goreportcard.com/report/github.com/vistone/netconnpool)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

A comprehensive, high-performance Go network connection pool management library with complete connection lifecycle management, health checks, and statistical monitoring.

**Core Features**:
- 🚀 **High Performance**: > 95% connection reuse rate, significantly improving performance
- 🔒 **Concurrency Safe**: Fully thread-safe, supports high-concurrency scenarios
- 🎯 **Flexible Configuration**: Supports both client and server modes
- 📊 **Detailed Statistics**: Rich statistical information for monitoring and optimization
- 🛡️ **Automatic Management**: Health checks, leak detection, automatic cleanup
- 🌐 **Protocol Support**: TCP/UDP support, IPv4/IPv6 support
- 🔄 **Smart Idle Pool**: Separate idle pools for TCP/UDP, avoiding performance jitter from protocol confusion
- 🪝 **Lifecycle Hooks**: Support for custom callbacks at Created/Borrow/Return stages

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage Guide](#usage-guide)
- [Workflow](#workflow)
- [Key Features](#key-features)
- [Configuration Options](#configuration-options)
- [Advanced Features](#advanced-features)
- [Notes](#notes)
- [Error Handling](#error-handling)
- [Performance Optimization](#performance-optimization)
- [Architecture Design](#architecture-design)
- [Example Code](#example-code)
- [Testing](#testing)

## Features

- ✅ **Client/Server Mode**: Supports both client active connection and server passive accept modes
- ✅ **Complete Connection Lifecycle Management**: Create, borrow, return, destroy
- ✅ **Connection Reuse Mechanism**: Automatically reuse established connections for significant performance improvement
- ✅ **TCP/UDP Protocol Support**: Automatic recognition and differentiation of TCP/UDP connections, supports acquiring connections with specific protocol type
- ✅ **IPv4/IPv6 Support**: Automatic recognition and differentiation of IPv4/IPv6 connections, supports acquiring connections with specific IP version
- ✅ **UDP Buffer Management**: Automatically cleans UDP read buffers to prevent data confusion
- ✅ **Flexible Connection Pool Configuration**: Max/min connections, idle connections, etc.
- ✅ **Connection Health Checks**: Automatically detect and clean up unhealthy connections
- ✅ **Connection Leak Detection**: Timely detection of connections not returned for a long time
- ✅ **Connection Timeout Management**: Creation timeout, idle timeout, lifetime limit
- ✅ **Statistics and Monitoring**: Detailed connection pool usage statistics (including separate statistics for TCP/UDP and IPv4/IPv6, connection reuse statistics)
- ✅ **Concurrency Safe**: Fully supports concurrent access
- ✅ **Graceful Shutdown**: Supports graceful shutdown and resource cleanup of the connection pool
- ✅ **Waiting Queue**: Automatic queuing when connection pool is exhausted (supports separate queuing for TCP/UDP and IPv4/IPv6)
- ✅ **Type Safety**: Based on `net.Conn` interface, providing stronger type checking

## Installation

```bash
go get github.com/vistone/netconnpool
```

## Quick Start

### Client Mode (Default)

Client mode is used for scenarios where you actively connect to a server, suitable for HTTP clients, database clients, RPC clients, etc.

```go
package main

import (
    "context"
    "net"
    "time"
    "github.com/vistone/netconnpool"
)

func main() {
    // Create client connection pool configuration
    config := netconnpool.DefaultConfig()
    config.MaxConnections = 10
    config.MinConnections = 2 // Pre-warm 2 connections
    
    // Set connection creation function
    config.Dialer = func(ctx context.Context) (net.Conn, error) {
        return net.DialTimeout("tcp", "127.0.0.1:8080", 5*time.Second)
    }
    
    // Create connection pool
    pool, err := netconnpool.NewPool(config)
    if err != nil {
        panic(err)
    }
    defer pool.Close()
    
    // Get connection
    conn, err := pool.Get(context.Background())
    if err != nil {
        panic(err)
    }
    defer pool.Put(conn)
    
    // Use connection
    netConn := conn.GetConn()
    // ... use connection for network operations ...
}
```

### Server Mode

Server mode is used for scenarios where you accept client connections, suitable for HTTP servers, TCP servers, etc.

```go
package main

import (
    "context"
    "net"
    "time"
    "github.com/vistone/netconnpool"
)

func main() {
    // Create listener
    listener, _ := net.Listen("tcp", ":8080")

    // Create server-side connection pool configuration
    config := netconnpool.DefaultServerConfig()
    config.Listener = listener
    config.MaxConnections = 100

    // Create connection pool
    pool, err := netconnpool.NewPool(config)
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // Get connection (wait to accept client connection)
    conn, err := pool.Get(context.Background())
    if err != nil {
        panic(err)
    }
    defer pool.Put(conn)

    // Use connection to handle client requests
    netConn := conn.GetConn()
    // ... handle client requests ...
}
```

## Usage Guide

### 1. Client vs Server Mode

The connection pool supports two usage modes to adapt to different application scenarios:

| Feature | Client Mode | Server Mode |
|---------|-------------|-------------|
| **Connection Creation** | Active Dial | Passive Accept |
| **Required Config** | `Dialer` | `Listener` |
| **Optional Config** | - | `Acceptor` |
| **Pre-warm Support** | ✅ Supported | ❌ Not supported |
| **Typical Scenarios** | HTTP clients, database clients | HTTP servers, TCP servers |
| **Connection Source** | Actively created | Client connections |
| **Config Function** | `DefaultConfig()` | `DefaultServerConfig()` |

#### Client Mode Configuration

```go
config := netconnpool.DefaultConfig()
config.Mode = netconnpool.PoolModeClient // Explicitly set (default value)
config.Dialer = func(ctx context.Context) (net.Conn, error) {
    return net.DialTimeout("tcp", "server:port", timeout)
}
```

#### Server Mode Configuration

```go
config := netconnpool.DefaultServerConfig()
listener, _ := net.Listen("tcp", ":8080")
config.Listener = listener

// Optional: custom connection accept logic
config.Acceptor = func(ctx context.Context, l net.Listener) (net.Conn, error) {
    // Custom Accept logic, such as adding logs, rate limiting, etc.
    conn, err := l.Accept()
    if err == nil {
        log.Printf("Accepted new connection: %s", conn.RemoteAddr())
    }
    return conn, err
}
```

### 2. Ways to Get Connections

The connection pool provides multiple methods to get connections:

```go
// Basic method: automatically select IP version and protocol
conn, err := pool.Get(ctx)

// Specify IP version
ipv4Conn, err := pool.GetIPv4(ctx)
ipv6Conn, err := pool.GetIPv6(ctx)

// Specify protocol
tcpConn, err := pool.GetTCP(ctx)
udpConn, err := pool.GetUDP(ctx)

// Use timeout control
conn, err := pool.GetWithTimeout(ctx, 5*time.Second)

// Specify IP version and timeout
conn, err := pool.GetWithIPVersion(ctx, netconnpool.IPVersionIPv6, 5*time.Second)

// Specify protocol and timeout
conn, err := pool.GetWithProtocol(ctx, netconnpool.ProtocolUDP, 5*time.Second)
```

### 3. Viewing Connection Information

The Connection object provides rich connection information query methods:

```go
conn, _ := pool.Get(ctx)
defer pool.Put(conn)

// Basic properties
id := conn.ID  // Connection unique ID

// Protocol and IP version information
ipVersion := conn.GetIPVersion()
fmt.Println(ipVersion.String()) // "IPv4" or "IPv6"

protocol := conn.GetProtocol()
fmt.Println(protocol.String()) // "TCP" or "UDP"

// Reuse information
reuseCount := conn.GetReuseCount()  // Get connection reuse count

// Connection status (thread-safe methods)
isInUse := conn.IsInUse()  // Check if connection is in use
isHealthy := conn.GetHealthStatus()  // Get connection health status

// Time information
age := conn.GetAge()  // Get connection age (time since creation)
idleTime := conn.GetIdleTime()  // Get connection idle time (returns 0 if in use)

// Get raw connection object
netConn := conn.GetConn()
```

**Connection Method Descriptions**:

| Method | Description | Thread Safe |
|--------|-------------|-------------|
| `GetProtocol()` | Get protocol type (TCP/UDP) | ✅ Yes |
| `GetIPVersion()` | Get IP version (IPv4/IPv6) | ✅ Yes |
| `GetConn()` | Get underlying connection object | ✅ Yes |
| `GetReuseCount()` | Get connection reuse count | ✅ Yes |
| `IsInUse()` | Check if connection is currently in use | ✅ Yes |
| `GetHealthStatus()` | Get connection health status | ✅ Yes |
| `GetAge()` | Get connection age | ✅ Yes |
| `GetIdleTime()` | Get connection idle time | ✅ Yes |
| `Close()` | Close connection | ✅ Yes |

**Important Notes**:
- `conn.ID` is a public field that can be accessed directly (read-only, thread-safe)
- Other information should use the provided getter methods, which are thread-safe
- `GetAge()` and `GetIdleTime()` can be used for monitoring and debugging
- `IsInUse()` and `GetHealthStatus()` are new thread-safe methods, recommended for use
- Do not directly access `conn.InUse` or `conn.IsHealthy` fields, use the corresponding methods instead

### 4. Returning Connections

Connections must be returned after use, otherwise it will cause connection leaks:

```go
// Method 1: Use defer to ensure return (recommended)
conn, err := pool.Get(ctx)
if err != nil {
    return err
}
defer pool.Put(conn)  // Ensure return

// Use connection...
```

```go
// Method 2: Normal return
conn, err := pool.Get(ctx)
if err != nil {
    return err
}

// Use connection...
err = pool.Put(conn)
if err != nil {
    // Handle return error
}
```

```go
// Method 3: Close instead of return when connection has errors
conn, err := pool.Get(ctx)
if err != nil {
    return err
}

if err := useConnection(conn); err != nil {
    // Connection error, close connection instead of returning
    conn.Close()  // Close error connection
    return err
}

// Normal return
pool.Put(conn)
```

**Important Notes**:
1. **Must return connections**: After using a connection, you must call `Put()` to return it, or call `Close()` to close it
2. **Use defer**: Recommended to use `defer pool.Put(conn)` to ensure connections are returned
3. **Error connection handling**: If the connection has errors during use, you should call `conn.Close()` to close it instead of returning it
4. **Leak detection**: Connections not returned will be marked as leaks by the leak detector, and will be logged after `ConnectionLeakTimeout`
5. **Pool closed**: If the connection pool is closed, `Put()` will automatically close the connection
6. **Idle pool full**: If the idle connection pool is full, `Put()` will automatically close the connection

### 5. TCP/UDP Protocol Support

The connection pool automatically recognizes the protocol type of connections and provides specific methods to get connections with specified protocols:

```go
// Get TCP connection
tcpConn, err := pool.GetTCP(context.Background())

// Get UDP connection
udpConn, err := pool.GetUDP(context.Background())

// Get connection with specified protocol
udpConn, err := pool.GetWithProtocol(context.Background(), netconnpool.ProtocolUDP, 5*time.Second)

// View connection protocol type
protocol := conn.GetProtocol()
fmt.Println(protocol.String()) // Output: "TCP" or "UDP"
```

### 6. IPv4/IPv6 Support

The connection pool automatically recognizes the IP version of connections and provides specific methods to get connections with specified IP versions:

```go
// Get IPv4 connection
ipv4Conn, err := pool.GetIPv4(context.Background())

// Get IPv6 connection
ipv6Conn, err := pool.GetIPv6(context.Background())

// Get connection with specified IP version
ipv6Conn, err := pool.GetWithIPVersion(context.Background(), netconnpool.IPVersionIPv6, 5*time.Second)

// View connection IP version
ipVersion := conn.GetIPVersion()
fmt.Println(ipVersion.String()) // Output: "IPv4" or "IPv6"
```

### 7. UDP Buffer Management

UDP connections may have residual data packets in the read buffer after use, which can lead to data confusion when connections are reused. The connection pool provides automatic buffer cleanup functionality:

#### Problem Description

When UDP connections are returned to the pool after use, there may be unprocessed data packets remaining in the read buffer. If this connection is reused by another goroutine, the new user may incorrectly read the old residual data packets, causing:
- **Data Confusion**: Reading old data that does not belong to the current request
- **Protocol Errors**: Response data does not match the request
- **Application Logic Errors**: Making wrong business decisions based on wrong data

#### Solution

```go
config := netconnpool.DefaultConfig()
config.ClearUDPBufferOnReturn = true  // Enable buffer cleanup (enabled by default)
config.UDPBufferClearTimeout = 100 * time.Millisecond  // Cleanup timeout
config.MaxBufferClearPackets = 100 // Maximum packets to clear

pool, _ := netconnpool.NewPool(config)
// UDP read buffer will be automatically cleared when connection is returned
```

#### How It Works

1. **Cleanup when returning connections**:
   - When UDP connections are returned to the pool, automatically clear the read buffer
   - Use read operations with timeout to avoid permanent blocking
   - Continue reading until buffer is empty or timeout occurs
   - Use non-blocking method (goroutine asynchronous cleanup), not affecting connection return performance

2. **Cleanup when creating connections**:
   - Newly created UDP connections are also cleaned
   - Prevent new connections from containing previous residual data
   - Use background cleanup, not blocking connection creation

3. **Health check optimization**:
   - UDP connection health checks use very short timeout (1ms)
   - Determine if connection is normal by timeout error (timeout means connection is normal but no data)
   - Avoid blocking issues during UDP health checks

**Important Notes**:
- By default, UDP connections automatically clear the read buffer when returned
- This prevents data confusion issues when connections are reused
- If this feature is not needed, you can set `ClearUDPBufferOnReturn = false`
- Cleanup operations use non-blocking methods and will not affect connection return performance
- Recommended to keep enabled in production environments

### 8. Connection Reuse Mechanism

Connection reuse is the core feature of the connection pool. By reusing already created connections instead of creating new connections for each request, it greatly reduces the overhead of connection creation and destruction.

#### How It Works

**Connection Acquisition Flow:**

1. **Get from idle connection pool**: Prioritize getting already returned idle connections from `idleTCPConnections` or `idleUDPConnections` channels
2. **Create new connection**: If idle connection pool is empty, and maximum connections not reached, create new connection
3. **Wait for available connection**: If maximum connections reached, join waiting queue, wait for other connections to be returned

**Connection Return Flow:**

1. **Verify connection validity**: Check if connection is still valid
2. **Clean connection state**: For UDP connections, clear read buffer (if enabled)
3. **Return to idle pool**: Put connection back to corresponding idle channel
4. **Notify waiting queue**: If there are waiting requests, notify immediately

#### Reuse Statistics

```go
stats := pool.Stats()
fmt.Printf("Total connections created: %d\n", stats.TotalConnectionsCreated)
fmt.Printf("Total reuse count: %d\n", stats.TotalConnectionsReused)
fmt.Printf("Average reuse count: %.2f\n", stats.AverageReuseCount)
fmt.Printf("Reuse rate: %.2f%%\n", 
    float64(stats.TotalConnectionsReused) / float64(stats.SuccessfulGets) * 100)

// View reuse count of individual connection
conn, _ := pool.Get(ctx)
reuseCount := conn.GetReuseCount()
fmt.Printf("Connection ID %d has been reused %d times\n", conn.ID, reuseCount)
```

#### Performance Benefits

1. **Reduce connection creation overhead**:
   - TCP three-way handshake
   - TLS handshake (if using)
   - Connection initialization

2. **Reduce system resource consumption**:
   - Reduce file descriptor usage
   - Reduce memory allocation
   - Reduce network bandwidth

3. **Improve response speed**:
   - Reused connections can be used immediately
   - No need to wait for connection establishment

4. **Avoid protocol confusion**:
   - Independent TCP/UDP idle pools, avoiding retry overhead caused by getting wrong protocol connections from general pool

Typical performance improvement: Using connection pool can achieve more than 40x performance improvement (compared to creating new connections each time).

## Workflow

### Connection Pool Initialization Flow

```
1. Create config (DefaultConfig or DefaultServerConfig)
   ↓
2. Set required config (Dialer or Listener)
   ↓
3. Call NewPool(config)
   ↓
4. Validate config
   ↓
5. Initialize pool structure (idle connection channels, wait queues, etc.)
   ↓
6. Initialize background managers (health check, cleanup, leak detection, statistics)
   ↓
7. Pre-warm connections (client mode only, create MinConnections connections)
   ↓
8. Start background tasks (health check loop, cleanup loop, leak detection loop)
   ↓
9. Return pool instance
```

### Connection Acquisition Flow

```
1. Call Get() / GetIPv4() / GetTCP() etc.
   ↓
2. Check if pool is closed
   ↓
3. Try to get connection from corresponding idle pool (TCP/UDP separated)
   ├─→ Found matching connection
   │   ├─→ Check connection validity
   │   ├─→ Check protocol/IP version match (if specified)
   │   ├─→ Mark as in use
   │   ├─→ Increment reuse count
   │   └─→ Return connection
   └─→ Idle connection pool is empty
       ↓
4. Check if max connections reached
   ├─→ Not reached: Create new connection
   │   ├─→ Acquire creation semaphore (limit concurrent creation count)
   │   ├─→ Call Dialer or Acceptor to create connection
   │   ├─→ Wrap as Connection object
   │   ├─→ Add to connection map
   │   ├─→ Update statistics
   │   └─→ Return connection
   └─→ Reached: Join waiting queue
       ├─→ Wait for available connection (wait queue notification)
       └─→ Timeout return error
```

### Connection Return Flow

```
1. Call Put(conn)
   ↓
2. Check if connection is valid
   ├─→ Invalid: Close connection
   └─→ Valid: Continue
       ↓
3. Clean connection state
   ├─→ UDP connection: Clear read buffer (if enabled, non-blocking)
   └─→ Mark as idle
       ↓
4. Try to return to corresponding idle pool
   ├─→ Success: Update statistics, notify wait queue
   └─→ Failure (idle pool full): Close connection
```

### Background Task Flow

#### Health Check Loop

```
1. Execute every HealthCheckInterval (default 30s, configurable)
   ↓
2. Get all connections
   ↓
3. Skip connections in use (only check idle connections)
   ↓
4. Use semaphore to limit concurrent check count (max 10 concurrent)
   ↓
5. Execute health check for each connection
   ├─→ TCP connection: Try to read (timeout HealthCheckTimeout, default 3s)
   ├─→ UDP connection: Try to read (timeout 1ms), timeout means normal
   └─→ Custom check: Call HealthChecker function
       ↓
6. Unhealthy connections are marked as unhealthy
   ↓
7. Unhealthy connections will be validated and closed when next acquired
```

**Health Check Notes**:
- Health checks only check idle connections, not connections in use
- Use semaphore to limit max 10 concurrent checks to avoid excessive resource consumption
- TCP connections use configured `HealthCheckTimeout` (default 3s) for health checks
- UDP connections use very short timeout (1ms), timeout means connection is normal (just no data)
- If custom `HealthChecker` is provided, it will use custom function for checking
- Unhealthy connections are marked and will be validated and closed when next acquired from idle pool
- Health checks run in background and will not block normal operations
- Health check failures are recorded in statistics (`HealthCheckFailures`, `UnhealthyConnections`)

#### Cleanup Loop

```
1. Execute every CleanupInterval
   ↓
2. Check idle connections
   ├─→ Exceeds IdleTimeout: Close
   └─→ Exceeds MaxLifetime: Close
       ↓
3. Check idle connection count
   └─→ Exceeds MaxIdleConnections: Close excess connections (prioritize closing longest idle time)
```

#### Leak Detection Loop

```
1. Execute every 1 minute (fixed interval)
   ↓
2. Check all connections in use
   ↓
3. Connection usage time exceeds ConnectionLeakTimeout
   └─→ Mark as leak, record in statistics
```

**Leak Detection Notes**:
- Detection interval is fixed at 1 minute
- Only detects connections in use (`InUse = true`)
- Leak detection does not automatically close connections, only records statistics
- Can view leaked connection count via `stats.LeakedConnections`
- In actual applications, can perform alerts or log recording based on statistics

## Key Features

### 1. Connection Lifecycle Management

The pool automatically manages the complete lifecycle of connections:

- **Create**: Create on demand or pre-warm create
- **Use**: Mark as in use after acquisition
- **Return**: Return to idle pool after use
- **Reuse**: Prioritize reusing idle connections
- **Cleanup**: Automatically cleanup expired, unhealthy connections
- **Destroy**: Destroy all connections when pool closes

### 2. Concurrency Safety

All operations are concurrency safe:
- Use mutex locks to protect shared state
- Use channels for goroutine communication
- Atomic operations update statistics
- Semaphore controls concurrent creation count

### 3. Connection Validity Check

Before reusing connections, checks are performed:
- Whether connection is closed
- Whether connection is healthy (health check)
- Whether connection is expired (exceeds MaxLifetime)
- Whether connection is idle too long (exceeds IdleTimeout)

### 4. Protocol and IP Version Matching

When acquiring connections, matching connections are selected from the idle pool based on specified protocol or IP version:
- `GetTCP()` only reuses TCP connections
- `GetUDP()` only reuses UDP connections
- `GetIPv4()` only reuses IPv4 connections
- `GetIPv6()` only reuses IPv6 connections

### 5. Waiting Queue Mechanism

When the pool reaches max connections, new acquisition requests will:
1. Join waiting queue (supports separate queuing by protocol/IP version)
   - General waiting queue: Used by `Get()`
   - TCP waiting queue: Used by `GetTCP()`
   - UDP waiting queue: Used by `GetUDP()`
   - IPv4 waiting queue: Used by `GetIPv4()`
   - IPv6 waiting queue: Used by `GetIPv6()`
2. Wait for other connections to be returned
3. When connection is returned, prioritize notifying corresponding protocol/IP version waiting queue
4. If timeout, return `ErrGetConnectionTimeout`

**Waiting Queue Features**:
- Each waiting queue buffer size is 100
- When connection is returned, prioritize notifying matching waiting queue
- If matching waiting queue is empty, notify general waiting queue
- Waiting queues use channel implementation, excellent performance
- When pool closes, all waiting goroutines are notified and return `ErrPoolClosed`

### 6. Graceful Shutdown

`Close()` method ensures:
1. Stop accepting new acquisition requests
2. Notify all waiting goroutines
3. Stop all background tasks
4. Close all connections
5. Cleanup all resources

## Configuration Options

### Config Structure

```go
type Config struct {
    // Mode Pool mode: Client or Server
    // Default value is PoolModeClient (Client mode)
    Mode PoolMode
    
    // MaxConnections Max connections, 0 means unlimited
    MaxConnections int
    
    // MinConnections Min connections (pre-warm connections, client mode only)
    MinConnections int
    
    // MaxIdleConnections Max idle connections
    MaxIdleConnections int
    
    // ConnectionTimeout Connection creation timeout
    ConnectionTimeout time.Duration
    
    // IdleTimeout Idle connection timeout, idle connections exceeding this time will be closed
    IdleTimeout time.Duration
    
    // MaxLifetime Connection max lifetime, connections exceeding this time will be closed
    MaxLifetime time.Duration
    
    // GetConnectionTimeout Connection acquisition timeout
    GetConnectionTimeout time.Duration
    
    // HealthCheckInterval Health check interval
    HealthCheckInterval time.Duration
    
    // HealthCheckTimeout Health check timeout
    HealthCheckTimeout time.Duration
    
    // ConnectionLeakTimeout Connection leak detection timeout
    ConnectionLeakTimeout time.Duration
    
    // Dialer Connection creation function (client mode required)
    Dialer Dialer
    
    // Listener Network listener (server mode required)
    Listener net.Listener
    
    // Acceptor Connection accept function (server mode optional)
    Acceptor Acceptor
    
    // HealthChecker Health check function (optional)
    HealthChecker HealthChecker
    
    // CloseConn Connection close function (optional)
    CloseConn func(conn net.Conn) error
    
    // OnCreated Called after connection creation
    OnCreated func(conn net.Conn) error

    // OnBorrow Called before connection is borrowed from pool
    OnBorrow func(conn net.Conn)

    // OnReturn Called before connection is returned to pool
    OnReturn func(conn net.Conn)
    
    // EnableStats Whether to enable statistics
    EnableStats bool
    
    // EnableHealthCheck Whether to enable health checks
    EnableHealthCheck bool
    
    // ClearUDPBufferOnReturn Whether to clear read buffer when returning UDP connections
    // Default: true
    ClearUDPBufferOnReturn bool
    
    // UDPBufferClearTimeout UDP buffer cleanup timeout
    // Default: 100ms
    UDPBufferClearTimeout time.Duration

    // MaxBufferClearPackets Max packets to clear for UDP buffer
    // Default: 100
    MaxBufferClearPackets int
}
```

### Default Configuration

#### Client Mode

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

#### Server Mode

```go
config := netconnpool.DefaultServerConfig()
// Mode: PoolModeServer
// MaxConnections: 100
// MinConnections: 0
// MaxIdleConnections: 50
// Other config same as client mode
```

### Configuration Validation

Configuration is automatically validated when creating the pool:

- Client mode must provide `Dialer`
- Server mode must provide `Listener`
- `MinConnections` cannot be less than 0
- `MinConnections` cannot be greater than `MaxConnections` (if MaxConnections > 0)
- `MaxIdleConnections` cannot be less than or equal to 0
- `ConnectionTimeout` must be greater than 0
- `MaxIdleConnections` should not exceed `MaxConnections` (will be auto-corrected)
- Health check timeout should not exceed check interval (will be auto-corrected)

## Advanced Features

### Custom Health Check

If default health check doesn't meet requirements, you can customize the health check function:

```go
config.HealthChecker = func(conn net.Conn) bool {
    // Execute custom health check logic
    // For example: Send ping message, check specific status, etc.
    
    // Example: Check if connection is writable
    conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
    _, err := conn.Write([]byte{})
    if err != nil {
        return false
    }
    return true
}
```
