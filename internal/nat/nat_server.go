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

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk-vpp/pkg/tools/ifindex"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/pkg/errors"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// natServer NAT Server组件
//
// 在Server链中运行,负责配置NAT inside接口(Server侧memif)。
// 配合NAT Client(在Client链中运行)共同完成NAT功能。
//
// 架构模式(参考xconnect和firewall):
//   - Server链: memif.NewServer() → NAT Server → connect.NewServer()
//   - Client链: memif.NewClient() → NAT Client
//
// 职责:
//   - 配置Server侧memif接口为NAT inside接口
//
// 依赖:
//   - 必须在memif.NewServer()之后执行
//   - 使用ifindex.Load(ctx, false)加载Server侧接口索引
type natServer struct {
	natConfigurator *vpp.NATConfigurator
}

// NewNATServer 创建NAT Server组件
//
// NAT Server在Server链中配置NAT inside接口。
// 必须放置在memif.NewServer()之后,确保Server侧接口索引已存储到元数据。
//
// 参数:
//   - natConfigurator: NAT配置器接口
//
// 返回值:
//   - networkservice.NetworkServiceServer: NSM Server链组件
//
// 示例(参考firewall架构):
//
//	mechanisms.NewServer(map[string]networkservice.NetworkServiceServer{
//	    memif.MECHANISM: chain.NewNetworkServiceServer(
//	        memif.NewServer(ctx, vppConn),
//	        NewNATServer(natConfigurator),  // 在memif.NewServer之后
//	    ),
//	}),
func NewNATServer(natConfigurator *vpp.NATConfigurator) networkservice.NetworkServiceServer {
	return &natServer{
		natConfigurator: natConfigurator,
	}
}

// Request Server端请求处理
//
// 配置NAT inside接口(Server侧memif)。
// 使用ifindex.Load(ctx, false)从元数据加载Server侧接口索引。
//
// 参数:
//   - ctx: 请求上下文
//   - request: NSM网络服务请求
//
// 返回值:
//   - *networkservice.Connection: 连接对象
//   - error: 错误信息(如接口索引加载失败或NAT配置失败)
func (ns *natServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	logger := log.FromContext(ctx).WithField("natServer", "Request")

	// 步骤1: 从元数据加载Server侧接口索引
	logger.Info("从元数据加载Server侧接口索引")
	serverSideIfIndex, ok := ifindex.Load(ctx, false) // false = Server侧
	if !ok {
		return nil, errors.New("failed to load server side interface index from metadata")
	}
	logger.Infof("加载Server侧接口索引: %d", serverSideIfIndex)

	// 步骤2: 配置NAT inside接口
	logger.Infof("配置NAT inside接口: %d", serverSideIfIndex)
	if err := ns.natConfigurator.ConfigureInsideInterface(uint32(serverSideIfIndex)); err != nil {
		return nil, errors.Wrapf(err, "failed to configure NAT inside interface %d", serverSideIfIndex)
	}

	logger.Info("NAT inside接口配置完成")

	// 步骤3: 调用下一个Server链节点
	return next.Server(ctx).Request(ctx, request)
}

// Close Server端关闭处理
//
// 当前版本不执行NAT清理操作,直接传递到下一个链节点。
//
// 参数:
//   - ctx: 请求上下文
//   - conn: 连接对象
//
// 返回值:
//   - *empty.Empty: 空响应
//   - error: 错误信息
func (ns *natServer) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	return next.Server(ctx).Close(ctx, conn)
}
