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
	"net/url"
	"time"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/mechanisms/memif"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/up"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/xconnect"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/endpoint"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/clienturl"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/connect"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/recvfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/sendfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanismtranslation"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/passthrough"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
	"github.com/networkservicemesh/sdk/pkg/networkservice/utils/metadata"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

// Endpoint NAT网络服务端点
type Endpoint struct {
	endpoint.Endpoint
}

// Options NAT端点配置选项
type Options struct {
	// Name 端点名称
	Name string

	// ConnectTo NSM连接地址
	ConnectTo *url.URL

	// Labels 端点标签
	Labels map[string]string

	// NATConfig NAT配置
	NATConfig *config.NATConfig

	// NATConfigurator VPP NAT配置器
	NATConfigurator *vpp.NATConfigurator

	// MaxTokenLifetime token最大生命周期
	MaxTokenLifetime time.Duration

	// VPPConn VPP API连接
	VPPConn vpp.Connection

	// Source SPIFFE X509源
	Source *workloadapi.X509Source

	// ClientOptions gRPC客户端选项
	ClientOptions []grpc.DialOption
}

// NewEndpoint 创建NAT网络服务端点
//
// 创建包含完整NSM链的NAT端点，包括：
//   - NAT配置处理（SNAT/DNAT）
//   - VPP xconnect
//   - Memif机制支持
//   - 文件描述符传递
//   - 授权和token生成
//
// 参数：
//   - ctx: 上下文
//   - opts: 端点配置选项
//
// 返回值：
//   - endpoint: NAT端点实例
//
// 示例：
//
//	ep := nat.NewEndpoint(ctx, nat.Options{
//	    Name:             "nat-server",
//	    ConnectTo:        &cfg.ConnectTo,
//	    Labels:           cfg.Labels,
//	    NATConfig:        natConfig,
//	    NATConfigurator:  natCfg,
//	    MaxTokenLifetime: cfg.MaxTokenLifetime,
//	    VPPConn:          vppConn,
//	    Source:           source,
//	    ClientOptions:    clientOptions,
//	})
func NewEndpoint(ctx context.Context, opts Options) *Endpoint {
	ep := &Endpoint{}

	// 创建token生成器
	tokenGenerator := spiffejwt.TokenGeneratorFunc(opts.Source, opts.MaxTokenLifetime)

	// 构建端点链
	ep.Endpoint = endpoint.NewServer(
		ctx,
		tokenGenerator,
		endpoint.WithName(opts.Name),
		endpoint.WithAuthorizeServer(authorize.NewServer()),
		endpoint.WithAdditionalFunctionality(
			// 接收文件描述符
			recvfd.NewServer(),
			// 发送文件描述符
			sendfd.NewServer(),
			// VPP接口UP
			up.NewServer(ctx, opts.VPPConn),
			// 客户端URL传递
			clienturl.NewServer(opts.ConnectTo),
			// VPP xconnect
			xconnect.NewServer(opts.VPPConn),
			// Memif机制支持（Server侧）
			mechanisms.NewServer(map[string]networkservice.NetworkServiceServer{
				memif.MECHANISM: chain.NewNetworkServiceServer(
					memif.NewServer(ctx, opts.VPPConn),
				),
			}),
			// 连接到下游服务
			connect.NewServer(
				client.NewClient(
					ctx,
					client.WithoutRefresh(),
					client.WithName(opts.Name),
					client.WithDialOptions(opts.ClientOptions...),
					client.WithAdditionalFunctionality(
						// 元数据传递
						metadata.NewClient(),
						// 机制转换
						mechanismtranslation.NewClient(),
						// 标签透传
						passthrough.NewClient(opts.Labels),
						// VPP接口UP（客户端侧）
						up.NewClient(ctx, opts.VPPConn),
						// VPP xconnect（客户端侧）
						xconnect.NewClient(opts.VPPConn),
						// Memif机制（客户端侧）
						memif.NewClient(ctx, opts.VPPConn),
						// NAT配置应用（必须在memif.NewClient之后，此时两侧接口索引都已存储到元数据）
						NewNATClient(opts.NATConfig, opts.NATConfigurator),
						// 发送文件描述符（客户端侧）
						sendfd.NewClient(),
						// 接收文件描述符（客户端侧）
						recvfd.NewClient(),
					),
				),
			),
		),
	)

	return ep
}
