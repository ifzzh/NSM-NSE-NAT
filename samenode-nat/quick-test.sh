#!/bin/bash
#
# NSM Firewall 快速测试脚本 - 仅验证核心功能
#
set -e

NAMESPACE="ns-nse-composition"

echo "🚀 快速测试 NSM Firewall NSE (重构版)"
echo "========================================="

# 1. 部署
echo ""
echo "📦 步骤1: 部署环境..."
kubectl apply -k /home/ifzzh/Project/nsm-nse-app/samenode-firewall-refactored/

# 2. 等待就绪
echo ""
echo "⏳ 步骤2: 等待 Pod 就绪..."
echo "  等待 Firewall NSE..."
kubectl wait --for=condition=ready --timeout=120s pod -l app=nse-firewall-vpp -n $NAMESPACE
echo "  等待 Client..."
kubectl wait --for=condition=ready --timeout=60s pod -l app=alpine -n $NAMESPACE
echo "  等待 Server..."
kubectl wait --for=condition=ready --timeout=60s pod -l app=nse-kernel -n $NAMESPACE

# 3. 显示状态
echo ""
echo "📋 步骤3: Pod 状态"
kubectl get pods -n $NAMESPACE -o wide

# 4. 检查日志
echo ""
echo "📜 步骤4: Firewall NSE 日志 (最后20行)"
FIREWALL_POD=$(kubectl get pod -n $NAMESPACE -l app=nse-firewall-vpp -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n $NAMESPACE $FIREWALL_POD --tail=20

# 5. 检查网络接口
echo ""
echo "🌐 步骤5: 客户端网络接口"
CLIENT_POD=$(kubectl get pod -n $NAMESPACE -l app=alpine -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n $NAMESPACE $CLIENT_POD -- ip addr show | grep -A 3 nsm || echo "  警告: 未找到 NSM 接口"

# 6. 测试连通性
echo ""
echo "🔍 步骤6: 测试连通性"
echo -n "  ICMP (ping): "
if kubectl exec -n $NAMESPACE $CLIENT_POD -- ping -c 2 -W 3 172.16.1.100 &>/dev/null; then
    echo "✅ 通过"
else
    echo "❌ 失败"
fi

echo -n "  TCP 80 (应被阻止): "
if kubectl exec -n $NAMESPACE $CLIENT_POD -- timeout 3 nc -zv 172.16.1.100 80 &>/dev/null; then
    echo "❌ 未被阻止 (规则失效!)"
else
    echo "✅ 已阻止"
fi

echo -n "  TCP 8080 (应被阻止): "
if kubectl exec -n $NAMESPACE $CLIENT_POD -- timeout 3 nc -zv 172.16.1.100 8080 &>/dev/null; then
    echo "❌ 未被阻止 (规则失效!)"
else
    echo "✅ 已阻止"
fi

# 7. 检查VPP
echo ""
echo "🔧 步骤7: VPP 状态"
kubectl exec -n $NAMESPACE $FIREWALL_POD -- vppctl show version | head -1 || echo "  警告: 无法获取 VPP 版本"

echo ""
echo "========================================="
echo "✅ 快速测试完成！"
echo ""
echo "💡 提示:"
echo "  - 查看详细日志: kubectl logs -n $NAMESPACE $FIREWALL_POD"
echo "  - 进入客户端: kubectl exec -it -n $NAMESPACE $CLIENT_POD -- sh"
echo "  - 清理环境: kubectl delete ns $NAMESPACE"
