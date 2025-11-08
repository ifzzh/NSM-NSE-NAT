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
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk-vpp/pkg/tools/ifindex"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// natClient NAT Client组件
//
// 在Client链中运行,负责配置NAT outside接口和SNAT地址池。
// 配合NAT Server(在Server链中运行)共同完成NAT功能。
//
// 架构模式(参考xconnect和firewall):
//   - Server链: memif.NewServer() → NAT Server(配置inside) → connect.NewServer()
//   - Client链: memif.NewClient() → NAT Client(配置outside和地址池)
//
// 职责:
//   - 配置Client侧memif接口为NAT outside接口
//   - 添加SNAT地址池
//
// 依赖:
//   - 必须在memif.NewClient()之后执行
//   - 使用ifindex.Load(ctx, true)加载Client侧接口索引
type natClient struct {
	natConfig       *config.NATConfig
	natConfigurator *vpp.NATConfigurator
	configuredConns genericsync.Map[string, bool] // 跟踪已配置NAT的连接
}

// NewNATClient 创建NAT Client组件
//
// NAT Client在Client链中配置NAT outside接口和地址池。
// 必须放置在memif.NewClient()之后,确保Client侧接口索引已存储到元数据。
//
// 参数:
//   - natConfig: NAT配置(包含natIP等)
//   - natConfigurator: NAT配置器
//
// 返回值:
//   - networkservice.NetworkServiceClient: NSM Client链组件
//
// 示例(参考firewall架构):
//
//	client.WithAdditionalFunctionality(
//	    memif.NewClient(ctx, vppConn),
//	    NewNATClient(natConfig, natConfigurator),  // 在memif.NewClient之后
//	    sendfd.NewClient(),
//	    recvfd.NewClient(),
//	)
func NewNATClient(natConfig *config.NATConfig, natConfigurator *vpp.NATConfigurator) networkservice.NetworkServiceClient {
	return &natClient{
		natConfig:       natConfig,
		natConfigurator: natConfigurator,
	}
}

// Request Client端请求处理
//
// 配置NAT outside接口(Client侧memif)和SNAT地址池。
// 使用ifindex.Load(ctx, true)从元数据加载Client侧接口索引。
//
// 参数:
//   - ctx: 请求上下文
//   - request: NSM网络服务请求
//   - opts: gRPC调用选项
//
// 返回值:
//   - *networkservice.Connection: 连接对象
//   - error: 错误信息(如接口索引加载失败或NAT配置失败)
func (nc *natClient) Request(ctx context.Context, request *networkservice.NetworkServiceRequest, opts ...grpc.CallOption) (*networkservice.Connection, error) {
	logger := log.FromContext(ctx).WithField("natClient", "Request")

	// 检查此连接是否已配置NAT
	connID := request.GetConnection().GetId()
	_, alreadyConfigured := nc.configuredConns.Load(connID)
	if alreadyConfigured {
		logger.Infof("NAT已配置,跳过重复配置,连接ID: %s", connID)
		return next.Client(ctx).Request(ctx, request, opts...)
	}

	// 步骤1: 从元数据加载Client侧接口索引
	logger.Info("从元数据加载Client侧接口索引")
	clientSideIfIndex, ok := ifindex.Load(ctx, true) // true = Client侧
	if !ok {
		return nil, errors.New("failed to load client side interface index from metadata")
	}
	logger.Infof("加载Client侧接口索引: %d", clientSideIfIndex)

	// 步骤2: 配置NAT outside接口
	logger.Infof("配置NAT outside接口: %d", clientSideIfIndex)
	if err := nc.natConfigurator.ConfigureOutsideInterface(uint32(clientSideIfIndex)); err != nil {
		return nil, errors.Wrapf(err, "failed to configure NAT outside interface %d", clientSideIfIndex)
	}

	// 步骤3: 添加SNAT地址池
	logger.Infof("添加NAT地址池: %s", nc.natConfig.NatIP)
	if err := nc.natConfigurator.AddNATAddressPool(nc.natConfig.NatIP); err != nil {
		return nil, errors.Wrapf(err, "failed to add NAT address pool %s", nc.natConfig.NatIP)
	}

	// 标记连接已配置NAT
	nc.configuredConns.Store(connID, true)

	logger.Info("NAT outside接口和地址池配置完成")

	// 步骤4: 调用下一个Client链节点
	return next.Client(ctx).Request(ctx, request, opts...)
}

// Close Client端关闭处理
//
// 清理NAT配置记录。
//
// 参数:
//   - ctx: 请求上下文
//   - conn: 连接对象
//   - opts: gRPC调用选项
//
// 返回值:
//   - *empty.Empty: 空响应
//   - error: 错误信息
func (nc *natClient) Close(ctx context.Context, conn *networkservice.Connection, opts ...grpc.CallOption) (*empty.Empty, error) {
	logger := log.FromContext(ctx).WithField("natClient", "Close")

	// 从已配置连接列表中移除
	_, wasConfigured := nc.configuredConns.LoadAndDelete(conn.GetId())
	if wasConfigured {
		logger.Infof("清理NAT配置(Client侧),连接ID: %s", conn.GetId())
	}

	// 调用下一个Client链节点
	return next.Client(ctx).Close(ctx, conn, opts...)
}
