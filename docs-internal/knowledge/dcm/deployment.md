# Deployment

## Kubernetes Deployment (Helm)

### Chart Metadata

- Chart name: `device-config-manager-charts`
- Version: `v1.4.1`
- App version: `v1.4.1`

### Helm Values

```yaml
platform: k8s
image:
  repository: registry.test.pensando.io:5000/device-config-manager
  tag: latest
  pullPolicy: Always
  initContainerImage: busybox:1.36

# GPU ConfigMap — chart always creates from templates/configmap.yaml
# Default name: "default-dcm-config" (override with configMap value)
configMap: ""

# AINIC ConfigMap (optional)
ainicConfigMap: ""             # Empty = no AINIC ConfigMap mount
createAinicConfigMap: false    # Must be true to create the ConfigMap

# Host mounts
ainicHostMounts: false         # Set true to mount AINIC host paths
simEnabled: true               # Set true in E2E/CI without real GPUs
```

### DaemonSet

**Name**: `<release>-amdgpu-device-config-manager`
**ServiceAccount**: `<release>-config-manager`

**Init Container (driver-init)**:
- Image: `busybox:1.36`
- If `simEnabled: true`: exits immediately (`exit 0`)
- If `simEnabled: false`: loops checking `/host-sys/class/kfd` and `/host-sys/module/amdgpu/drivers/` every 2 seconds
- Security: `privileged: true`

**Main Container**:
- Image: `<repository>:<tag>`
- Security: `privileged: true`
- Working directory: `/root`

**Environment Variables** (from fieldRef):
- `DS_NODE_NAME` ← `spec.nodeName`
- `POD_NAME` ← `metadata.name`
- `POD_NAMESPACE` ← `metadata.namespace`

**Required Toleration**:
```yaml
- effect: NoExecute
  key: amd-dcm
  operator: Equal
  value: up
```

### Volume Mounts

**Always mounted (hostPath)**:

| Mount Path | Host Path | Type |
|-----------|----------|------|
| `/dev` | `/dev` | Directory |
| `/sys` | `/sys` | Directory |
| `/lib/modules` | `/lib/modules` | Directory |
| `/var/lib/` | `/var/lib/` | Directory |
| `/etc/systemd` | `/etc/systemd` | Directory |
| `/run/systemd` | `/run/systemd` | Directory |
| `/usr/lib/systemd` | `/usr/lib/systemd` | Directory |
| `/var/run/dbus` | `/var/run/dbus` | Directory |

**Always mounted (ConfigMap)**:
- `/etc/config-manager/` ← GPU ConfigMap (named via `dcm.gpuConfigMapName` helper, defaults to `default-dcm-config`)

**Conditional (when `ainicConfigMap` is set)**:
- `/etc/config-manager-ainic/` ← AINIC ConfigMap

**Conditional (when `ainicHostMounts: true`)**:

| Mount Path | Host Path | Type |
|-----------|----------|------|
| `/usr/sbin/nicctl` | `/usr/sbin/nicctl` | File |
| `/opt/amd` | `/opt/amd` | Directory |
| `/etc/amd/ainic` | `/etc/amd/ainic` | Directory |

### RBAC

**ClusterRole permissions**:

| API Group | Resources | Verbs |
|-----------|-----------|-------|
| "" (core) | events | create, get, list, update |
| "" (core) | nodes | get, list, watch, update |
| "apps" | daemonsets | get, list, watch, delete, create, update |
| "" (core) | pods | get, list, watch, delete, create, update |

### Default GPU ConfigMap

Created from `templates/configmap.yaml` with predefined profiles:
- `default`: SPX-NPS1
- `cpx_nps1_all`: CPX-NPS1
- `cpx_nps4_all`: CPX-NPS4
- `dpx_nps2_all`: DPX-NPS2
- `qpx_nps1_all`: QPX-NPS1
- `heterogeneous_example`: CPX-NPS1 (2 GPUs) + SPX-NPS1 (remainder)

Default systemd services: `["amd-metrics-exporter", "gpuagent"]`

### AINIC ConfigMap Template

Created only when `ainicConfigMap` is set AND `createAinicConfigMap: true`.
Contains DCQCN profiles, port profiles, node profiles, and NIC profiles.

### Template Helpers (`_helpers.tpl`)

- `dcm.gpuConfigMapName`: Returns GPU ConfigMap name, defaults to `"default-dcm-config"` if `.Values.configMap` is empty

### Standalone ConfigMap YAML

`helm-charts/default-dcm-config.yaml` — same content as ConfigMap template, namespace `kube-amd-gpu`. Can be applied manually for GitOps workflows.

## Debian Deployment

### systemd Service

**File**: `debian/usr/lib/systemd/system/amd-config-manager.service`

```ini
[Service]
User=root
Group=root
Restart=on-failure
RestartSec=10
Type=simple
Environment="LD_LIBRARY_PATH=/usr/local/configs/lib:$LD_LIBRARY_PATH"
ExecStartPre=/bin/sleep 5
ExecStart=/usr/local/bin/device-config-manager-UBUNTU_VERSION_PLACEHOLDER
ExecStop=/bin/kill -15 $MAINPID

[Install]
WantedBy=multi-user.target
```

- Runs as root
- Restart on failure with 10-second delay
- 5-second pre-start delay
- SIGTERM for graceful stop
- `UBUNTU_VERSION_PLACEHOLDER` replaced at build time with `jammy` or `noble`

### Debian Package Contents

| Path | Content |
|------|---------|
| `/usr/local/bin/device-config-manager-{jammy\|noble}` | DCM binary |
| `/usr/local/configs/lib/` | AMD SMI shared libraries |
| `/usr/lib/systemd/system/amd-config-manager.service` | systemd service |
| `/etc/config-manager-ainic/ainic_config.json` | AINIC config (conffile) |
| `/etc/profile.d/dcm.sh` | LD_LIBRARY_PATH setup (created by postinst) |

### Package Control

```
Package: amdgpu-configmanager
Priority: optional
Section: utils
Maintainer: AMD Inc.
Architecture: amd64
Version: BUILD_VER_ENV
```

### Package Scripts

- **postinst**: Sets up `LD_LIBRARY_PATH` in `/etc/profile.d/dcm.sh`
- **prerm**: Stops and disables `amd-config-manager.service`

## Deployment Modes Summary

| Feature | Kubernetes | Debian |
|---------|-----------|--------|
| GPU partitioning | Yes (full) | Not supported |
| AINIC configuration | Yes (full) | Yes (full) |
| Config source | ConfigMaps + node labels | JSON files |
| Profile selection | Node label | Config file field |
| Status reporting | K8s events + labels | Logs only |
| Process management | DaemonSet | systemd |
| Library loading | LD_PRELOAD in entrypoint | LD_LIBRARY_PATH in env |
