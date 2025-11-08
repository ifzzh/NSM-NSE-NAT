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

//go:build linux
// +build linux

package main

import (
	"os"
	"time"

	"github.com/edwarnicke/grpcfd"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/opentelemetry"
	"github.com/networkservicemesh/sdk/pkg/tools/pprofutils"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
	"github.com/networkservicemesh/sdk/pkg/tools/tracing"

	_ "github.com/networkservicemesh/cmd-nse-nat-vpp/internal/imports"

	"github.com/networkservicemesh/cmd-nse-nat-vpp/internal/nat"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/config"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/lifecycle"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/registry"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/server"
	"github.com/networkservicemesh/cmd-nse-nat-vpp/pkg/vpp"
)

func main() {
	// ********************************************************************************
	// 设置上下文以捕获信号
	// ********************************************************************************
	ctx, cancel := lifecycle.NotifyContext()
	defer cancel()

	// ********************************************************************************
	// 设置日志系统
	// ********************************************************************************
	// 注意：暂时使用默认日志级别，稍后从配置加载后再更新
	ctx = lifecycle.InitializeLogging(ctx, "INFO")

	// 枚举启动阶段
	log.FromContext(ctx).Infof("there are 6 phases which will be executed followed by a success message:")
	log.FromContext(ctx).Infof("the phases include:")
	log.FromContext(ctx).Infof("1: get config from environment")
	log.FromContext(ctx).Infof("2: retrieve spiffe svid")
	log.FromContext(ctx).Infof("3: create grpc client options")
	log.FromContext(ctx).Infof("4: create nat network service endpoint")
	log.FromContext(ctx).Infof("5: create grpc and mount nse")
	log.FromContext(ctx).Infof("6: register nse with nsm")
	log.FromContext(ctx).Infof("a final success message with start time duration")

	starttime := time.Now()

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 1: get config from environment")
	// ********************************************************************************
	cfg, err := config.Load(ctx)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// 加载NAT配置
	if err := cfg.LoadNATConfig(); err != nil {
		logrus.Fatalf("failed to load NAT config: %v", err)
	}
	log.FromContext(ctx).Infof("NAT config loaded: natIP=%s, snatRules=%d, dnatRules=%d",
		cfg.NATConfig.NatIP, len(cfg.NATConfig.SnatRules), len(cfg.NATConfig.DnatRules))

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logrus.Fatalf("invalid config: %v", err)
	}

	// 使用配置的日志级别重新初始化日志
	ctx = lifecycle.InitializeLogging(ctx, cfg.LogLevel)

	log.FromContext(ctx).Infof("Config: %#v", cfg)

	// ********************************************************************************
	// 配置 OpenTelemetry
	// ********************************************************************************
	if opentelemetry.IsEnabled() {
		collectorAddress := cfg.OpenTelemetryEndpoint
		spanExporter := opentelemetry.InitSpanExporter(ctx, collectorAddress)
		metricExporter := opentelemetry.InitOPTLMetricExporter(ctx, collectorAddress, cfg.MetricsExportInterval)
		o := opentelemetry.Init(ctx, spanExporter, metricExporter, cfg.Name)
		defer func() {
			if err = o.Close(); err != nil {
				log.FromContext(ctx).Error(err.Error())
			}
		}()
	}

	// ********************************************************************************
	// 配置 pprof
	// ********************************************************************************
	if cfg.PprofEnabled {
		go pprofutils.ListenAndServe(ctx, cfg.PprofListenOn)
	}

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 2: retrieving svid, check spire agent logs if this is the last line you see")
	// ********************************************************************************
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		logrus.Fatalf("error getting x509 source: %+v", err)
	}
	svid, err := source.GetX509SVID()
	if err != nil {
		logrus.Fatalf("error getting x509 svid: %+v", err)
	}
	log.FromContext(ctx).Infof("SVID: %q", svid.ID)

	// 创建TLS配置
	tlsClientConfig := server.CreateTLSClientConfig(source)
	tlsServerConfig := server.CreateTLSServerConfig(source)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 3: create grpc client options")
	// ********************************************************************************
	clientOptions := append(
		tracing.WithTracingDial(),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(source, cfg.MaxTokenLifetime))),
		),
		grpc.WithTransportCredentials(
			grpcfd.TransportCredentials(
				credentials.NewTLS(tlsClientConfig),
			),
		),
		grpcfd.WithChainStreamInterceptor(),
		grpcfd.WithChainUnaryInterceptor(),
	)

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 4: create nat network service endpoint")
	// ********************************************************************************
	vppConn, vppErrCh, err := vpp.StartAndDial(ctx)
	if err != nil {
		logrus.Fatalf("error starting VPP: %+v", err)
	}
	lifecycle.MonitorErrorChannel(ctx, cancel, vppErrCh)

	// 创建NAT配置器
	// 使用vppConn (api.Connection) 直接创建，无需Channel
	natConfigurator := vpp.NewNATConfigurator(vppConn)

	// 创建NAT端点
	natEndpoint := nat.NewEndpoint(ctx, nat.Options{
		Name:             cfg.Name,
		ConnectTo:        &cfg.ConnectTo,
		Labels:           cfg.Labels,
		NATConfig:        cfg.NATConfig,
		NATConfigurator:  natConfigurator,
		MaxTokenLifetime: cfg.MaxTokenLifetime,
		VPPConn:          vppConn,
		Source:           source,
		ClientOptions:    clientOptions,
	})

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 5: create grpc server and register nat-server")
	// ********************************************************************************
	srvResult, err := server.New(ctx, server.Options{
		TLSConfig: tlsServerConfig,
		Name:      cfg.Name,
		ListenOn:  cfg.ListenOn,
	})
	if err != nil {
		logrus.Fatalf("error creating server: %+v", err)
	}
	defer func() { _ = os.RemoveAll(srvResult.TmpDir) }()

	// 注册NAT端点到gRPC服务器
	natEndpoint.Register(srvResult.Server)

	// 监控服务器错误
	lifecycle.MonitorErrorChannel(ctx, cancel, srvResult.ErrCh)
	log.FromContext(ctx).Infof("grpc server started")

	// ********************************************************************************
	log.FromContext(ctx).Infof("executing phase 6: register nse with nsm")
	// ********************************************************************************
	registryClient, err := registry.NewClient(ctx, registry.Options{
		ConnectTo:   &cfg.ConnectTo,
		Policies:    cfg.RegistryClientPolicies,
		DialOptions: clientOptions,
	})
	if err != nil {
		logrus.Fatalf("error creating registry client: %+v", err)
	}

	nse, err := registryClient.Register(ctx, registry.RegisterSpec{
		Name:        cfg.Name,
		ServiceName: cfg.ServiceName,
		Labels:      cfg.Labels,
		URL:         srvResult.ListenURL.String(),
	})
	if err != nil {
		log.FromContext(ctx).Fatalf("unable to register nse %+v", err)
	}
	logrus.Infof("nse: %+v", nse)

	// ********************************************************************************
	log.FromContext(ctx).Infof("startup completed in %v", time.Since(starttime))
	// ********************************************************************************

	// 等待服务器退出
	<-ctx.Done()
	<-vppErrCh
}
