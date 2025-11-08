# NSM NAT NSE 测试指南

本目录包含用于测试 NAT NSE 的完整测试环境和脚本。

## 📦 测试镜像

- **镜像**: `ifzzh520/nse-nat-vpp:latest`
- **Docker Hub**: https://hub.docker.com/r/ifzzh520/nse-nat-vpp

## 🚀 快速开始

### 方式1: 自动化完整测试（推荐）

运行完整的自动化测试脚本，包含9个测试步骤：

```bash
cd /home/ifzzh/Project/nsm-app-20251105/NSM-NSE-NAT/samenode-nat
./test-nat.sh
```

**测试内容包括：**
1. ✅ 环境清理
2. ✅ 部署所有组件
3. ✅ 验证 Pod 状态
4. ✅ 验证 NAT NSE 注册
5. ✅ 验证网络接口创建
6. ✅ 验证 NAT 配置挂载
7. ✅ 测试 ICMP 连通性（NAT转换后应该通过）
8. ✅ 验证源 IP NAT 转换（检查Server日志）
9. ✅ 检查 VPP NAT44 会话
10. ✅ 检查 VPP 状态

### 方式2: 手动测试

手动执行测试步骤：

```bash
# 1. 部署
kubectl apply -k /home/ifzzh/Project/nsm-app-20251105/NSM-NSE-NAT/samenode-nat/

# 2. 等待就绪
kubectl wait --for=condition=ready --timeout=120s pod -l app=nse-nat-vpp -n ns-nse-composition
kubectl wait --for=condition=ready --timeout=60s pod -l app=alpine -n ns-nse-composition

# 3. 查看状态
kubectl get pods -n ns-nse-composition -o wide

# 4. 测试连通性 (NAT转换)
kubectl exec -n ns-nse-composition alpine -- ping -c 3 172.16.1.100

# 5. 检查 VPP NAT 会话
NAT_POD=$(kubectl get pod -n ns-nse-composition -l app=nse-nat-vpp -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# 6. 检查 NAT 地址池
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 addresses

# 7. 检查 NAT 接口配置
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 interfaces

# 8. 清理
kubectl delete ns ns-nse-composition
```

## 📋 NAT 配置说明

当前配置的 NAT 规则（见 `config-file.yaml`）：

```yaml
# 外部NAT IP地址 (SNAT使用的公网IP)
natIP: "203.0.113.10"

# NAT端口范围
portRange:
  start: 10000
  end: 20000

# SNAT规则: 将内部私有IP转换为外部公网IP
snatRules:
  - srcNet: "10.0.0.0/8"      # 所有10.x.x.x的流量
  - srcNet: "172.16.0.0/12"   # 所有172.16-31.x.x的流量
  - srcNet: "192.168.0.0/16"  # 所有192.168.x.x的流量
```

**SNAT工作原理**:
- 内部客户端（源IP: 10.x.x.x）发送数据包到外部服务器
- NAT NSE 将源IP转换为 `203.0.113.10`
- NAT NSE 分配端口号（10000-20000范围）
- 外部服务器看到的源IP是 `203.0.113.10:端口号`
- 返回流量经过NAT NSE反向转换回内部IP

## 🔍 故障排查

### 查看 NAT NSE 日志

```bash
kubectl logs -n ns-nse-composition deployment/nse-nat-vpp --tail=50
```

**关键日志检查点**:
- `NAT config loaded: natIP=...` - NAT配置加载成功
- `executing phase 6` - NSE注册完成
- `startup completed in ...` - 启动成功

### 查看 Pod 详细信息

```bash
kubectl describe pod -n ns-nse-composition -l app=nse-nat-vpp
```

### 检查网络接口

```bash
# 客户端接口
kubectl exec -n ns-nse-composition alpine -- ip addr show

# 服务端接口
SERVER_POD=$(kubectl get pod -n ns-nse-composition -l app=server -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n ns-nse-composition $SERVER_POD -- ip addr show
```

### 检查 VPP NAT 状态

```bash
NAT_POD=$(kubectl get pod -n ns-nse-composition -l app=nse-nat-vpp -o jsonpath='{.items[0].metadata.name}')

# VPP版本
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show version

# VPP接口列表
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show interface

# NAT44 会话（活动的NAT转换）
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# NAT44 地址池
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 addresses

# NAT44 接口配置 (inside/outside)
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 interfaces

# NAT44 统计信息
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 statistics
```

