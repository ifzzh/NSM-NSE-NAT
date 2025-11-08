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
	"fmt"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/pkg/errors"
)

// ExtractInterfaceIndex 从连接元数据中提取VPP接口索引
//
// NSM连接的元数据中包含VPP接口相关信息,此函数提取VPP接口索引用于NAT配置。
// 接口索引存储在Connection.Mechanism的参数中。
//
// 参数:
//   - conn: NSM网络服务连接对象
//
// 返回:
//   - uint32: VPP接口索引
//   - error: 如果元数据缺失或格式错误
//
// 示例:
//
//	swIfIndex, err := ExtractInterfaceIndex(conn)
//	if err != nil {
//	    log.Fatalf("无法提取接口索引: %v", err)
//	}
func ExtractInterfaceIndex(conn *networkservice.Connection) (uint32, error) {
	if conn == nil {
		return 0, errors.New("connection is nil")
	}

	// 检查机制是否存在
	if conn.GetMechanism() == nil {
		return 0, errors.New("connection mechanism is nil")
	}

	// 从机制参数中提取VPP接口索引
	// NSM使用 "vpp_sw_if_index" 键存储VPP接口索引
	params := conn.GetMechanism().GetParameters()
	if params == nil {
		return 0, errors.New("mechanism parameters are nil")
	}

	swIfIndexStr, ok := params["vpp_sw_if_index"]
	if !ok {
		return 0, errors.New("vpp_sw_if_index not found in mechanism parameters")
	}

	// 解析接口索引字符串
	var swIfIndex uint32
	_, err := fmt.Sscanf(swIfIndexStr, "%d", &swIfIndex)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse vpp_sw_if_index: %s", swIfIndexStr)
	}

	return swIfIndex, nil
}

// VerifyInterfaceMapping 验证inside/outside接口映射的正确性
//
// 验证规则(FR-023):
//   - Server侧连接(来自NSC)必须映射为NAT inside接口
//   - Client侧连接(到外网)必须映射为NAT outside接口
//   - 不允许两个接口使用相同的VPP索引
//
// 参数:
//   - serverSideIfIndex: Server侧接口索引(inside)
//   - clientSideIfIndex: Client侧接口索引(outside)
//
// 返回:
//   - error: 如果映射验证失败
//
// 示例:
//
//	err := VerifyInterfaceMapping(serverIfIndex, clientIfIndex)
//	if err != nil {
//	    log.Fatalf("接口映射验证失败: %v", err)
//	}
func VerifyInterfaceMapping(serverSideIfIndex, clientSideIfIndex uint32) error {
	// 验证索引不为0(VPP中0通常是无效索引)
	if serverSideIfIndex == 0 {
		return errors.New("server side interface index cannot be 0")
	}

	if clientSideIfIndex == 0 {
		return errors.New("client side interface index cannot be 0")
	}

	// 验证两个接口索引不相同
	if serverSideIfIndex == clientSideIfIndex {
		return fmt.Errorf(
			"server side and client side interfaces must have different indexes, got: %d",
			serverSideIfIndex,
		)
	}

	return nil
}

// InterfacePair 表示NAT的接口对
//
// 包含Server侧(inside)和Client侧(outside)的VPP接口索引,
// 用于NAT配置的接口管理。
type InterfacePair struct {
	// ServerSideIndex Server侧接口索引(NAT inside)
	ServerSideIndex uint32

	// ClientSideIndex Client侧接口索引(NAT outside)
	ClientSideIndex uint32
}

// Validate 验证接口对的有效性
//
// 检查接口索引是否有效,是否存在冲突。
//
// 返回:
//   - error: 验证失败时返回错误
//
// 示例:
//
//	ifPair := &InterfacePair{ServerSideIndex: 1, ClientSideIndex: 2}
//	if err := ifPair.Validate(); err != nil {
//	    log.Fatalf("接口对验证失败: %v", err)
//	}
func (ip *InterfacePair) Validate() error {
	return VerifyInterfaceMapping(ip.ServerSideIndex, ip.ClientSideIndex)
}

// String 返回接口对的字符串表示
func (ip *InterfacePair) String() string {
	return fmt.Sprintf("InterfacePair{ServerSide(inside): %d, ClientSide(outside): %d}",
		ip.ServerSideIndex, ip.ClientSideIndex)
}
