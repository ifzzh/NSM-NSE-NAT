#!/usr/bin/env bash
set -euo pipefail

# Simple helper to repeat common kubectl workflows for this repo
# Usage examples:
#   ./natctl.sh apply
#   ./natctl.sh watch
#   ./natctl.sh get
#   ./natctl.sh logs
#   ./natctl.sh describe
#   ./natctl.sh delete
#
# Options:
#   -n|--namespace <ns>    Override namespace (default: ns-nse-composition)
#   -k|--kustomize <dir>   Kustomize dir for apply (default: .)
#   -w|--watch-interval N  watch interval seconds (default: 2)
#   -h|--help              Show help

NAMESPACE="ns-nse-composition"
KUSTOMIZE_DIR="."
WATCH_INTERVAL=2
APP_LABEL="nse-nat-vpp"

# Parse flags before subcommand
ACTION="help"
ARGS=("$@")
while [[ ${#ARGS[@]} -gt 0 ]]; do
  case "${ARGS[0]}" in
    -n|--namespace)
      NAMESPACE="${ARGS[1]:-}"; ARGS=(${ARGS[@]:2}) ;;
    -k|--kustomize)
      KUSTOMIZE_DIR="${ARGS[1]:-}"; ARGS=(${ARGS[@]:2}) ;;
    -w|--watch-interval)
      WATCH_INTERVAL="${ARGS[1]:-}"; ARGS=(${ARGS[@]:2}) ;;
    -a|--app-label)
      APP_LABEL="${ARGS[1]:-}"; ARGS=(${ARGS[@]:2}) ;;
    -h|--help)
      ACTION="help"; ARGS=(${ARGS[@]:1}); break ;;
    apply|get|watch|logs|describe|delete|full|help)
      ACTION="${ARGS[0]}"; ARGS=(${ARGS[@]:1}); break ;;
    *)
      echo "Unknown option or action: ${ARGS[0]}" >&2
      ACTION="help"; break ;;
  esac
done

script_dir() {
  cd "$(dirname "$0")" && pwd
}

ensure_logs_dir() {
  mkdir -p "$(script_dir)/logs"
}

# Print and execute a command safely (shell-escaped preview)
run() {
  printf '+ '
  printf '%q ' "$@"
  echo
  "$@"
}

get_vpp_pod() {
  kubectl get pod -n "$NAMESPACE" -l app="$APP_LABEL" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true
}

get_alpine_pod() {
  # The Alpine pod name is static in manifests; fall back to label if changed later
  if kubectl get pod -n "$NAMESPACE" alpine >/dev/null 2>&1; then
    echo "alpine"
  else
    kubectl get pod -n "$NAMESPACE" -l app=alpine \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true
  fi
}

print_context() {
  echo "Kube context: $(kubectl config current-context 2>/dev/null || echo unknown)"
  echo "Namespace:    $NAMESPACE"
  echo "App label:    app=$APP_LABEL"
}

cmd_apply() {
  print_context
  echo "Applying kustomize from '$KUSTOMIZE_DIR'..."
  run kubectl apply -k "$KUSTOMIZE_DIR"
}

cmd_get() {
  print_context
  run kubectl get pod -n "$NAMESPACE" -o wide
}

cmd_watch() {
  print_context
  if command -v watch >/dev/null 2>&1; then
    echo "+ watch -n $WATCH_INTERVAL kubectl get pod -n $NAMESPACE -o wide"
    watch -n "$WATCH_INTERVAL" "kubectl get pod -n $NAMESPACE -o wide"
  else
    echo "'watch' not found; falling back to kubectl --watch (Ctrl+C to stop)" >&2
    echo "+ kubectl get pod -n $NAMESPACE -o wide --watch"
    kubectl get pod -n "$NAMESPACE" -o wide --watch
  fi
}

cmd_logs() {
  print_context
  ensure_logs_dir
  # Write logs to fixed filenames (no timestamps) per user's preference

  local alpine_pod
  alpine_pod=$(get_alpine_pod)
  if [[ -n "$alpine_pod" ]]; then
    echo "Saving logs for $alpine_pod (container: cmd-nsc-init)"
    run kubectl logs -n "$NAMESPACE" "$alpine_pod" -c cmd-nsc-init \
      > "$(script_dir)/logs/cmd-nsc-init.log" || true
  else
    echo "WARN: Alpine pod not found; skipping cmd-nsc-init logs" >&2
  fi

  local vpp_pod
  vpp_pod=$(get_vpp_pod)
  if [[ -n "$vpp_pod" ]]; then
    echo "Saving logs for $vpp_pod"
    run kubectl logs -n "$NAMESPACE" "$vpp_pod" \
      > "$(script_dir)/logs/nse-nat-vpp.log" || true
  else
    echo "WARN: nse-nat-vpp pod not found; skipping vpp logs" >&2
  fi

  echo "Logs written to $(script_dir)/logs/"
}

