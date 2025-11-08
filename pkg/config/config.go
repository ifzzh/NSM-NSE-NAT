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
	"context"
	"net/url"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
)

// Config 包含从环境变量加载的配置参数
type Config struct {
	Name                   string            `default:"nat-server" desc:"Name of NAT Server"`
	ListenOn               string            `default:"listen.on.sock" desc:"listen on socket" split_words:"true"`
	ConnectTo              url.URL           `default:"unix:///var/lib/networkservicemesh/nsm.io.sock" desc:"url to connect to" split_words:"true"`
	MaxTokenLifetime       time.Duration     `default:"10m" desc:"maximum lifetime of tokens" split_words:"true"`
	RegistryClientPolicies []string          `default:"etc/nsm/opa/common/.*.rego,etc/nsm/opa/registry/.*.rego,etc/nsm/opa/client/.*.rego" desc:"paths to files and directories that contain registry client policies" split_words:"true"`
	ServiceName            string            `default:"" desc:"Name of providing service" split_words:"true"`
	Labels                 map[string]string `default:"" desc:"Endpoint labels"`
	NATConfigPath          string            `default:"/etc/nat/config.yaml" desc:"Path to NAT config file" split_words:"true"`
	NATConfig              *NATConfig        `desc:"Loaded NAT configuration"`
	LogLevel               string            `default:"INFO" desc:"Log level" split_words:"true"`
	OpenTelemetryEndpoint  string            `default:"otel-collector.observability.svc.cluster.local:4317" desc:"OpenTelemetry Collector Endpoint" split_words:"true"`
	MetricsExportInterval  time.Duration     `default:"10s" desc:"interval between mertics exports" split_words:"true"`
	PprofEnabled           bool              `default:"false" desc:"is pprof enabled" split_words:"true"`
	PprofListenOn          string            `default:"localhost:6060" desc:"pprof URL to ListenAndServe" split_words:"true"`
}

// Load 从环境变量加载配置，返回配置实例
//
// 使用envconfig库从环境变量中读取配置，所有配置项使用"NSM_"前缀。
// 例如：NSM_NAME, NSM_CONNECT_TO, NSM_SERVICE_NAME 等
//
// 示例：
//
//	ctx := context.Background()
//	cfg, err := config.Load(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
func Load(ctx context.Context) (*Config, error) {
	c := new(Config)

	// 打印环境变量使用说明
	if err := envconfig.Usage("nsm", c); err != nil {
		return nil, errors.Wrap(err, "cannot show usage of envconfig nsm")
	}

	// 从环境变量加载配置
	if err := envconfig.Process("nsm", c); err != nil {
		return nil, errors.Wrap(err, "cannot process envconfig nsm")
	}

	return c, nil
}

// LoadNATConfig 从YAML配置文件加载NAT配置
//
// 读取NATConfigPath指定的YAML文件，解析NAT配置（包括natIP、SNAT规则、DNAT规则等）。
// 如果文件不存在或解析失败，会返回错误。
//
// 示例：
//
//	cfg, _ := config.Load(ctx)
//	if err := cfg.LoadNATConfig(); err != nil {
//	    log.Fatalf("Failed to load NAT config: %v", err)
//	}
func (c *Config) LoadNATConfig() error {
	// 使用pkg/config中的加载函数
	natCfg, err := LoadNATConfigFromFile(c.NATConfigPath)
	if err != nil {
		return errors.Wrapf(err, "failed to load NAT config from %s", c.NATConfigPath)
	}

	// 验证NAT配置
	if err := ValidateNATConfig(natCfg); err != nil {
		return errors.Wrap(err, "invalid NAT configuration")
	}

	c.NATConfig = natCfg
	return nil
}

// Validate 验证配置的完整性和有效性
//
// 检查必填字段是否存在，URL格式是否正确。
// 返回第一个发现的验证错误。
//
// 示例：
//
//	cfg, _ := config.Load(ctx)
//	if err := cfg.Validate(); err != nil {
//	    log.Fatalf("Invalid config: %v", err)
//	}
func (c *Config) Validate() error {
	// 检查必填字段
	if c.Name == "" {
		return errors.New("Name is required")
	}
	if c.ServiceName == "" {
		return errors.New("ServiceName is required")
	}

	// 验证ConnectTo URL格式
	if c.ConnectTo.String() == "" {
		return errors.New("ConnectTo URL is required")
	}

	// 验证NAT配置已加载
	if c.NATConfig == nil {
		return errors.New("NAT configuration is required (call LoadNATConfig first)")
	}

	// 验证NAT配置有效性
	if err := ValidateNATConfig(c.NATConfig); err != nil {
		return errors.Wrap(err, "invalid NAT configuration")
	}

	return nil
}
