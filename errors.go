// Copyright (c) 2025, vistone
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

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
