# TLS支持分析与建议

## 当前状态

### ✅ 当前架构已支持TLS

当前连接池的架构设计非常灵活，**已经可以支持TLS连接**，无需修改核心代码：

1. **类型系统支持**：连接池使用 `any` 类型存储连接，可以存储任何实现了 `net.Conn` 接口的类型，包括 `*tls.Conn`
2. **灵活的Dialer/Acceptor**：用户可以在 `Dialer` 或 `Acceptor` 函数中自定义TLS握手逻辑
3. **健康检查可定制**：可以通过 `HealthChecker` 函数自定义TLS连接的健康检查逻辑

### 📝 使用方式

#### 客户端TLS连接（HTTPS客户端）

```go
config.Dialer = func(ctx context.Context) (any, error) {
    // 1. 创建TCP连接
    tcpConn, err := net.DialTimeout("tcp", "example.com:443", 5*time.Second)
    if err != nil {
        return nil, err
    }

    // 2. 配置TLS
    tlsConfig := &tls.Config{
        ServerName: "example.com",
    }

    // 3. 升级为TLS连接
    tlsConn := tls.Client(tcpConn, tlsConfig)

    // 4. 执行握手
    if err := tlsConn.HandshakeContext(ctx); err != nil {
        tcpConn.Close()
        return nil, err
    }

    return tlsConn, nil
}
```

#### 服务器端TLS连接（HTTPS服务器）

```go
// 创建TLS监听器
listener, _ := tls.Listen("tcp", ":8443", &tls.Config{
    Certificates: []tls.Certificate{cert},
})

config := netconnpool.DefaultServerConfig()
config.Listener = listener

// Acceptor可以是默认的，因为tls.Listener已经处理了握手
pool, _ := netconnpool.NewPool(config)
```

## 是否需要内置TLS支持？

### ❌ 建议：**不需要内置TLS支持**

#### 理由1：保持架构简洁
- 当前架构已经足够灵活，用户可以完全控制TLS配置
- 添加内置支持会增加代码复杂度和维护成本
- 连接池的职责是管理连接，而不是创建连接

#### 理由2：TLS配置多样性
TLS配置有太多变体和选项，难以覆盖所有场景：
- 客户端证书认证（mTLS）
- 不同的TLS版本要求（TLS 1.0/1.1/1.2/1.3）
- 自定义CA证书
- 证书链验证
- 会话恢复
- ALPN协议协商
- 证书轮换

如果内置支持，需要大量配置参数，反而降低了易用性。

#### 理由3：Go标准库已提供完整支持
- `crypto/tls` 包提供了完整的TLS实现
- `tls.Dial` 和 `tls.Listen` 已经封装了常用场景
- 用户可以直接使用标准库，在Dialer中调用

#### 理由4：符合单一职责原则
- **连接池的职责**：管理连接的获取、归还、健康检查、清理
- **Dialer的职责**：创建连接（包括TCP、TLS、WebSocket等）
- 职责分离使代码更清晰、更易维护

#### 理由5：用户有更多控制权
- 可以自定义握手超时
- 可以处理握手过程中的特殊逻辑
- 可以记录TLS握手相关的日志
- 可以根据不同服务器使用不同的TLS配置

### ✅ 建议：提供TLS使用示例

虽然不需要内置支持，但应该：
1. **提供完整的TLS使用示例**（已在 `examples/tls/` 目录创建）
2. **在README中说明TLS使用方法**
3. **提供常见TLS场景的最佳实践**

## 推荐做法

### 对于用户

1. **客户端场景**：
   ```go
   config.Dialer = func(ctx context.Context) (any, error) {
       return tls.DialWithDialer(&net.Dialer{Timeout: 5*time.Second}, 
                                  "tcp", "example.com:443", 
                                  &tls.Config{ServerName: "example.com"})
   }
   ```

2. **服务器场景**：
   ```go
   listener, _ := tls.Listen("tcp", ":8443", tlsConfig)
   config.Listener = listener
   ```

3. **自定义健康检查**：
   ```go
   config.HealthChecker = func(conn any) bool {
       tlsConn, ok := conn.(*tls.Conn)
       if !ok || !tlsConn.ConnectionState().HandshakeComplete {
           return false
       }
       // 进一步检查连接健康状态
       return true
   }
   ```

### 对于项目维护者

1. ✅ **保持当前架构不变**
2. ✅ **维护TLS使用示例**
3. ✅ **在文档中说明TLS使用方法**
4. ✅ **可以考虑添加TLS相关的工具函数**（可选，非必需）
   - 例如：`NewTLSDialer()` 辅助函数，返回一个配置好的Dialer

## 总结

- ✅ **当前架构已支持TLS**，用户可以在Dialer/Acceptor中实现
- ❌ **不建议内置TLS支持**，保持架构简洁和灵活性
- ✅ **建议提供示例和文档**，帮助用户快速上手

这种设计既保证了功能的完整性，又保持了代码的简洁性和灵活性，是最佳的架构选择。

