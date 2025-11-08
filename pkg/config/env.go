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

	"github.com/pkg/errors"
)

// LoadNATConfigFromEnv 从环境变量加载NAT配置
//
// 支持以下环境变量：
//   - NAT_CONFIG_PATH: NAT配置文件路径（YAML格式）
//
// 如果NAT_CONFIG_PATH未设置，返回错误。
// 该函数是环境变量配置和文件配置的桥接层。
//
// 返回：
//   - *NATConfig: 解析后的NAT配置
//   - error: 环境变量缺失或文件加载错误
//
// 示例：
//
//	// 设置环境变量: export NAT_CONFIG_PATH=/etc/nat/config.yaml
//	natCfg, err := LoadNATConfigFromEnv()
//	if err != nil {
//	    log.Fatalf("Failed to load NAT config from env: %v", err)
//	}
func LoadNATConfigFromEnv() (*NATConfig, error) {
	// 读取NAT配置文件路径环境变量
	configPath := os.Getenv("NAT_CONFIG_PATH")
	if configPath == "" {
		return nil, errors.New("NAT_CONFIG_PATH environment variable is not set")
	}

	// 从文件加载NAT配置
	natCfg, err := LoadNATConfigFromFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load NAT config from path: %s", configPath)
	}

	return natCfg, nil
}

// MustLoadNATConfigFromEnv 从环境变量加载NAT配置（失败时panic）
//
// 与LoadNATConfigFromEnv功能相同，但在加载失败时直接panic。
// 适用于应用启动时必须加载配置的场景。
//
// 返回：
//   - *NATConfig: 解析后的NAT配置
//
// 示例：
//
//	// 应用启动时使用，配置加载失败直接终止
//	natCfg := MustLoadNATConfigFromEnv()
func MustLoadNATConfigFromEnv() *NATConfig {
	natCfg, err := LoadNATConfigFromEnv()
	if err != nil {
		panic(errors.Wrap(err, "failed to load NAT config from environment"))
	}
	return natCfg
}
