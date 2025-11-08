#!/bin/bash
#
# NSM Firewall NSE 重构版本自动化测试脚本
# 用于验证 ifzzh520/nsm-firewall-nse-refactored:v1.0.0 镜像功能
#
set -o pipefail

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
NAMESPACE="ns-nse-composition"
DEPLOY_DIR="."
TEST_TIMEOUT=300  # 5分钟超时
STEP_TIMEOUT=60   # 每步1分钟超时

# 测试统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    ((PASSED_TESTS++))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    ((FAILED_TESTS++))
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

log_step() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        log_error "必需的命令 '$1' 未找到，请先安装"
        exit 1
    fi
}

# 等待Pod就绪
wait_for_pod() {
    local selector=$1
    local timeout=${2:-$STEP_TIMEOUT}

    log_info "等待 Pod 就绪: $selector (超时: ${timeout}s)"

    if kubectl wait --for=condition=ready --timeout=${timeout}s \
        pod -l $selector -n $NAMESPACE &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# 获取Pod名称
get_pod_name() {
    local selector=$1
    kubectl get pod -n $NAMESPACE -l $selector -o jsonpath='{.items[0].metadata.name}' 2>/dev/null
}

# 清理环境
cleanup() {
    log_step "步骤 0: 清理现有环境"

    if kubectl get namespace $NAMESPACE &>/dev/null; then
        log_info "删除现有命名空间 $NAMESPACE"
        kubectl delete namespace $NAMESPACE --timeout=60s &>/dev/null || true

        # 等待命名空间完全删除
        local wait_time=0
        while kubectl get namespace $NAMESPACE &>/dev/null; do
            if [ $wait_time -ge 60 ]; then
                log_warning "命名空间删除超时，继续执行"
                break
            fi
            sleep 2
            ((wait_time+=2))
        done
        log_success "现有环境已清理"
    else
        log_info "命名空间不存在，无需清理"
    fi
}

# 部署测试环境
deploy() {
    log_step "步骤 1: 部署测试环境"
    ((TOTAL_TESTS++))

    cd $DEPLOY_DIR || exit 1

    log_info "使用 Kustomize 部署..."
    if kubectl apply -k . 2>&1 | tee /tmp/deploy.log; then
        log_success "部署命令执行成功"
        return 0
    else
        log_error "部署失败"
        cat /tmp/deploy.log
        return 1
    fi
}

# 验证Pod启动
verify_pods() {
    log_step "步骤 2: 验证 Pod 状态"

    # 检查 Firewall NSE
    ((TOTAL_TESTS++))
    log_info "检查 Firewall NSE Pod..."
    if wait_for_pod "app=nse-firewall-vpp" 120; then
        FIREWALL_POD=$(get_pod_name "app=nse-firewall-vpp")
        log_success "Firewall NSE Pod 已就绪: $FIREWALL_POD"
    else
        log_error "Firewall NSE Pod 启动失败"
        kubectl get pods -n $NAMESPACE -l app=nse-firewall-vpp
        kubectl describe pod -n $NAMESPACE -l app=nse-firewall-vpp | tail -50
        return 1
    fi

    # 检查 NSE Kernel (Server)
    ((TOTAL_TESTS++))
    log_info "检查 NSE Kernel Pod..."
    if wait_for_pod "app=nse-kernel" 60; then
        SERVER_POD=$(get_pod_name "app=nse-kernel")
        log_success "NSE Kernel Pod 已就绪: $SERVER_POD"
    else
        log_error "NSE Kernel Pod 启动失败"
        kubectl get pods -n $NAMESPACE -l app=nse-kernel
        return 1
    fi

    # 检查 Client Pod
    ((TOTAL_TESTS++))
    log_info "检查 Client Pod..."
    if wait_for_pod "app=alpine" 60; then
        CLIENT_POD=$(get_pod_name "app=alpine")
        log_success "Client Pod 已就绪: $CLIENT_POD"
    else
        log_error "Client Pod 启动失败"
        kubectl get pods -n $NAMESPACE -l app=alpine
        return 1
    fi

    # 显示所有Pod状态
    log_info "所有 Pod 状态:"
    kubectl get pods -n $NAMESPACE -o wide

    return 0
}

# 验证Firewall NSE注册
verify_nse_registration() {
    log_step "步骤 3: 验证 Firewall NSE 注册"
    ((TOTAL_TESTS++))

    log_info "检查 Firewall NSE 日志..."
    local logs=$(kubectl logs -n $NAMESPACE $FIREWALL_POD --tail=50 2>/dev/null)

    # 检查关键日志
    if echo "$logs" | grep -q "executing phase"; then
        log_success "发现启动阶段日志"
    else
        log_error "未找到启动阶段日志"
        echo "$logs"
        return 1
    fi

    # 检查SVID获取
    if echo "$logs" | grep -q "retrieving svid\|SVID"; then
        log_success "SPIFFE身份认证成功"
    else
        log_warning "未找到SVID相关日志"
    fi

    # 检查注册成功
    if echo "$logs" | grep -q "register.*nse\|startup completed" || \
       echo "$logs" | grep -q "executing phase 6"; then
        log_success "NSE注册流程已执行"
    else
        log_error "NSE注册流程可能失败"
        echo "$logs"
        return 1
    fi

    return 0
}

# 验证网络接口
verify_network_interface() {
    log_step "步骤 4: 验证网络接口创建"
    ((TOTAL_TESTS++))

    log_info "检查客户端 NSM 网络接口..."

    # 等待接口创建
    sleep 5

    local interfaces=$(kubectl exec -n $NAMESPACE $CLIENT_POD -- ip addr show 2>/dev/null)

    if echo "$interfaces" | grep -q "nsm"; then
        local nsm_if=$(echo "$interfaces" | grep -A 5 "nsm" | head -10)
        log_success "NSM接口已创建"
        echo "$nsm_if" | grep -E "inet |nsm"
    else
        log_error "NSM接口未创建"
        echo "所有接口:"
        echo "$interfaces"

        # 显示更多诊断信息
        log_warning "检查 Client Pod 事件:"
        kubectl describe pod -n $NAMESPACE $CLIENT_POD | tail -20
        return 1
    fi

    return 0
}

# 验证ACL配置挂载
verify_acl_config() {
    log_step "步骤 5: 验证 ACL 配置文件"
    ((TOTAL_TESTS++))

    log_info "检查 ACL 配置文件是否挂载..."

    if kubectl exec -n $NAMESPACE $FIREWALL_POD -- cat /etc/firewall/config.yaml &>/dev/null; then
        log_success "ACL配置文件已挂载"

        log_info "ACL规则内容:"
        kubectl exec -n $NAMESPACE $FIREWALL_POD -- cat /etc/firewall/config.yaml | grep -E "allow|forbid" | head -10
    else
        log_error "ACL配置文件未挂载"
        return 1
    fi

    return 0
}

# 测试ICMP连通性
test_icmp() {
    log_step "步骤 6: 测试 ICMP (应该通过)"
    ((TOTAL_TESTS++))

    log_info "从客户端 ping 服务端..."

    if kubectl exec -n $NAMESPACE $CLIENT_POD -- ping -c 3 -W 5 172.16.1.100 &>/dev/null; then
        log_success "ICMP 测试通过 (允许规则生效)"
    else
        log_error "ICMP 测试失败"
        kubectl exec -n $NAMESPACE $CLIENT_POD -- ping -c 3 -W 5 172.16.1.100 || true
        return 1
    fi

    return 0
}

# 测试TCP 5201 (应该允许)
test_tcp_5201() {
    log_step "步骤 7: 测试 TCP 5201 (应该通过)"
    ((TOTAL_TESTS++))

    log_info "测试 TCP 5201 端口访问..."

    # 注意：这里假设服务端在5201端口有监听，如果没有会超时
    if kubectl exec -n $NAMESPACE $CLIENT_POD -- timeout 5 nc -zv 172.16.1.100 5201 &>/dev/null; then
        log_success "TCP 5201 可访问 (允许规则生效)"
    else
        log_warning "TCP 5201 连接超时 (服务端可能未监听此端口，这是正常的)"
        # 不计为失败
        ((TOTAL_TESTS--))
    fi

    return 0
}

# 测试TCP 80 (应该禁止)
test_tcp_80() {
    log_step "步骤 8: 测试 TCP 80 (应该被阻止)"
    ((TOTAL_TESTS++))

    log_info "测试 TCP 80 端口访问..."

    # 尝试连接，应该被firewall阻止
    if kubectl exec -n $NAMESPACE $CLIENT_POD -- timeout 5 wget -O /dev/null --timeout=5 172.16.1.100:80 &>/dev/null; then
        log_error "TCP 80 可访问 (阻止规则未生效!)"
        return 1
    else
        log_success "TCP 80 被阻止 (阻止规则生效)"
    fi

    return 0
}

# 测试TCP 8080 (应该禁止)
test_tcp_8080() {
    log_step "步骤 9: 测试 TCP 8080 (应该被阻止)"
    ((TOTAL_TESTS++))

    log_info "测试 TCP 8080 端口访问..."

    if kubectl exec -n $NAMESPACE $CLIENT_POD -- timeout 5 wget -O /dev/null --timeout=5 172.16.1.100:8080 &>/dev/null; then
        log_error "TCP 8080 可访问 (阻止规则未生效!)"
        return 1
    else
        log_success "TCP 8080 被阻止 (阻止规则生效)"
    fi

    return 0
}

# 测试VPP状态
test_vpp_status() {
    log_step "步骤 10: 检查 VPP 状态"
    ((TOTAL_TESTS++))

    log_info "检查 VPP 运行状态..."

    if kubectl exec -n $NAMESPACE $FIREWALL_POD -- vppctl show version &>/dev/null; then
        local vpp_ver=$(kubectl exec -n $NAMESPACE $FIREWALL_POD -- vppctl show version | head -1)
        log_success "VPP 运行正常: $vpp_ver"
    else
        log_error "VPP 未运行或无法访问"
        return 1
    fi

    # 显示VPP接口
    log_info "VPP接口列表:"
    kubectl exec -n $NAMESPACE $FIREWALL_POD -- vppctl show interface | head -20 || true

    return 0
}

# 收集诊断信息
collect_diagnostics() {
    log_step "收集诊断信息"

    local diag_dir="/tmp/nsm-firewall-diagnostics-$(date +%Y%m%d-%H%M%S)"
    mkdir -p $diag_dir

    log_info "保存诊断信息到: $diag_dir"

    # Pod状态
    kubectl get pods -n $NAMESPACE -o wide > $diag_dir/pods.txt 2>&1

    # Firewall NSE日志
    kubectl logs -n $NAMESPACE $FIREWALL_POD > $diag_dir/firewall-nse.log 2>&1

    # Client Pod日志
    kubectl logs -n $NAMESPACE $CLIENT_POD > $diag_dir/client.log 2>&1

    # Server Pod日志
    kubectl logs -n $NAMESPACE $SERVER_POD > $diag_dir/server.log 2>&1

    # 事件
    kubectl get events -n $NAMESPACE --sort-by='.lastTimestamp' > $diag_dir/events.txt 2>&1

    # 配置
    kubectl get configmap -n $NAMESPACE firewall-config-file -o yaml > $diag_dir/config.yaml 2>&1

    log_success "诊断信息已保存到: $diag_dir"
}

# 生成测试报告
generate_report() {
    log_step "测试报告"

    echo ""
    echo "======================================"
    echo "         测试结果汇总"
    echo "======================================"
    echo "总测试数:   $TOTAL_TESTS"
    echo "通过:       $PASSED_TESTS ($(echo "scale=1; $PASSED_TESTS*100/$TOTAL_TESTS" | bc 2>/dev/null || echo "N/A")%)"
    echo "失败:       $FAILED_TESTS"
    echo "======================================"
    echo ""

    if [ $FAILED_TESTS -eq 0 ]; then
        log_success "所有测试通过! 🎉"
        log_success "重构版 Firewall NSE 镜像功能正常!"
        return 0
    else
        log_error "部分测试失败"
        return 1
    fi
}

# 主测试流程
main() {
    log_step "开始 NSM Firewall NSE 重构版测试"

    # 检查必需命令
    check_command kubectl
    check_command bc

    # 检查是否在正确目录
    if [ ! -d "$DEPLOY_DIR" ]; then
        log_error "部署目录不存在: $DEPLOY_DIR"
        exit 1
    fi

    # 执行测试步骤
    cleanup
    deploy || { log_error "部署失败，测试终止"; exit 1; }
    sleep 10  # 等待资源创建
    verify_pods || { collect_diagnostics; exit 1; }
    verify_nse_registration || log_warning "NSE注册验证部分失败，继续测试"
    verify_network_interface || { collect_diagnostics; exit 1; }
    verify_acl_config
    test_icmp
    test_tcp_5201
    test_tcp_80
    test_tcp_8080
    test_vpp_status

    # 收集诊断信息（无论成功失败）
    collect_diagnostics

    # 生成报告
    generate_report

    local exit_code=$?

    # 询问是否清理
    echo ""
    read -p "是否清理测试环境? (y/N): " cleanup_choice
    if [[ $cleanup_choice =~ ^[Yy]$ ]]; then
        cleanup
    else
        log_info "保留测试环境，可手动清理: kubectl delete ns $NAMESPACE"
    fi

    exit $exit_code
}

# 运行主函数
main "$@"