# Wait until at least one pod with app label exists (up to 60s), then wait Ready (up to 180s)
wait_for_app_pods_ready() {
  local exist_timeout=60
  local start_ts=$(date +%s)
  echo "Waiting for pods with label app=$APP_LABEL to appear (timeout: ${exist_timeout}s)..."
  while true; do
    local count
    count=$(kubectl get pod -n "$NAMESPACE" -l app="$APP_LABEL" --no-headers 2>/dev/null | wc -l || true)
    if [[ "$count" -gt 0 ]]; then
      echo "Found $count pod(s) with app=$APP_LABEL. Waiting for Ready (timeout: 180s)..."
      break
    fi
    if (( $(date +%s) - start_ts >= exist_timeout )); then
      echo "WARN: No pods with app=$APP_LABEL appeared within ${exist_timeout}s" >&2
      break
    fi
    sleep 2
  done

  # If pods exist, wait for Ready
  if kubectl get pod -n "$NAMESPACE" -l app="$APP_LABEL" --no-headers >/dev/null 2>&1; then
    kubectl wait --for=condition=Ready pod -l app="$APP_LABEL" -n "$NAMESPACE" --timeout=180s || {
      echo "WARN: kubectl wait did not complete successfully for app=$APP_LABEL" >&2
    }
  fi
}

cmd_full() {
  ensure_logs_dir
  local cmdline_log="$(script_dir)/logs/cmdline.log"
  # Capture all stdout/stderr from this block into cmdline.log while still showing on screen
  {
    echo "=== APPLY ==="
    print_context
    echo "Applying kustomize from '$KUSTOMIZE_DIR'..."
    kubectl apply -k "$KUSTOMIZE_DIR"
    echo

    echo "=== WAIT READY (app=$APP_LABEL) ==="
    wait_for_app_pods_ready
    echo

    echo "Current pods:"
    kubectl get pod -n "$NAMESPACE" -o wide || true
    echo

    echo "=== SLEEP 10s ==="
    sleep 10
    echo

    echo "Current pods:"
    kubectl get pod -n "$NAMESPACE" -o wide || true
    echo

    echo "=== COLLECT LOGS ==="
    cmd_logs
    echo

    echo "=== DESCRIBE (app=$APP_LABEL) ==="
    cmd_describe || true
    echo

    echo "=== DONE ==="
  } > >(tee "$cmdline_log") 2>&1

  echo "Full flow complete. Cmdline log saved to $cmdline_log"
}

cmd_describe() {
  print_context
  local vpp_pod
  vpp_pod=$(get_vpp_pod)
  if [[ -z "$vpp_pod" ]]; then
    echo "nse-nat-vpp pod not found in namespace '$NAMESPACE'" >&2
    exit 1
  fi
  run kubectl describe pod -n "$NAMESPACE" "$vpp_pod"
}

cmd_delete() {
  print_context
  read -r -p "Delete namespace '$NAMESPACE'? [y/N] " ans
  if [[ "$ans" =~ ^[Yy]$ ]]; then
    run kubectl delete ns "$NAMESPACE"
  else
    echo "Aborted."
  fi
}

cmd_help() {
  cat <<'EOF'
Usage: natctl.sh [options] <action>

Actions:
  apply        kubectl apply -k <dir> (default: .)
  get          kubectl get pods in namespace
  watch        watch kubectl get pods (requires 'watch'; falls back otherwise)
  logs         collect logs to ./logs/cmd-nsc-init.log and ./logs/nse-nat-vpp.log
  describe     describe the nse-nat-vpp pod
  full         run: apply -> wait Ready (app=$APP_LABEL) -> sleep 10s -> logs -> describe
  delete       delete the namespace
  help         show this message

Options:
  -n, --namespace <ns>      namespace (default: ns-nse-composition)
  -k, --kustomize <dir>     kustomize dir for apply (default: .)
  -w, --watch-interval <n>  watch refresh interval seconds (default: 2)
  -a, --app-label <value>   value for 'app' label to target (default: nse-nat-vpp)
  -h, --help                show help

Examples:
  ./natctl.sh apply
  ./natctl.sh watch
  ./natctl.sh get
  ./natctl.sh logs
  ./natctl.sh describe
  ./natctl.sh full
  ./natctl.sh delete
EOF
}

case "$ACTION" in
  apply)    cmd_apply ;;
  get)      cmd_get ;;
  watch)    cmd_watch ;;
  logs)     cmd_logs ;;
  describe) cmd_describe ;;
  full)     cmd_full ;;
  delete)   cmd_delete ;;
  help|*)   cmd_help ;;
esac
