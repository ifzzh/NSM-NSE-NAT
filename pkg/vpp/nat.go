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

package vpp

import (
	"fmt"
	"net"

	"github.com/networkservicemesh/govpp/binapi/interface_types"
	"github.com/networkservicemesh/govpp/binapi/ip_types"
	"github.com/networkservicemesh/govpp/binapi/nat44_ed"
	"github.com/networkservicemesh/govpp/binapi/nat_types"
	"github.com/pkg/errors"
	"go.fd.io/govpp/api"
)

// NATConfigurator VPP NAT44 配置器
//
// 封装VPP NAT44 API调用,提供高层次的NAT配置接口。
// 实现SNAT和DNAT功能配置,支持接口标记、地址池管理和静态映射。
//
// 参考: contracts/vpp-nat44-api.md
type NATConfigurator struct {
	vppConn api.Connection
}

// NewNATConfigurator 创建NAT配置器实例
//
// 参数:
//   - vppConn: VPP GoVPP连接,用于发送API请求
//
// 返回:
//   - *NATConfigurator: NAT配置器实例
//
// 示例:
//
//	vppConn, _ := vpp.StartAndDial(ctx)
//	natCfg := NewNATConfigurator(vppConn)
func NewNATConfigurator(vppConn api.Connection) *NATConfigurator {
	return &NATConfigurator{vppConn: vppConn}
}

// ConfigureInsideInterface 配置NAT inside接口
//
// 将指定接口标记为NAT inside(内部网络侧),用于接收内部客户端流量。
// 在NSM NAT NSE中,此接口对应Server侧接口(memif0/0),接收来自NSC的流量。
//
// 参数:
//   - swIfIndex: VPP接口索引
//
// 返回:
//   - error: VPP API调用错误或VPP返回的错误码
//
// 示例:
//
//	err := natCfg.ConfigureInsideInterface(serverSideIfIndex)
//	if err != nil {
//	    log.Fatalf("配置inside接口失败: %v", err)
//	}
func (nc *NATConfigurator) ConfigureInsideInterface(swIfIndex uint32) error {
	req := &nat44_ed.Nat44InterfaceAddDelFeature{
		IsAdd:     true, // 添加NAT特性
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Flags:     nat_types.NAT_IS_INSIDE, // 标记为inside接口
	}

	reply := &nat44_ed.Nat44InterfaceAddDelFeatureReply{}
	if err := nc.vppConn.Invoke(nil, req, reply); err != nil {
		return errors.Wrapf(err, "VPP API Nat44InterfaceAddDelFeature failed for inside interface %d", swIfIndex)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("VPP returned error code %d when configuring inside interface %d", reply.Retval, swIfIndex)
	}

	return nil
}

// ConfigureOutsideInterface 配置NAT outside接口
//
// 将指定接口标记为NAT outside(外部网络侧),用于发送转换后的外部流量。
// 在NSM NAT NSE中,此接口对应Client侧接口(memif0/1),发送流量到外网。
//
// 参数:
//   - swIfIndex: VPP接口索引
//
// 返回:
//   - error: VPP API调用错误或VPP返回的错误码
//
// 示例:
//
//	err := natCfg.ConfigureOutsideInterface(clientSideIfIndex)
//	if err != nil {
//	    log.Fatalf("配置outside接口失败: %v", err)
//	}
func (nc *NATConfigurator) ConfigureOutsideInterface(swIfIndex uint32) error {
	req := &nat44_ed.Nat44InterfaceAddDelFeature{
		IsAdd:     true, // 添加NAT特性
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Flags:     nat_types.NAT_IS_OUTSIDE, // 标记为outside接口
	}

	reply := &nat44_ed.Nat44InterfaceAddDelFeatureReply{}
	if err := nc.vppConn.Invoke(nil, req, reply); err != nil {
		return errors.Wrapf(err, "VPP API Nat44InterfaceAddDelFeature failed for outside interface %d", swIfIndex)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("VPP returned error code %d when configuring outside interface %d", reply.Retval, swIfIndex)
	}

	return nil
}