### 检查 NAT 配置文件

```bash
kubectl exec -n ns-nse-composition $NAT_POD -- cat /etc/nat/config.yaml
```

### 验证 NAT 转换

```bash
# 1. 产生流量
kubectl exec -n ns-nse-composition alpine -- ping -c 10 172.16.1.100

# 2. 检查 VPP NAT 会话
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# 3. 检查服务端日志（应该看到NAT IP: 203.0.113.10）
kubectl logs -n ns-nse-composition $SERVER_POD --tail=20
```

## 📊 测试结果示例

```
========================================
         测试结果汇总
========================================
总测试数:   9
通过:       9 (100%)
失败:       0
========================================

[✓] 所有测试通过! 🎉
[✓] NAT NSE 镜像功能正常!
```

## 🔄 与 Firewall NSE 对比

| 指标 | Firewall NSE | NAT NSE | 状态 |
|------|--------------|---------|------|
| **功能** | ACL 访问控制 | NAT 地址转换 | ✅ 不同功能 |
| **代码结构** | 模块化 pkg/* | 模块化 pkg/* | ✅ 一致 |
| **配置方式** | YAML (ACL规则) | YAML (NAT规则) | ✅ 类似 |
| **VPP插件** | ACL Plugin | NAT44 ED Plugin | ✅ 不同插件 |
| **接口角色** | N/A | inside/outside | ✅ NAT特有 |

## 📁 文件说明

```
samenode-nat/
├── test-nat.sh               # 完整自动化测试脚本
├── TEST_GUIDE.md             # 本文件
├── kustomization.yaml        # Kustomize 主配置
├── nse-nat/
│   └── nat.yaml              # NAT NSE 部署配置
├── config-file.yaml          # NAT 规则配置 (ConfigMap)
├── client.yaml               # 测试客户端
├── server.yaml               # 测试服务端 (nse-kernel)
├── sfc.yaml                  # 网络服务链配置
└── README.md                 # 项目说明
```

## 🎯 预期行为

### 1. NAT NSE 成功启动

- Pod 进入 Running 状态
- 日志显示所有6个启动阶段完成:
  ```
  executing phase 1: get config from environment
  NAT config loaded: natIP=203.0.113.10, snatRules=3, dnatRules=0
  executing phase 2: retrieve spiffe svid
  executing phase 3: create grpc client options
  executing phase 4: create nat network service endpoint
  executing phase 5: create grpc and mount nse
  executing phase 6: register nse with nsm
  startup completed in ...
  ```
- 成功注册到 NSM

### 2. 网络接口创建

- 客户端 Pod 有 `nsm-1` 接口
- 服务端 Pod 有对应的网络接口
- NAT NSE 有 `memif0/0` (inside) 和 `memif0/1` (outside) 接口

### 3. NAT 规则生效

**SNAT转换验证**:
- 客户端 ping 服务端成功 (ICMP流量被NAT转换)
- VPP NAT44 会话显示活动转换:
  ```
  vpp# show nat44 sessions
  NAT44 sessions:
    thread 0 vpp_main: 1 sessions
      i2o 10.0.0.x:xxxxx -> 203.0.113.10:xxxxx [proto: icmp]
      o2i 172.16.1.100:xxxxx -> 10.0.0.x:xxxxx [proto: icmp]
  ```
- 服务端日志（如有）显示源IP为NAT IP (`203.0.113.10`)

**NAT接口角色验证**:
```
vpp# show nat44 interfaces
NAT44 interfaces:
 memif0/0 in          # Server侧 = inside
 memif0/1 out         # Client侧 = outside
```

**NAT地址池验证**:
```
vpp# show nat44 addresses
NAT44 pool addresses:
203.0.113.10
  tenant VRF independent
  10000 busy ports
  10000 free ports
```

### 4. VPP 正常运行

- `vppctl show version` 返回版本信息
- `vppctl show interface` 显示 memif 接口
- `vppctl show nat44 sessions` 显示活动会话

## 🧪 NAT 功能测试场景

### 场景1: SNAT 基本转换

```bash
# 1. 部署NAT NSE
kubectl apply -k .

# 2. 等待就绪
kubectl wait --for=condition=ready pod -l app=nse-nat-vpp -n ns-nse-composition --timeout=120s

# 3. 产生流量
kubectl exec -n ns-nse-composition alpine -- ping -c 5 172.16.1.100

# 4. 验证NAT会话
NAT_POD=$(kubectl get pod -n ns-nse-composition -l app=nse-nat-vpp -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# 预期结果: 显示ICMP会话,源IP被转换为203.0.113.10
```

### 场景2: NAT 端口复用 (PAT/NAPT)

```bash
# 1. 并发连接测试
for i in {1..5}; do
  kubectl exec -n ns-nse-composition alpine -- ping -c 1 172.16.1.100 &
done
wait

# 2. 检查端口分配
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# 预期结果: 多个会话共享同一NAT IP,但端口号不同
```

### 场景3: NAT 配置错误处理

```bash
# 1. 创建无效配置
kubectl create configmap invalid-nat-config -n ns-nse-composition \
  --from-literal=config.yaml='natIP: "999.999.999.999"'

# 2. 尝试启动NAT NSE
kubectl apply -f nse-nat/nat.yaml

# 3. 检查日志
kubectl logs -n ns-nse-composition deployment/nse-nat-vpp

# 预期结果: 启动失败,日志显示"invalid config"错误
```

## 💡 提示

- 测试脚本会自动收集诊断信息到 `/tmp/nsm-nat-diagnostics-*` 目录
- 诊断信息包括:
  - Pod 状态
  - NAT NSE 日志
  - Client/Server 日志
  - VPP 状态（版本、接口、NAT会话、NAT地址池）
  - Kubernetes 事件
  - NAT 配置文件
- 测试完成后可以选择保留或清理测试环境
- 如果测试失败，检查 NSM 基础设施是否正常运行

## 🆘 常见问题

### Q1: NAT NSE 无法启动

**症状**: Pod一直处于 `CrashLoopBackOff` 或 `Error` 状态

**排查**:
```bash
# 检查日志
kubectl logs -n ns-nse-composition deployment/nse-nat-vpp --tail=50

# 常见原因:
# - NAT配置文件格式错误
# - NAT IP地址无效
# - VPP启动失败
# - NSM基础设施未运行
```

### Q2: NAT 会话为空

**症状**: `show nat44 sessions` 没有输出

**原因**:
1. 没有产生流量
2. NAT转换未配置
3. 会话已超时清除

**解决**:
```bash
# 产生新流量
kubectl exec -n ns-nse-composition alpine -- ping -c 5 172.16.1.100

# 立即检查会话
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions
```

### Q3: 网络接口未创建

**症状**: Client Pod 没有 `nsm-1` 接口

**排查**:
```bash
# 检查NSM组件
kubectl get pods -n nsm-system

# 检查Client Pod事件
kubectl describe pod -n ns-nse-composition alpine

# 检查NAT NSE日志
kubectl logs -n ns-nse-composition deployment/nse-nat-vpp
```

### Q4: SNAT转换未生效

**症状**: Server看到的源IP不是NAT IP

**验证**:
```bash
# 1. 检查NAT地址池
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 addresses

# 2. 检查NAT接口
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 interfaces

# 3. 检查NAT会话
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl show nat44 sessions

# 预期:
# - 地址池包含203.0.113.10
# - memif0/0 = inside, memif0/1 = outside
# - 会话显示源IP转换
```

## 🔬 高级调试

### 启用 VPP 调试日志

```bash
kubectl exec -n ns-nse-composition $NAT_POD -- vppctl set logging level debug

# 重新产生流量
kubectl exec -n ns-nse-composition alpine -- ping -c 3 172.16.1.100

# 查看详细日志
kubectl logs -n ns-nse-composition deployment/nse-nat-vpp --tail=100
```

### 抓包分析

```bash
# 在NAT NSE上抓包
kubectl exec -n ns-nse-composition $NAT_POD -- \
  timeout 10 tcpdump -i any -nn icmp -c 10

# 在Client上抓包
kubectl exec -n ns-nse-composition alpine -- \
  timeout 10 tcpdump -i nsm-1 -nn -c 10
```

### VPP Packet Trace

```bash
# 启用packet trace
kubectl exec -n ns-nse-composition $NAT_POD -- \
  vppctl trace add memif-input 10

# 产生流量
kubectl exec -n ns-nse-composition alpine -- ping -c 2 172.16.1.100

# 查看trace
kubectl exec -n ns-nse-composition $NAT_POD -- \
  vppctl show trace
```

---

**最后更新**: 2025-11-07
**测试环境**: NSM + Kubernetes
**NAT插件**: VPP NAT44 Endpoint-Dependent (ED)
