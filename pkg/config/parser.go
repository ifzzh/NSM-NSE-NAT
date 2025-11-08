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
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// LoadNATConfigFromFile 从YAML文件加载NAT配置
//
// 读取指定路径的YAML配置文件，解析为NATConfig结构体。
// 支持相对路径和绝对路径。
//
// 参数：
//   - configPath: YAML配置文件路径
//
// 返回：
//   - *NATConfig: 解析后的NAT配置
//   - error: 文件读取或解析错误
//
// 示例：
//
//	natCfg, err := LoadNATConfigFromFile("/etc/nat/config.yaml")
//	if err != nil {
//	    log.Fatalf("Failed to load NAT config: %v", err)
//	}
func LoadNATConfigFromFile(configPath string) (*NATConfig, error) {
	// 读取文件内容
	raw, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read NAT config file: %s", configPath)
	}

	// 解析YAML
	var natCfg NATConfig
	if err := yaml.Unmarshal(raw, &natCfg); err != nil {
		return nil, errors.Wrapf(err, "failed to parse NAT config YAML: %s", configPath)
	}

	// 应用默认值
	applyDefaults(&natCfg)

	return &natCfg, nil
}

// ParseNATConfigFromYAML 从YAML字节流解析NAT配置
//
// 直接从字节数组解析YAML配置，适用于配置已经在内存中的场景。
//
// 参数：
//   - yamlData: YAML格式的配置数据
//
// 返回：
//   - *NATConfig: 解析后的NAT配置
//   - error: 解析错误
//
// 示例：
//
//	yamlData := []byte(`
//	name: nat-nse
//	natIP: "203.0.113.10"
//	snatRules:
//	  - srcNet: "10.0.0.0/8"
//	`)
//	natCfg, err := ParseNATConfigFromYAML(yamlData)
func ParseNATConfigFromYAML(yamlData []byte) (*NATConfig, error) {
	var natCfg NATConfig
	if err := yaml.Unmarshal(yamlData, &natCfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse NAT config YAML")
	}

	// 应用默认值
	applyDefaults(&natCfg)

	return &natCfg, nil
}

// applyDefaults 为NAT配置应用默认值
//
// 对于可选字段，如果用户未提供，则使用默认值：
// - PortRange: 1024-65535
// - Timeouts: VPP默认超时值
// - Labels: 空map
func applyDefaults(cfg *NATConfig) {
	// 应用默认端口范围
	if cfg.PortRange == nil {
		cfg.PortRange = DefaultPortRange()
	} else {
		// 如果只设置了部分值，补全默认值
		if cfg.PortRange.Start == 0 {
			cfg.PortRange.Start = 1024
		}
		if cfg.PortRange.End == 0 {
			cfg.PortRange.End = 65535
		}
	}

	// 应用默认超时配置
	if cfg.Timeouts == nil {
		cfg.Timeouts = DefaultNATTimeouts()
	} else {
		// 如果只设置了部分超时值，补全默认值
		defaults := DefaultNATTimeouts()
		if cfg.Timeouts.TcpEstablished == 0 {
			cfg.Timeouts.TcpEstablished = defaults.TcpEstablished
		}
		if cfg.Timeouts.TcpTransitory == 0 {
			cfg.Timeouts.TcpTransitory = defaults.TcpTransitory
		}
		if cfg.Timeouts.Udp == 0 {
			cfg.Timeouts.Udp = defaults.Udp
		}
		if cfg.Timeouts.Icmp == 0 {
			cfg.Timeouts.Icmp = defaults.Icmp
		}
	}

	// 初始化空的Labels map
	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}

	// 初始化空的DnatRules slice（避免nil）
	if cfg.DnatRules == nil {
		cfg.DnatRules = []DNATRule{}
	}
}
