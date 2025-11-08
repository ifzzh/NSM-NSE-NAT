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
	"github.com/networkservicemesh/sdk-vpp/pkg/tools/ifindex"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// natClient NAT配置客户端
//
// 实现NSM NetworkServiceClient接口，负责配置NAT inside和outside接口。
// 在Client链中执行，可以访问Server侧和Client侧的接口索引：
//   - Server侧接口索引由memif.NewServer()存储到metadata
//   - Client侧接口索引由memif.NewClient()存储到metadata
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

// Request 处理客户端连接请求并配置NAT inside和outside接口
//
// 工作流程：
//  1. 从metadata加载Server侧接口索引（inside接口）
//  2. 从metadata加载Client侧接口索引（outside接口）
//  3. 配置NAT inside接口
//  4. 配置NAT outside接口
//  5. 添加NAT地址池
//  6. 调用链中下一个元素
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

	// 步骤1: 从元数据加载接口索引
	// Server侧和Client侧的接口索引由memif在元数据中存储
	// Server侧接口由memif.NewServer()存储，Client侧接口由memif.NewClient()存储
	logger.Info("从元数据加载Server侧和Client侧接口索引")

	// 加载Server侧接口索引（inside接口，接收来自NSC的流量）
	serverSideIfIndex, ok := ifindex.Load(ctx, false)
	if !ok {
		return nil, errors.New("failed to load server side interface index from metadata")
	}
	logger.Infof("加载Server侧接口索引: %d", serverSideIfIndex)

	// 加载Client侧接口索引（outside接口，发送到外网的流量）
	clientSideIfIndex, ok := ifindex.Load(ctx, true)
	if !ok {
		return nil, errors.New("failed to load client side interface index from metadata")
	}
	logger.Infof("加载Client侧接口索引: %d", clientSideIfIndex)

	// 步骤2: 配置NAT inside接口
	logger.Infof("配置NAT inside接口: %d", serverSideIfIndex)
	if err := nc.natConfigurator.ConfigureInsideInterface(uint32(serverSideIfIndex)); err != nil {
		return nil, errors.Wrapf(err, "failed to configure inside interface %d", serverSideIfIndex)
	}

	// 步骤3: 配置NAT outside接口
	logger.Infof("配置NAT outside接口: %d", clientSideIfIndex)
	if err := nc.natConfigurator.ConfigureOutsideInterface(uint32(clientSideIfIndex)); err != nil {
		return nil, errors.Wrapf(err, "failed to configure outside interface %d", clientSideIfIndex)
	}

	// 步骤4: 添加SNAT地址池（仅配置一次）
	logger.Infof("添加NAT地址池: %s", nc.natConfig.NatIP)
	if err := nc.natConfigurator.AddNATAddressPool(nc.natConfig.NatIP); err != nil {
		return nil, errors.Wrapf(err, "failed to add NAT address pool %s", nc.natConfig.NatIP)
	}

	// 标记连接已配置NAT
	nc.configuredConns.Store(request.GetConnection().GetId(), true)

	logger.Info("NAT基本配置完成")

	// 步骤5: 调用链中的下一个元素
	return next.Client(ctx).Request(ctx, request, opts...)
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

