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

package config

// NATConfig NAT配置顶层实体
//
// 包含所有NAT相关配置参数，对应data-model.md中的NATConfig实体。
// 用于从YAML文件或环境变量加载NAT配置。
type NATConfig struct {
	// Name NSE实例名称（如"nat-nse"）
	Name string `yaml:"name" json:"name"`

	// Labels Kubernetes标签，用于服务发现和路由
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// NatIP SNAT外部IP地址（IPv4格式）
	NatIP string `yaml:"natIP" json:"natIP"`

	// PortRange SNAT端口池范围（可选，默认1024-65535）
	PortRange *PortRange `yaml:"portRange,omitempty" json:"portRange,omitempty"`

	// SnatRules SNAT规则列表（至少1条）
	SnatRules []SNATRule `yaml:"snatRules" json:"snatRules"`

	// DnatRules DNAT规则列表（可选，P2优先级）
	DnatRules []DNATRule `yaml:"dnatRules,omitempty" json:"dnatRules,omitempty"`

	// Timeouts NAT会话超时参数（可选，P4优先级）
	Timeouts *NATTimeouts `yaml:"timeouts,omitempty" json:"timeouts,omitempty"`
}

// PortRange 端口范围配置
//
// 定义SNAT使用的端口池范围，用于PAT/NAPT端口分配。
// 对应data-model.md中的PortRange实体。
type PortRange struct {
	// Start 起始端口（默认1024，避免特权端口）
	Start uint16 `yaml:"start" json:"start"`

	// End 结束端口（默认65535）
	End uint16 `yaml:"end" json:"end"`
}

// SNATRule SNAT规则配置
//
// 定义允许进行SNAT转换的源网段。
// 对应data-model.md中的SNATRule实体。
type SNATRule struct {
	// SrcNet 源网段（CIDR格式，如"192.168.1.0/24"或"0.0.0.0/0"）
	SrcNet string `yaml:"srcNet" json:"srcNet"`
}

// DNATRule DNAT规则配置
//
// 定义外部IP/端口到内部IP/端口的静态映射（端口转发）。
// 对应data-model.md中的DNATRule实体。
// P2优先级功能。
type DNATRule struct {
	// ExternalIP 外部IP地址（公网IP）
	ExternalIP string `yaml:"externalIP" json:"externalIP"`

	// ExternalPort 外部端口
	ExternalPort uint16 `yaml:"externalPort" json:"externalPort"`

	// InternalIP 内部服务器IP地址（私有IP）
	InternalIP string `yaml:"internalIP" json:"internalIP"`

	// InternalPort 内部服务器端口
	InternalPort uint16 `yaml:"internalPort" json:"internalPort"`

	// Protocol 协议（"tcp"或"udp"）
	Protocol string `yaml:"protocol" json:"protocol"`
}

// NATTimeouts NAT会话超时参数
//
// 定义NAT会话的超时参数，用于自动清理空闲会话。
// 对应data-model.md中的NATTimeouts实体。
// P4优先级功能。
type NATTimeouts struct {
	// TcpEstablished TCP已建立连接超时（秒，默认7440约2小时）
	TcpEstablished uint32 `yaml:"tcpEstablished,omitempty" json:"tcpEstablished,omitempty"`

	// TcpTransitory TCP临时连接超时（秒，默认240即4分钟）
	TcpTransitory uint32 `yaml:"tcpTransitory,omitempty" json:"tcpTransitory,omitempty"`

	// Udp UDP超时（秒，默认300即5分钟）
	Udp uint32 `yaml:"udp,omitempty" json:"udp,omitempty"`

	// Icmp ICMP超时（秒，默认60即1分钟）
	Icmp uint32 `yaml:"icmp,omitempty" json:"icmp,omitempty"`
}

// DefaultPortRange 返回默认端口范围配置
func DefaultPortRange() *PortRange {
	return &PortRange{
		Start: 1024,
		End:   65535,
	}
}

// DefaultNATTimeouts 返回默认NAT超时配置
//
// 默认值参考VPP NAT44默认值和RFC 4787建议
func DefaultNATTimeouts() *NATTimeouts {
	return &NATTimeouts{
		TcpEstablished: 7440, // 2小时
		TcpTransitory:  240,  // 4分钟
		Udp:            300,  // 5分钟
		Icmp:           60,   // 1分钟
	}
}

// AvailablePortsCount 计算可用端口数量
func (pr *PortRange) AvailablePortsCount() int {
	if pr == nil {
		return 0
	}
	return int(pr.End) - int(pr.Start) + 1
}
