# Changelog

English | [中文](../CHANGELOG_zh.md)

## [1.0.4] - 2026-03-14

### Fixed
- Fixed Goroutine leak issue in `defaultAcceptor` function, added doneChan to ensure goroutine exits correctly
- Fixed Goroutine leak issue in `ClearUDPReadBufferNonBlocking` function, added panic recovery and connection validity check
- Fixed potential data send after channel close issue in `waitForConnectionWithProtocol` function, simplified close logic
- Fixed resource leak issue in `waitForConnection` function
- Fixed potential infinite loop issue in `GetWithProtocol` function, added failure count and nil protection
- Fixed Goroutine leak issue in `checkConnection` function, added exitChan notification mechanism
- Fixed race condition in `closeConnection` function, adjusted map delete and connection close order
- Fixed statistics inconsistency issue in `enforceMaxIdleConnections` function, accurately count actually closed connections