// AddNATAddressPool 添加SNAT地址池
//
// 配置SNAT使用的外部IP地址池。支持单个IP或IP范围。
// 注意:端口范围不通过此函数配置,需单独使用ConfigurePortRange()。
//
// 参数:
//   - natIP: SNAT外部IP地址(IPv4格式字符串,如"203.0.113.10")
//
// 返回:
//   - error: IP地址解析错误、VPP API调用错误或VPP返回的错误码
//
// 示例:
//
//	err := natCfg.AddNATAddressPool("203.0.113.10")
//	if err != nil {
//	    log.Fatalf("添加NAT地址池失败: %v", err)
//	}
func (nc *NATConfigurator) AddNATAddressPool(natIP string) error {
	// 解析NAT IP地址
	ip := net.ParseIP(natIP)
	if ip == nil {
		return fmt.Errorf("invalid IP address format: %s", natIP)
	}

	// 确保是IPv4地址
	ipv4 := ip.To4()
	if ipv4 == nil {
		return fmt.Errorf("NAT IP must be IPv4 address: %s", natIP)
	}

	// 转换为VPP IP4Address类型
	var vppIP ip_types.IP4Address
	copy(vppIP[:], ipv4)

	req := &nat44_ed.Nat44AddDelAddressRange{
		IsAdd:          true,   // 添加地址池
		FirstIPAddress: vppIP,  // 地址池起始IP
		LastIPAddress:  vppIP,  // 地址池结束IP(单IP时相同)
		VrfID:          0,      // VRF ID(默认0)
		Flags:          0,      // 标志位(0=默认行为)
	}

	reply := &nat44_ed.Nat44AddDelAddressRangeReply{}
	if err := nc.vppConn.Invoke(nil, req, reply); err != nil {
		return errors.Wrapf(err, "VPP API Nat44AddDelAddressRange failed for IP %s", natIP)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("VPP returned error code %d when adding NAT address pool %s", reply.Retval, natIP)
	}

	return nil
}

// ConfigurePortRange 配置NAT端口分配范围
//
// 使用VPP CLI命令配置NAT端口分配算法的端口范围。
// 此功能为P3优先级,如不配置则使用VPP默认值(1024-65535)。
//
// 参数:
//   - portStart: 起始端口(建议1024以上,避免特权端口)
//   - portEnd: 结束端口(最大65535)
//
// 返回:
//   - error: VPP CLI命令执行错误
//
// 示例:
//
//	err := natCfg.ConfigurePortRange(10000, 20000)
//	if err != nil {
//	    log.Errorf("配置端口范围失败(可能影响端口分配策略): %v", err)
//	}
func (nc *NATConfigurator) ConfigurePortRange(portStart, portEnd uint16) error {
	// 验证端口范围
	if portStart == 0 || portStart > 65535 {
		return fmt.Errorf("invalid portStart: %d (must be 1-65535)", portStart)
	}

	if portEnd == 0 || portEnd > 65535 {
		return fmt.Errorf("invalid portEnd: %d (must be 1-65535)", portEnd)
	}

	if portStart > portEnd {
		return fmt.Errorf("portStart (%d) must be <= portEnd (%d)", portStart, portEnd)
	}

	// TODO(P3): 实现VPP CLI命令调用
	// 当前版本：端口范围验证通过，但不实际配置VPP，使用默认值1024-65535
	// 未来实现：调用 vpe.CliInband API执行 "nat addr-port-assignment-alg port-range <start> - <end>"
	//
	// 示例代码(需要导入 "github.com/networkservicemesh/govpp/binapi/vpe"):
	// cliCmd := fmt.Sprintf("nat addr-port-assignment-alg port-range %d - %d", portStart, portEnd)
	// req := &vpe.CliInband{Cmd: cliCmd}
	// reply := &vpe.CliInbandReply{}
	// return nc.vppCh.SendRequest(req).ReceiveReply(reply)

	return nil
}
