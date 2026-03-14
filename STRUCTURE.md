# 项目结构说明

[English](STRUCTURE_en.md) | 中文

```
netconnpool/
├── .gemini/                      # 代码审核和优化文档
│   ├── code_audit_report.md     # 详细的代码审核报告
│   ├── audit_summary.md         # 审核总结
│   └── optimization_complete.md # 优化完成报告
│
├── examples/                     # 示例代码
│   ├── comprehensive_demo.go    # 综合功能演示
│   ├── ipv6_example.go          # IPv6 连接示例
│   └── tls/                     # TLS 相关示例
│       ├── tls_example.go       # TLS 客户端示例
│       └── tls_server_example.go # TLS 服务器示例
│
├── test/                        # 测试文件目录
│   ├── README.md                # 测试说明文档
│   └── pool_race_test.go        # 并发安全性测试
│
├── cleanup.go                   # 连接清理管理器
├── config.go                    # 配置结构和验证
├── connection.go                # 连接封装和生命周期管理
├── errors.go                    # 错误定义
├── health.go                    # 健康检查管理器
├── ipversion.go                 # IP 版本检测
├── leak.go                      # 连接泄露检测器
├── mode.go                      # 连接池模式定义
├── pool.go                      # 核心连接池实现
├── protocol.go                  # 协议类型检测
├── stats.go                     # 统计信息收集器
├── udp_utils.go                 # UDP 工具函数
├── go.mod                       # Go 模块定义
├── go.sum                       # 依赖校验和
├── LICENSE                      # BSD-3-Clause 许可证
└── README.md                    # 项目文档

```

## 核心文件说明

### 连接池核心
- **pool.go**: 连接池的核心实现，包含连接获取、归还、创建等逻辑
- **connection.go**: 连接对象的封装，提供线程安全的连接信息访问
- **config.go**: 配置结构体和默认配置，支持客户端/服务器端两种模式

### 管理器
- **health.go**: 健康检查管理器，定期检查空闲连接的健康状态
- **cleanup.go**: 清理管理器，定期清理过期和不健康的连接
- **leak.go**: 泄露检测器，检测长时间未归还的连接

### 工具和辅助
- **stats.go**: 统计信息收集器，提供详细的连接池使用统计
- **protocol.go**: 协议类型检测（TCP/UDP）
- **ipversion.go**: IP 版本检测（IPv4/IPv6）
- **udp_utils.go**: UDP 特定的工具函数，如缓冲区清理
- **errors.go**: 错误定义和常量
- **mode.go**: 连接池模式定义（客户端/服务器端）

## 目录说明

### examples/
包含各种使用场景的示例代码：
- 基本的 TCP/UDP 连接池使用
- IPv6 连接支持
- TLS 加密连接
- 生命周期钩子使用
- 混合协议处理

### test/
包含所有测试文件：
- 并发安全性测试
- 竞态条件检测
- 等待队列测试
- 健康检查测试

使用外部测试包 (`netconnpool_test`)，确保只测试公开 API。

### .gemini/
代码审核和优化过程的文档：
- 发现的问题清单
- 修复方案
- 优化建议
- 测试结果

## 代码组织原则

1. **关注点分离**: 每个文件负责特定的功能模块
2. **清晰的命名**: 文件名直接反映其功能
3. **模块化设计**: 管理器独立实现，易于测试和维护
4. **文档齐全**: 每个目录都有相应的 README 说明

## 开发建议

- 核心逻辑修改：主要关注 `pool.go` 和 `connection.go`
- 添加新功能：考虑创建新的管理器文件
- 性能优化：关注 `stats.go` 和锁的使用
- 添加测试：在 `test/` 目录下创建新的测试文件
- 添加示例：在 `examples/` 目录下创建新的示例程序
