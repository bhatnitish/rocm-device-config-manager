# Testing

## Overview

DCM uses E2E tests only — there are no unit tests at the package level. Tests use the **gocheck** framework (`gopkg.in/check.v1`) and require a running Kubernetes cluster with Helm.

## Running Tests

```bash
# Full E2E suite (30-minute timeout)
make e2e

# AINIC E2E tests only
make test-ainic

# Run specific test (from test/k8s-e2e/)
cd test/k8s-e2e && make dcm_e2e DCM_E2E_CHECK="TestName"

# Lint test code
cd test/k8s-e2e && make lint   # gofmt, goimports, go vet
```

**Single test syntax**: Uses gocheck `-check.f` filter, e.g.:
```bash
go test -v ./... -timeout 30m -check.f "TestDCMConfigMapUpdatePartition"
```

## Test Framework Setup

**File**: `test/k8s-e2e/suite_test.go`

```go
. "gopkg.in/check.v1"

func Test(t *testing.T) { TestingT(t) }
var _ = Suite(&E2ESuite{})
```

### E2ESuite Structure

```go
E2ESuite {
    helmChart   string           // Path to Helm chart
    kubeconfig  string           // KUBECONFIG path
    ns          string           // Test namespace
    registry    string           // Image registry
    imageTag    string           // Image tag
    platform    string           // "k8s" or "openshift"
    simEnable   bool             // Simulation mode
    k8sclient   *K8sClient       // Kubernetes client
    helmClient  *HelmClient      // Helm client
    restConfig  *restclient.Config
}
```

### SetUpSuite

1. Initializes configuration from command-line flags
2. Validates platform ("k8s" or "openshift")
3. Creates Kubernetes client from kubeconfig
4. Creates Helm client with namespace and REST config
5. Installs DCM Helm chart with test values

### TearDownSuite

1. Uninstalls Helm release
2. Deletes test namespace

## Configuration Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `KUBECONFIG` | `~/.kube/config` | K8s cluster config |
| `NAMESPACE` | `kube-amd-dcm-test` | Test namespace |
| `REGISTRY` | `ghcr.io/rocm/device-config-manager` | Image registry |
| `IMAGE_TAG` | `dev` | Image tag |
| `SIMENABLED` | `true` | Skip tests needing real GPU |
| `E2E_SIM_FLAG` | Derived from SIMENABLED | Simulation flag |
| `PLATFORM` | `k8s` | Platform type |
| `HELMCHART` | `../../helm-charts` | Helm chart path |

## Simulation Mode

When `SIMENABLED=true` (default):
- `simEnabled: true` in Helm values → init container exits immediately (no driver check)
- Tests that require real GPU hardware are skipped
- Tests focus on K8s resource management (ConfigMaps, labels, events, pod lifecycle)

When `SIMENABLED=false`:
- Init container waits for amdgpu driver to load
- Tests can validate actual GPU partitioning behavior

## Test Helpers

### Helm Client (`test/k8s-e2e/clients/helm.go`)

```go
type HelmClient struct {
    client helmclient.Client
    ns     string
}
```

**Key methods**:
- `NewHelmClient(ns, restConfig)` — Creates Helm client for namespace
- `InstallChart(chartPath, releaseName, values)` — Installs or upgrades chart
- `UninstallChart(releaseName)` — Removes release
- `GetRelease(releaseName)` — Gets release info

### K8s Client (`test/k8s-e2e/clients/k8s.go`)

```go
type K8sClient struct {
    Clientset *kubernetes.Clientset
    Config    *restclient.Config
}
```

**Key methods**:
- `NewK8sClient(kubeconfig)` — Creates client from kubeconfig
- `CreateNamespace(ns)` — Creates test namespace
- `DeleteNamespace(ns)` — Deletes test namespace
- `GetPods(ns)` — Lists pods in namespace
- `GetPodStatus(ns, podName)` — Gets pod phase
- `WaitForPodReady(ns, labelSelector, timeout)` — Waits for pod to be Running
- `GetEvents(ns)` — Lists events
- `GetNodeLabels(nodeName)` — Gets node labels
- `SetNodeLabel(nodeName, key, value)` — Sets node label
- `DeleteNodeLabel(nodeName, key)` — Removes node label
- `CreateConfigMap(ns, name, data)` — Creates ConfigMap
- `UpdateConfigMap(ns, name, data)` — Updates ConfigMap
- `DeleteConfigMap(ns, name)` — Deletes ConfigMap
- `GetDaemonSet(ns, name)` — Gets DaemonSet
- `WaitForDaemonSetReady(ns, name, timeout)` — Waits for desired == ready

## DCM Tests (`test/k8s-e2e/dcm_test.go`)

Tests validate GPU partitioning workflow through K8s resources:

**Test patterns**:
1. Install chart with specific ConfigMap
2. Wait for DCM pod to be running
3. Set node label with profile name
4. Wait for events indicating partition result
5. Verify node labels reflect success/failure
6. Verify K8s events contain expected status

**Example test areas**:
- ConfigMap creation and update → triggers reconfiguration
- Node label changes → triggers profile selection
- Invalid profiles → proper error events
- Duplicate profiles → error detection
- Missing ConfigMap → appropriate error handling

## AINIC Tests (`test/k8s-e2e/ainic_test.go`)

Tests validate AINIC configuration workflow:
- AINIC ConfigMap creation and mounting
- Node label changes for NIC profile selection
- Card profile application events
- Port profile events
- DCQCN configuration events
- Validation failure events

## E2E Makefile Targets (`test/k8s-e2e/Makefile`)

```bash
make all          # Run all E2E tests (30m timeout)
make dcm_e2e      # DCM tests with optional filter (DCM_E2E_CHECK)
make test-ainic   # AINIC tests only
make lint         # gofmt + goimports + go vet
```

## Test Dependencies

- `gopkg.in/check.v1` — gocheck framework
- `github.com/mittwald/go-helm-client` — Helm operations
- `k8s.io/client-go` — K8s API
- `github.com/stretchr/testify` — Assertions
