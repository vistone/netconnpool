# 更新日志

## [1.0.4] - 2026-03-14

### 修复
- 修复 `defaultAcceptor` 函数中的 Goroutine 泄漏问题，添加 doneChan 确保 goroutine 正确退出
- 修复 `ClearUDPReadBufferNonBlocking` 函数中的 Goroutine 泄漏问题，添加 panic 恢复和连接有效性检查
- 修复 `waitForConnectionWithProtocol` 函数中 channel 关闭后可能发送数据的问题，简化关闭逻辑
- 修复 `waitForConnection` 函数中的资源泄漏问题
- 修复 `GetWithProtocol` 函数中的潜在无限循环问题，添加失败计数和 nil 保护
- 修复 `checkConnection` 函数中的 Goroutine 泄漏问题，添加 exitChan 通知机制
- 修复 `closeConnection` 函数中的竞态条件，调整 map 删除和连接关闭顺序
- 修复 `enforceMaxIdleConnections` 函数中的统计信息不一致问题，准确统计实际关闭的连接数
