# Architecture

## Project Overview

Device Config Manager (DCM) is a unified daemon that manages AMD GPU and AINIC (AI Network Interface Card) configurations. It runs as a privileged Kubernetes DaemonSet or Debian systemd service.

**Module path**: `github.com/ROCm/device-config-manager`
**Go version**: 1.25.8
**Supported platforms**: Ubuntu 22.04/24.04, RHEL 9, Kubernetes v1.24+, ROCm 6.3–7.2.1

## Entry Point and Startup Sequence

`cmd/deviceconfigmanager/main.go` is the single entry point. Build-time variables injected via ldflags:

```go
var (
    Version   string  // -X main.Version=$VERSION
    BuildDate string  // -X main.BuildDate=$BUILD_DATE
    GitCommit string  // -X main.GitCommit=$GIT_COMMIT
)
```

**Command-line flags:**
- `-manage-gpu` (bool, default: true) — enable GPU configuration management
- `-manage-ainic` (bool, default: true) — enable AINIC configuration management

**Startup sequence:**

1. **Environment detection**: `utils.IsKubernetes()` checks `KUBERNETES_SERVICE_HOST` env var
2. **Device capability detection** (`detectDeviceCapabilities()`):
   - K8s mode: checks GPU config at `/etc/config-manager/config.json` (file must exist AND have size > 0); checks AINIC config at `/etc/config-manager-ainic/ainic_config.json`
   - Debian mode: only AINIC config checked; GPU partitioning not supported
3. **K8s initialization** (if K8s mode):
   - Creates `K8sClient` with `context.Background()`
   - Gets node name via `k8sclient.GetNodeName()` (reads `DS_NODE_NAME` env var)
   - Deletes stale labels: `dcm.amd.com/gpu-config-profile-state` and `dcm.amd.com/nic-config-profile-state`
4. **GPU Manager launch** (if K8s + GPU config present):
   - `go configmanager.Worker()` — retry worker goroutine
   - `configmanager.TriggerRetryLoop(profile, "initial partitioning")` — initial partition trigger
   - `go configmanager.StartFileWatcher(profile)` — watches config file
   - `go configmanager.NodeLabelWatcher()` — watches node labels
5. **AINIC Manager launch** (if AINIC config present, K8s or Debian):
   - `go ainicmanager.ConfigureAINICs()` — initial configuration
   - `go ainicmanager.StartFileWatcher(isKubernetes)` — watches config file
   - `go ainicmanager.NodeLabelWatcher()` — watches node labels (K8s only)
6. **Error exits (code 1)**:
   - GPU requested in Debian mode
   - GPU enabled but no config file found
   - No device managers initialized
7. **Keep-alive**: `<-make(chan struct{})` blocks forever

**No signal handling** — relies on Kubernetes/systemd to manage process lifecycle.

## Package Layout

| Package | Role |
|---------|------|
| `cmd/deviceconfigmanager` | Entry point, environment detection, goroutine orchestration |
| `pkg/config_manager` | GPU partitioning via CGo AMD SMI bindings |
| `pkg/ainic_manager` | AINIC configuration via `nicctl` CLI |
| `pkg/amdgpu/k8sclient` | Kubernetes API wrapper (node labels, events, informers, dynamic client) |
| `pkg/globals` | Constants: label keys, event reasons, file paths, partition defaults, AMD SMI status codes |
| `pkg/interface` | Status types: `PartitionStatus`, `AINICStatus`, `GPUPartitionStatus`, `AINICCardStatus` |
| `pkg/utils` | File watching (fsnotify), systemd/D-Bus control, K8s detection, node label watching |
| `gen/partition` | Generated Go code from `proto/partition.proto` |
| `gen/ainic` | Generated Go code from `proto/ainic.proto` |
| `proto/` | Protobuf definitions for GPU and AINIC configs |
| `helm-charts/` | Kubernetes deployment (DaemonSet, RBAC, ConfigMaps) |
| `test/k8s-e2e/` | E2E test suite (gocheck framework) |

## Component Relationship Diagram

```
main.go (Orchestrator)
  ├── Detects environment (K8s/Debian)
  ├── Detects device capabilities (GPU/AINIC config files)
  └── Launches managers as goroutines
        │
  ┌─────┴──────────────────────────────────────┐
  │                                            │
  GPU Manager (K8s only)              AINIC Manager (K8s + Debian)
  pkg/config_manager                  pkg/ainic_manager
  │                                   │
  ├─ Worker goroutine (retryCh)       ├─ ConfigureAINICs()
  ├─ RetryPartition (30m timeout)     ├─ nicctl shell commands
  ├─ AMD SMI C API (CGo)              ├─ SR-IOV via sysfs
  ├─ systemd service control          ├─ DCQCN config
  ├─ File watcher (config.json)       ├─ File watcher (ainic_config.json)
  └─ Node label watcher               └─ Node label watcher
        │                                   │
        └───────────┬───────────────────────┘
                    │
              K8s Client (pkg/amdgpu/k8sclient)
              ├─ GetNodeLabel / AddNodeLabel / DeleteNodeLabel
              ├─ CreateEvent (status reporting)
              ├─ GetNodeInformer (label watching)
              └─ DeleteNodeModulesConfig (KMM recovery)
                    │
              Utils (pkg/utils)
              ├─ StartFileWatcher (fsnotify)
              ├─ NodeLabelWatcher (K8s informer)
              ├─ systemd D-Bus control
              └─ IsKubernetes()
```

## Configuration Flow

- **K8s mode**: ConfigMaps mounted as JSON files. Profile selection via node labels (`dcm.amd.com/gpu-config-profile` for GPU, `dcm.amd.com/nic-config-profile` for AINIC).
- **Debian mode**: JSON config files at same paths. Only AINIC supported. Profile selection from `selectednodeProfile` field in config JSON.
- **Trigger sources**: File change (fsnotify) or node label change (K8s informer) → triggers reconfiguration.
- **Results reported as**: Kubernetes events (JSON-serialized status) + node labels (success/failure state).

## Runtime Model

DCM is a **pull-based daemon**. It has no API server or incoming connections. It reads configuration from mounted files and Kubernetes node labels, applies hardware changes, and reports status via K8s events and labels.

**Concurrency model**: Each manager runs as independent goroutines. GPU Manager uses a buffered channel (`retryCh`, capacity 1) with a mutex-protected worker to serialize partition operations. AINIC Manager runs synchronously per trigger.

## Key Environment Variables

| Variable | Source | Purpose |
|----------|--------|---------|
| `KUBERNETES_SERVICE_HOST` | K8s auto-injected | Detects K8s vs Debian mode |
| `DS_NODE_NAME` | DaemonSet spec.nodeName | Node identity for labels/events |
| `POD_NAME` | metadata.name | Event source identification |
| `POD_NAMESPACE` | metadata.namespace | Event namespace |
| `KMM_DRIVER_ENABLED` | Helm/manual | Enables KMM driver recovery path |

## Key Dependencies

| Library | Purpose |
|---------|---------|
| `k8s.io/client-go` v0.32.3 | Kubernetes API access |
| `google.golang.org/protobuf` v1.36.8 | Config serialization |
| `github.com/fsnotify/fsnotify` v1.8.0 | File change detection |
| `github.com/godbus/dbus/v5` v5.1.0 | systemd service management |
| `github.com/sirupsen/logrus` v1.9.3 | Structured logging |
| `gopkg.in/check.v1` | gocheck test framework |
| AMD SMI C library (`libamd_smi`, `libdrm_amdgpu`, `libdrm`) | GPU hardware management |
