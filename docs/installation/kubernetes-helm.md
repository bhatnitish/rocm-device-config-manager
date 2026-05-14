# Kubernetes (Helm) installation

Install AMD Device Config Manager with the Helm chart in this repository.

## Requirements

- ROCm 6.2.0
- Ubuntu 22.04 or later
- Kubernetes v1.29.0 or later
- Helm v3.2.0 or later
- `kubectl` configured for the cluster

## Install

1. Create `values.yaml`:

```yaml
platform: k8s

nodeSelector: {}

image:
  repository: rocm/device-config-manager
  tag: v1.4.0
  pullPolicy: Always
```

1. Install:

```bash
make helm-build
cd ./helm-charts
helm install amd-gpu-operator \
  ./device-config-manager-charts-v1.4.0.tgz -n kube-amd-gpu \
  --create-namespace -f values.yaml
```

The chart **always** creates and mounts the GPU profile `ConfigMap` from its template. **Omit** the `configMap` value key for the default name `default-dcm-config`; set `configMap` only when you want another object name (same chart-managed content).

```bash
helm install amd-gpu-operator ./device-config-manager-charts-v1.4.0.tgz \
  -n kube-amd-gpu --create-namespace \
  --set configMap=my-custom-dcm-config
```
