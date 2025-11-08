// Copyright (c) 2021-2023 Doc.ai and/or its affiliates.
//
// Copyright (c) 2023-2024 Cisco and/or its affiliates.
//
// Copyright (c) 2024 OpenInfra Foundation Europe. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nat

import (
	"context"

	"github.com/edwarnicke/genericsync"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/postpone"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// natClient NAT配置客户端
//
// 实现NSM NetworkServiceClient接口，在客户端侧配置NAT outside接口。
// 与natServer配合工作：
//   - natServer (Server侧): 配置inside接口和地址池
//   - natClient (Client侧): 配置outside接口
type natClient struct {
	natConfig       *config.NATConfig
	natConfigurator *vpp.NATConfigurator
	configuredConns genericsync.Map[string, bool] // 跟踪已配置NAT的连接
}

// NewNATClient 创建NAT配置客户端
//
// 参数：
//   - natConfig: NAT配置
//   - natConfigurator: VPP NAT配置器
//
// 返回：
//   - NAT配置客户端实例
//
// 示例：
//
//	natClient := nat.NewNATClient(natConfig, natCfg)
func NewNATClient(natConfig *config.NATConfig, natConfigurator *vpp.NATConfigurator) networkservice.NetworkServiceClient {
	return &natClient{
		natConfig:       natConfig,
		natConfigurator: natConfigurator,
	}
}

// Request 处理客户端连接请求并配置NAT outside接口
//
// 工作流程：
//  1. 调用链中下一个元素（确保接口已创建）
//  2. 提取Client侧接口索引
//  3. 配置NAT outside接口
//
// 参数：
//   - ctx: 上下文
//   - request: NSM连接请求
//   - opts: gRPC调用选项
//
// 返回：
//   - 成功建立的连接
//   - 错误（如果配置失败）
func (nc *natClient) Request(ctx context.Context, request *networkservice.NetworkServiceRequest, opts ...grpc.CallOption) (*networkservice.Connection, error) {
	logger := log.FromContext(ctx).WithField("natClient", "Request")

	// 延迟执行上下文（用于错误时清理）
	postponeCtxFunc := postpone.ContextWithValues(ctx)

	// 调用链中下一个元素（确保VPP接口已创建）
	conn, err := next.Client(ctx).Request(ctx, request, opts...)
	if err != nil {
		return nil, err
	}

	// 检查此连接是否已配置NAT
	_, alreadyConfigured := nc.configuredConns.Load(conn.GetId())
	if alreadyConfigured {
		logger.Infof("NAT already configured for connection (client side): %s", conn.GetId())
		return conn, nil
	}

	// 提取Client侧接口索引
	logger.Info("提取Client侧接口索引")
	clientSideIfIndex, err := ExtractInterfaceIndex(conn)
	if err != nil {
		return nil, nc.cleanupOnError(postponeCtxFunc, conn, errors.Wrap(err, "failed to extract client side interface index"), opts...)
	}
	logger.Infof("提取到Client侧接口索引: %d", clientSideIfIndex)

	// 配置NAT outside接口
	logger.Infof("配置NAT outside接口: %d", clientSideIfIndex)
	if err = nc.natConfigurator.ConfigureOutsideInterface(clientSideIfIndex); err != nil {
		return nil, nc.cleanupOnError(postponeCtxFunc, conn, errors.Wrapf(err, "failed to configure outside interface %d", clientSideIfIndex), opts...)
	}

	// 标记连接已配置NAT
	nc.configuredConns.Store(conn.GetId(), true)

	logger.Infof("NAT配置成功完成（客户端侧），连接ID: %s", conn.GetId())
	return conn, nil
}

// Close 关闭连接并清理NAT配置
//
// 参数：
//   - ctx: 上下文
//   - conn: 要关闭的连接
//   - opts: gRPC调用选项
//
// 返回：
//   - 空响应
//   - 错误（如果关闭失败）
func (nc *natClient) Close(ctx context.Context, conn *networkservice.Connection, opts ...grpc.CallOption) (*empty.Empty, error) {
	logger := log.FromContext(ctx).WithField("natClient", "Close")

	// 从已配置连接列表中移除
	_, wasConfigured := nc.configuredConns.LoadAndDelete(conn.GetId())
	if wasConfigured {
		logger.Infof("清理NAT配置（客户端侧），连接ID: %s", conn.GetId())
	}

	// 调用链中下一个元素
	return next.Client(ctx).Close(ctx, conn, opts...)
}

// cleanupOnError 错误时清理连接
//
// 参数：
//   - postponeCtxFunc: 延迟执行上下文函数
//   - conn: 要清理的连接
//   - originalErr: 原始错误
//   - opts: gRPC调用选项
//
// 返回：
//   - 包含清理信息的错误
func (nc *natClient) cleanupOnError(postponeCtxFunc func() (context.Context, context.CancelFunc), conn *networkservice.Connection, originalErr error, opts ...grpc.CallOption) error {
	closeCtx, cancelClose := postponeCtxFunc()
	defer cancelClose()

	if _, closeErr := nc.Close(closeCtx, conn, opts...); closeErr != nil {
		return errors.Wrapf(originalErr, "连接清理失败（客户端侧）: %s", closeErr.Error())
	}

	return originalErr
}
