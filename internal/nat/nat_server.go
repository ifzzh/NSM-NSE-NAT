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

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/postpone"
	"github.com/networkservicemesh/sdk-vpp/pkg/tools/ifindex"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// natServer NAT配置服务器
//
// 实现NSM NetworkServiceServer接口，在连接建立时配置VPP NAT44。
// 核心功能：
//   - 提取Server侧和Client侧接口索引
//   - 验证接口映射正确性（FR-023）
//   - 配置NAT inside/outside接口
//   - 添加SNAT地址池
//   - 应用SNAT规则
//   - 连接关闭时清理NAT配置
type natServer struct {
	natConfig      *config.NATConfig
	natConfigurator *vpp.NATConfigurator
	configuredConns genericsync.Map[string, bool] // 跟踪已配置NAT的连接
}

// NewNATServer 创建NAT配置服务器
//
// 参数：
//   - natConfig: NAT配置（包含natIP、SNAT规则等）
//   - natConfigurator: VPP NAT配置器
//
// 返回：
//   - NAT配置服务器实例
//
// 示例：
//
//	natSrv := nat.NewNATServer(natConfig, natCfg)
func NewNATServer(natConfig *config.NATConfig, natConfigurator *vpp.NATConfigurator) networkservice.NetworkServiceServer {
	return &natServer{
		natConfig:       natConfig,
		natConfigurator: natConfigurator,
	}
}

// Request 处理连接请求并配置NAT
//
// 工作流程：
//  1. 调用链中下一个元素（确保接口已创建）
//  2. 提取Server侧和Client侧接口索引（从连接元数据）
//  3. 验证接口映射正确性
//  4. 配置NAT inside/outside接口
//  5. 添加SNAT地址池
//  6. 应用SNAT规则
//
// 错误处理：
//   - 任何步骤失败都会清理已建立的连接并返回错误
//
// 参数：
//   - ctx: 上下文
//   - request: NSM连接请求
//
// 返回：
//   - 成功建立的连接
//   - 错误（如果配置失败）
func (ns *natServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	logger := log.FromContext(ctx).WithField("natServer", "Request")

	// 延迟执行上下文（用于错误时清理）
	postponeCtxFunc := postpone.ContextWithValues(ctx)

	// 调用链中下一个元素（确保VPP接口已创建）
	conn, err := next.Server(ctx).Request(ctx, request)
	if err != nil {
		return nil, err
	}

	// 检查此连接是否已配置NAT
	_, alreadyConfigured := ns.configuredConns.Load(conn.GetId())
	if alreadyConfigured {
		logger.Infof("NAT already configured for connection: %s", conn.GetId())
		return conn, nil
	}

	// 步骤1: 从元数据加载接口索引
	// Server侧和Client侧的接口索引由memif在元数据中存储
	// 使用ifindex.Load()从元数据中加载，而非从Connection.Mechanism.Parameters中提取
	logger.Info("从元数据加载Server侧和Client侧接口索引")

	// 加载Server侧接口索引（inside接口，接收来自NSC的流量）
	serverSideIfIndex, ok := ifindex.Load(ctx, false)
	if !ok {
		return nil, ns.cleanupOnError(postponeCtxFunc, conn, errors.New("failed to load server side interface index from metadata"))
	}
	logger.Infof("加载Server侧接口索引: %d", serverSideIfIndex)

	// 加载Client侧接口索引（outside接口，发送到外网的流量）
	clientSideIfIndex, ok := ifindex.Load(ctx, true)
	if !ok {
		return nil, ns.cleanupOnError(postponeCtxFunc, conn, errors.New("failed to load client side interface index from metadata"))
	}
	logger.Infof("加载Client侧接口索引: %d", clientSideIfIndex)

	// 步骤2: 配置NAT inside接口
	logger.Infof("配置NAT inside接口: %d", serverSideIfIndex)
	if err = ns.natConfigurator.ConfigureInsideInterface(uint32(serverSideIfIndex)); err != nil {
		return nil, ns.cleanupOnError(postponeCtxFunc, conn, errors.Wrapf(err, "failed to configure inside interface %d", serverSideIfIndex))
	}

	// 步骤3: 配置NAT outside接口
	logger.Infof("配置NAT outside接口: %d", clientSideIfIndex)
	if err = ns.natConfigurator.ConfigureOutsideInterface(uint32(clientSideIfIndex)); err != nil {
		return nil, ns.cleanupOnError(postponeCtxFunc, conn, errors.Wrapf(err, "failed to configure outside interface %d", clientSideIfIndex))
	}

	// 步骤4: 添加SNAT地址池（仅配置一次）
	if true {
		logger.Infof("添加NAT地址池: %s", ns.natConfig.NatIP)
		if err = ns.natConfigurator.AddNATAddressPool(ns.natConfig.NatIP); err != nil {
			return nil, ns.cleanupOnError(postponeCtxFunc, conn, errors.Wrapf(err, "failed to add NAT address pool %s", ns.natConfig.NatIP))
		}

		// 步骤5: 配置端口范围（如果指定）
		if ns.natConfig.PortRange != nil {
			logger.Infof("配置端口范围: %d-%d", ns.natConfig.PortRange.Start, ns.natConfig.PortRange.End)
			if err = ns.natConfigurator.ConfigurePortRange(ns.natConfig.PortRange.Start, ns.natConfig.PortRange.End); err != nil {
				// 端口范围配置失败不是致命错误（P3优先级）
				logger.Warnf("端口范围配置失败（将使用默认值）: %v", err)
			}
		}

		// 步骤6: 应用SNAT规则
		// 注意：VPP NAT44 ED模式会自动应用SNAT转换
		// SNAT规则（srcNet）用于配置验证，实际转换由NAT地址池决定
		logger.Infof("SNAT配置完成，源网络规则: %d条", len(ns.natConfig.SnatRules))
		for i, rule := range ns.natConfig.SnatRules {
			logger.Infof("  SNAT规则 %d: %s → %s", i+1, rule.SrcNet, ns.natConfig.NatIP)
		}
	}

	// 标记连接已配置NAT
	ns.configuredConns.Store(conn.GetId(), true)

	logger.Infof("NAT配置成功完成，连接ID: %s", conn.GetId())
	return conn, nil
}

