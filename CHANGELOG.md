# Changelog

## [1.0.4] - 2026-03-14

### Fixed
- 修复 `defaultAcceptor` 函数中的 Goroutine 泄漏问题，添加 doneChan 确保 goroutine 正确退出
  - Fixed Goroutine leak issue in `defaultAcceptor` function, added doneChan to ensure goroutine exits correctly
- 修复 `ClearUDPReadBufferNonBlocking` 函数中的 Goroutine 泄漏问题，添加 panic 恢复和连接有效性检查
  - Fixed Goroutine leak issue in `ClearUDPReadBufferNonBlocking` function, added panic recovery and connection validity check
- 修复 `waitForConnectionWithProtocol` 函数中 channel 关闭后可能发送数据的问题，简化关闭逻辑
  - Fixed potential data send after channel close issue in `waitForConnectionWithProtocol` function, simplified close logic
- 修复 `waitForConnection` 函数中的资源泄漏问题
  - Fixed resource leak issue in `waitForConnection` function
- 修复 `GetWithProtocol` 函数中的潜在无限循环问题，添加失败计数和 nil 保护
  - Fixed potential infinite loop issue in `GetWithProtocol` function, added failure count and nil protection
- 修复 `checkConnection` 函数中的 Goroutine 泄漏问题，添加 exitChan 通知机制
  - Fixed Goroutine leak issue in `checkConnection` function, added exitChan notification mechanism
- 修复 `closeConnection` 函数中的竞态条件，调整 map 删除和连接关闭顺序
  - Fixed race condition in `closeConnection` function, adjusted map delete and connection close order
- 修复 `enforceMaxIdleConnections` 函数中的统计信息不一致问题，准确统计实际关闭的连接数
  - Fixed statistics inconsistency issue in `enforceMaxIdleConnections` function, accurately count actually closed connections
