# Test NSE composition

This example demonstrates a more complex Network Service, where we chain three passthrough and one ACL Filtering NS endpoints.
It demonstrates how NSM allows for service composition (chaining).
It involves a combination of kernel and memif mechanisms, as well as VPP enabled endpoints.

## Requires

Make sure that you have completed steps from [basic](../../basic) or [memory](../../memory) setup.

## Run

Deploy NSC and NSE:
```bash
kubectl apply -k ./samenode-firewall-refactored/
```


kubectl get pod -n ns-nse-composition -o wide

kubectl logs -n ns-nse-composition <pod-name>

Wait for applications ready:
```bash
kubectl wait --for=condition=ready --timeout=5m pod -l app=alpine -n ns-nse-composition
```
```bash
kubectl wait --for=condition=ready --timeout=1m pod -l app=nse-kernel -n ns-nse-composition
```

Ping from NSC to NSE:
```bash
kubectl exec pods/alpine -n ns-nse-composition -- ping -c 4 172.16.1.100
```

Check TCP Port 8080 on NSE is accessible to NSC
```bash
kubectl exec pods/alpine -n ns-nse-composition -- wget -O /dev/null --timeout 5 "172.16.1.100:8080"
```

Check TCP Port 80 on NSE is inaccessible to NSC
```bash
kubectl exec pods/alpine -n ns-nse-composition -- wget -O /dev/null --timeout 5 "172.16.1.100:80"
if [ 0 -eq $? ]; then
  echo "error: port :80 is available" >&2
  false
else
  echo "success: port :80 is unavailable"
fi
```

Check TCP Port 80 on NSE is inaccessible to NSC
```bash
kubectl exec pods/alpine -n ns-nse-composition -- wget -O /dev/null --timeout 5 "172.16.1.100:8080"
if [ 0 -eq $? ]; then
  echo "error: port :8080 is available" >&2
  false
else
  echo "success: port :8080 is unavailable"
fi
```

Ping from NSE to NSC:
```bash
kubectl exec deployments/nse-kernel -n ns-nse-composition -- ping -c 4 172.16.1.101
```

安装iperf3:
```bash
kubectl exec -it pods/alpine -n ns-nse-composition -- apk add iperf3
```

安装iperf3服务端：
```bash
kubectl exec -it deployments/nse-kernel -n ns-nse-composition -- apk add iperf3
```

启动iperf3服务端：
```bash
kubectl exec -it deployments/nse-kernel -n ns-nse-composition -- iperf3 -s
```

启动iperf3客户端：
```bash
kubectl exec -it pods/alpine -n ns-nse-composition -- iperf3 -c 172.16.1.101 -t 30
kubectl exec -it pods/alpine -n ns-nse-composition -- iperf3 -c 172.16.1.100 -t 30 -u -b 20G
```

## Cleanup

Delete ns:
```bash
kubectl delete ns ns-nse-composition
```