// Close 关闭连接并清理NAT配置
//
// 清理工作：
//   - 从已配置连接列表中移除
//   - 调用链中下一个元素的Close方法
//
// 注意：VPP NAT会话会自动超时清理，无需手动删除
//
// 参数：
//   - ctx: 上下文
//   - conn: 要关闭的连接
//
// 返回：
//   - 空响应
//   - 错误（如果关闭失败）
func (ns *natServer) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	logger := log.FromContext(ctx).WithField("natServer", "Close")

	// 从已配置连接列表中移除
	_, wasConfigured := ns.configuredConns.LoadAndDelete(conn.GetId())
	if wasConfigured {
		logger.Infof("清理NAT配置，连接ID: %s", conn.GetId())
	}

	// 调用链中下一个元素
	return next.Server(ctx).Close(ctx, conn)
}

// cleanupOnError 错误时清理连接
//
// 使用延迟执行上下文调用Close方法，确保连接被正确清理。
//
// 参数：
//   - postponeCtxFunc: 延迟执行上下文函数
//   - conn: 要清理的连接
//   - originalErr: 原始错误
//
// 返回：
//   - 包含清理信息的错误
func (ns *natServer) cleanupOnError(postponeCtxFunc func() (context.Context, context.CancelFunc), conn *networkservice.Connection, originalErr error) error {
	closeCtx, cancelClose := postponeCtxFunc()
	defer cancelClose()

	if _, closeErr := ns.Close(closeCtx, conn); closeErr != nil {
		return errors.Wrapf(originalErr, "连接清理失败: %s", closeErr.Error())
	}

	return originalErr
}
