package netconnpool

import "errors"

// 连接池相关错误定义
var (
	// ErrPoolClosed 连接池已关闭
	ErrPoolClosed = errors.New("连接池已关闭")

	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("连接已关闭")

	// ErrGetConnectionTimeout 获取连接超时
	ErrGetConnectionTimeout = errors.New("获取连接超时")

	// ErrMaxConnectionsReached 已达到最大连接数
	ErrMaxConnectionsReached = errors.New("已达到最大连接数限制")

	// ErrInvalidConnection 无效连接
	ErrInvalidConnection = errors.New("无效连接")

	// ErrConnectionUnhealthy 连接不健康
	ErrConnectionUnhealthy = errors.New("连接不健康")

	// ErrInvalidConfig 配置无效
	ErrInvalidConfig = errors.New("配置参数无效")

	// ErrConnectionLeaked 连接泄漏
	ErrConnectionLeaked = errors.New("连接泄漏检测：连接未在超时时间内归还")

	// ErrPoolExhausted 连接池耗尽
	ErrPoolExhausted = errors.New("连接池已耗尽，无法创建新连接")

	// ErrUnsupportedIPVersion 不支持的IP版本
	ErrUnsupportedIPVersion = errors.New("不支持的IP版本")

	// ErrNoConnectionForIPVersion 指定IP版本没有可用连接
	ErrNoConnectionForIPVersion = errors.New("指定IP版本没有可用连接")
	
	// ErrUnsupportedProtocol 不支持的协议
	ErrUnsupportedProtocol = errors.New("不支持的协议类型")
	
	// ErrNoConnectionForProtocol 指定协议没有可用连接
	ErrNoConnectionForProtocol = errors.New("指定协议没有可用连接")
)
