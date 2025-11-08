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

package config_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	// 清理环境变量
	clearEnv(t)

	ctx := context.Background()
	cfg, err := config.Load(ctx)

	require.NoError(t, err, "Load应该成功")
	require.NotNil(t, cfg, "Config不应该为nil")

	// 验证默认值
	require.Equal(t, "firewall-server", cfg.Name)
	require.Equal(t, "listen.on.sock", cfg.ListenOn)
	require.Equal(t, "unix:///var/lib/networkservicemesh/nsm.io.sock", cfg.ConnectTo.String())
	require.Equal(t, 10*time.Minute, cfg.MaxTokenLifetime)
	require.Equal(t, "INFO", cfg.LogLevel)
	require.Equal(t, "/etc/firewall/config.yaml", cfg.ACLConfigPath)
	require.Equal(t, 10*time.Second, cfg.MetricsExportInterval)
	require.False(t, cfg.PprofEnabled)
	require.Equal(t, "localhost:6060", cfg.PprofListenOn)
}

func TestLoad_CustomValues(t *testing.T) {
	// 设置自定义环境变量
	os.Setenv("NSM_NAME", "test-firewall")
	os.Setenv("NSM_SERVICE_NAME", "test-service")
	os.Setenv("NSM_LOG_LEVEL", "DEBUG")
	os.Setenv("NSM_MAX_TOKEN_LIFETIME", "5m")
	defer clearEnv(t)

	ctx := context.Background()
	cfg, err := config.Load(ctx)

	require.NoError(t, err)
	require.Equal(t, "test-firewall", cfg.Name)
	require.Equal(t, "test-service", cfg.ServiceName)
	require.Equal(t, "DEBUG", cfg.LogLevel)
	require.Equal(t, 5*time.Minute, cfg.MaxTokenLifetime)
}

func TestValidate_Success(t *testing.T) {
	cfg := &config.Config{
		Name:        "test-server",
		ServiceName: "test-service",
		ConnectTo:   url.URL{Scheme: "unix", Path: "/test/path"},
	}

	err := cfg.Validate()
	require.NoError(t, err, "有效配置应该验证通过")
}

func TestValidate_MissingName(t *testing.T) {
	cfg := &config.Config{
		Name:        "", // 缺失
		ServiceName: "test-service",
		ConnectTo:   url.URL{Scheme: "unix", Path: "/test/path"},
	}

	err := cfg.Validate()
	require.Error(t, err, "缺少Name应该返回错误")
	require.Contains(t, err.Error(), "Name is required")
}

func TestValidate_MissingServiceName(t *testing.T) {
	cfg := &config.Config{
		Name:        "test-server",
		ServiceName: "", // 缺失
		ConnectTo:   url.URL{Scheme: "unix", Path: "/test/path"},
	}

	err := cfg.Validate()
	require.Error(t, err, "缺少ServiceName应该返回错误")
	require.Contains(t, err.Error(), "ServiceName is required")
}

func TestValidate_MissingConnectTo(t *testing.T) {
	cfg := &config.Config{
		Name:        "test-server",
		ServiceName: "test-service",
		ConnectTo:   url.URL{}, // 空URL
	}

	err := cfg.Validate()
	require.Error(t, err, "空的ConnectTo URL应该返回错误")
	require.Contains(t, err.Error(), "ConnectTo URL is required")
}

func TestLoadACLRules_ValidFile(t *testing.T) {
	// 创建临时YAML文件
	tmpDir := t.TempDir()
	aclFile := filepath.Join(tmpDir, "acl.yaml")

	yamlContent := `
rule1:
  Tag: "test-rule-1"
  Rules:
    - IsPermit: 1
      Proto: 6
      SrcPort: 0
      DstPort: 80
rule2:
  Tag: "test-rule-2"
  Rules:
    - IsPermit: 1
      Proto: 17
      SrcPort: 0
      DstPort: 53
`
	err := os.WriteFile(aclFile, []byte(yamlContent), 0600)
	require.NoError(t, err)

	// 加载ACL规则
	ctx := context.Background()
	cfg := &config.Config{
		ACLConfigPath: aclFile,
	}
	cfg.LoadACLRules(ctx)

	// 验证加载了规则（ACLRule的具体结构由外部库定义，这里只验证数量）
	require.Len(t, cfg.ACLConfig, 2, "应该加载2条ACL规则")
}

func TestLoadACLRules_FileNotFound(t *testing.T) {
	// 使用不存在的文件路径
	ctx := context.Background()
	cfg := &config.Config{
		ACLConfigPath: "/nonexistent/path/acl.yaml",
	}

	// 调用LoadACLRules不应该panic，只是记录错误
	require.NotPanics(t, func() {
		cfg.LoadACLRules(ctx)
	})

	// 没有加载任何规则
	require.Empty(t, cfg.ACLConfig)
}

func TestLoadACLRules_InvalidYAML(t *testing.T) {
	// 创建无效的YAML文件
	tmpDir := t.TempDir()
	aclFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
this is not
  valid: yaml: content:
    - broken
`
	err := os.WriteFile(aclFile, []byte(invalidContent), 0600)
	require.NoError(t, err)

	ctx := context.Background()
	cfg := &config.Config{
		ACLConfigPath: aclFile,
	}

	// 调用LoadACLRules不应该panic
	require.NotPanics(t, func() {
		cfg.LoadACLRules(ctx)
	})

	// 没有成功加载规则
	require.Empty(t, cfg.ACLConfig)
}

func TestLoadACLRules_EmptyFile(t *testing.T) {
	// 创建空文件
	tmpDir := t.TempDir()
	aclFile := filepath.Join(tmpDir, "empty.yaml")

	err := os.WriteFile(aclFile, []byte(""), 0600)
	require.NoError(t, err)

	ctx := context.Background()
	cfg := &config.Config{
		ACLConfigPath: aclFile,
	}

	cfg.LoadACLRules(ctx)

	// 空文件不应该加载任何规则
	require.Empty(t, cfg.ACLConfig)
}

// 辅助函数：清理环境变量
func clearEnv(t *testing.T) {
	envVars := []string{
		"NSM_NAME",
		"NSM_LISTEN_ON",
		"NSM_CONNECT_TO",
		"NSM_MAX_TOKEN_LIFETIME",
		"NSM_REGISTRY_CLIENT_POLICIES",
		"NSM_SERVICE_NAME",
		"NSM_LABELS",
		"NSM_ACL_CONFIG_PATH",
		"NSM_ACL_CONFIG",
		"NSM_LOG_LEVEL",
		"NSM_OPEN_TELEMETRY_ENDPOINT",
		"NSM_METRICS_EXPORT_INTERVAL",
		"NSM_PPROF_ENABLED",
		"NSM_PPROF_LISTEN_ON",
	}

	for _, v := range envVars {
		os.Unsetenv(v)
	}

	t.Cleanup(func() {
		for _, v := range envVars {
			os.Unsetenv(v)
		}
	})
}
