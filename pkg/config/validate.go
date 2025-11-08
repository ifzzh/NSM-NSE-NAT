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

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
)

// ValidateNATConfig 验证NAT配置的完整性和有效性
//
// 实现data-model.md和nat-config-schema.yaml中定义的所有验证规则：
//   - 必填字段检查（name, natIP, snatRules）
//   - IP地址格式验证
//   - 端口范围验证
//   - CIDR格式验证
//   - 协议枚举验证
//
// 参数：
//   - cfg: 待验证的NAT配置
//
// 返回：
//   - error: 第一个发现的验证错误，如果配置有效则返回nil
//
// 示例：
//
//	natCfg, _ := LoadNATConfigFromFile("config.yaml")
//	if err := ValidateNATConfig(natCfg); err != nil {
//	    log.Fatalf("Invalid NAT config: %v", err)
//	}
func ValidateNATConfig(cfg *NATConfig) error {
	if cfg == nil {
		return errors.New("NAT config cannot be nil")
	}

	// 验证必填字段
	if err := validateRequiredFields(cfg); err != nil {
		return err
	}

	// 验证natIP格式
	if err := validateIPAddress(cfg.NatIP, "natIP"); err != nil {
		return err
	}

	// 验证端口范围
	if err := validatePortRange(cfg.PortRange); err != nil {
		return err
	}

	// 验证SNAT规则
	if err := validateSNATRules(cfg.SnatRules); err != nil {
		return err
	}

	// 验证DNAT规则（如果存在）
	if len(cfg.DnatRules) > 0 {
		if err := validateDNATRules(cfg.DnatRules); err != nil {
			return err
		}
	}

	// 验证超时配置（如果存在）
	if cfg.Timeouts != nil {
		if err := validateTimeouts(cfg.Timeouts); err != nil {
			return err
		}
	}

	return nil
}

// validateRequiredFields 验证必填字段
func validateRequiredFields(cfg *NATConfig) error {
	if cfg.Name == "" {
		return errors.New("field 'name' is required")
	}

	if cfg.NatIP == "" {
		return errors.New("field 'natIP' is required")
	}

	if len(cfg.SnatRules) == 0 {
		return errors.New("field 'snatRules' must contain at least one rule")
	}

	return nil
}

// validateIPAddress 验证IP地址格式
func validateIPAddress(ipStr string, fieldName string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("field '%s' has invalid IP address format: %s", fieldName, ipStr)
	}

	// 验证是IPv4地址
	if ip.To4() == nil {
		return fmt.Errorf("field '%s' must be an IPv4 address: %s", fieldName, ipStr)
	}

	return nil
}

// validatePortRange 验证端口范围配置
func validatePortRange(pr *PortRange) error {
	if pr == nil {
		return nil // 端口范围是可选的
	}

	// 验证端口值在有效范围内
	if pr.Start == 0 || pr.Start > 65535 {
		return fmt.Errorf("portRange.start must be between 1 and 65535, got: %d", pr.Start)
	}

	if pr.End == 0 || pr.End > 65535 {
		return fmt.Errorf("portRange.end must be between 1 and 65535, got: %d", pr.End)
	}

	// 验证start <= end
	if pr.Start > pr.End {
		return fmt.Errorf("portRange.start (%d) must be <= portRange.end (%d)", pr.Start, pr.End)
	}

	return nil
}

// validateSNATRules 验证SNAT规则列表
func validateSNATRules(rules []SNATRule) error {
	if len(rules) == 0 {
		return errors.New("snatRules cannot be empty")
	}

	for i, rule := range rules {
		if err := validateSNATRule(rule, i); err != nil {
			return err
		}
	}

	return nil
}

// validateSNATRule 验证单条SNAT规则
func validateSNATRule(rule SNATRule, index int) error {
	if rule.SrcNet == "" {
		return fmt.Errorf("snatRules[%d].srcNet is required", index)
	}

	// 验证CIDR格式
	_, _, err := net.ParseCIDR(rule.SrcNet)
	if err != nil {
		return fmt.Errorf("snatRules[%d].srcNet has invalid CIDR format '%s': %v", index, rule.SrcNet, err)
	}

	return nil
}

// validateDNATRules 验证DNAT规则列表
func validateDNATRules(rules []DNATRule) error {
	// 用于检测端口冲突
	externalEndpoints := make(map[string]bool)

	for i, rule := range rules {
		if err := validateDNATRule(rule, i); err != nil {
			return err
		}

		// 检测DNAT规则冲突（同一个externalIP:port不能映射到多个内部服务）
		endpoint := fmt.Sprintf("%s:%s:%d", strings.ToLower(rule.Protocol), rule.ExternalIP, rule.ExternalPort)
		if externalEndpoints[endpoint] {
			return fmt.Errorf("dnatRules[%d]: duplicate DNAT mapping for %s:%d (protocol: %s)", i, rule.ExternalIP, rule.ExternalPort, rule.Protocol)
		}
		externalEndpoints[endpoint] = true
	}

	return nil
}

// validateDNATRule 验证单条DNAT规则
func validateDNATRule(rule DNATRule, index int) error {
	// 验证externalIP
	if err := validateIPAddress(rule.ExternalIP, fmt.Sprintf("dnatRules[%d].externalIP", index)); err != nil {
		return err
	}

	// 验证internalIP
	if err := validateIPAddress(rule.InternalIP, fmt.Sprintf("dnatRules[%d].internalIP", index)); err != nil {
		return err
	}

	// 验证端口
	if rule.ExternalPort == 0 || rule.ExternalPort > 65535 {
		return fmt.Errorf("dnatRules[%d].externalPort must be between 1 and 65535, got: %d", index, rule.ExternalPort)
	}

	if rule.InternalPort == 0 || rule.InternalPort > 65535 {
		return fmt.Errorf("dnatRules[%d].internalPort must be between 1 and 65535, got: %d", index, rule.InternalPort)
	}

	// 验证协议
	protocol := strings.ToLower(rule.Protocol)
	if protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("dnatRules[%d].protocol must be 'tcp' or 'udp', got: '%s'", index, rule.Protocol)
	}

	return nil
}

// validateTimeouts 验证NAT超时配置
func validateTimeouts(timeouts *NATTimeouts) error {
	if timeouts == nil {
		return nil
	}

	// 所有超时值必须>0
	if timeouts.TcpEstablished > 0 && timeouts.TcpEstablished < 60 {
		return fmt.Errorf("timeouts.tcpEstablished should be >= 60 seconds, got: %d", timeouts.TcpEstablished)
	}

	if timeouts.TcpTransitory > 0 && timeouts.TcpTransitory < 30 {
		return fmt.Errorf("timeouts.tcpTransitory should be >= 30 seconds, got: %d", timeouts.TcpTransitory)
	}

	if timeouts.Udp > 0 && timeouts.Udp < 30 {
		return fmt.Errorf("timeouts.udp should be >= 30 seconds, got: %d", timeouts.Udp)
	}

	if timeouts.Icmp > 0 && timeouts.Icmp < 10 {
		return fmt.Errorf("timeouts.icmp should be >= 10 seconds, got: %d", timeouts.Icmp)
	}

	return nil
}
