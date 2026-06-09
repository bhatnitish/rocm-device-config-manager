# Kubernetes Client and Utilities

## K8s Client (`pkg/amdgpu/k8sclient/k8s.go`)

### Client Structure

```go
type K8sClient struct {
    sync.Mutex
    ctx       context.Context
    clientset *kubernetes.Clientset
}
```

### Initialization

- **In-cluster only**: Uses `rest.InClusterConfig()` exclusively. No out-of-cluster/kubeconfig support.
- **Lazy initialization**: `reConnect()` calls `init()` only if clientset is nil.
- **Constructor**: `NewClient(ctx context.Context) *K8sClient`

### Node Label Operations

**GetNodeLabel**:
```go
func (k *K8sClient) GetNodeLabel(nodeName string) (map[string]string, error)
```
- Retries: 10 attempts, 30-second intervals
- Returns full label map

**AddNodeLabel**:
```go
func (k *K8sClient) AddNodeLabel(nodeName string, key string, value string) error
```
- Retries: 10 attempts, 10-second intervals
- Fetches latest node on each retry (handles conflicts)
- Updates via `k.clientset.CoreV1().Nodes().Update()`

**DeleteNodeLabel**:
```go
func (k *K8sClient) DeleteNodeLabel(nodeName string, key string) error
```
- Retries: 10 attempts, 10-second intervals
- Returns nil if label not found (no error)

### Event Creation

```go
func (k *K8sClient) CreateEvent(evtObj *v1.Event) error
```
- Uses `context.WithCancel(context.Background())` (not k.ctx)
- Creates via `k.clientset.CoreV1().Events(namespace).Create()`

### Node Informer

```go
func (k *K8sClient) GetNodeInformer(nodeName string) cache.SharedIndexInformer
```
- Creates `SharedInformerFactory` with field selector `metadata.name=<nodeName>`
- Resync period: 0 (no periodic resync)

### KMM NodeModulesConfig Deletion

```go
func (k *K8sClient) DeleteNodeModulesConfig(nodeName string) error
```
- Uses dynamic client for custom resource:
  - Group: `kmm.sigs.x-k8s.io`
  - Version: `v1beta1`
  - Resource: `nodemodulesconfigs`

### Pod/DaemonSet Discovery

```go
func (k *K8sClient) GetDaemonSets() []string              // All namespaces
func (k *K8sClient) GetPods(nodeName string) []string      // Filtered by node name
```

### Environment Variable Helpers

```go
func IsKMMDriverEnabled() bool    // KMM_DRIVER_ENABLED env var
func GetNodeName() string         // DS_NODE_NAME env var
func GetPodName() string          // POD_NAME env var
func GetPodNameSpace() string     // POD_NAMESPACE env var
```

### Thread Safety

All K8sClient methods use `k.Lock()` / `defer k.Unlock()` pattern.

---

## Utilities (`pkg/utils/utils.go`)

### File Watcher

```go
type FileChangeCallback func()

func StartFileWatcher(filePath string, callback FileChangeCallback)
```

- Library: `github.com/fsnotify/fsnotify`
- Events handled: Create, Write, Remove, Rename
- **File existence polling**: 30-minute timeout with 30-second retry intervals before giving up
- **Re-add pattern**: After each event, removes and re-adds file to watcher (handles K8s ConfigMap atomic replace)
- **No debouncing or rate limiting**
- Runs event handler in goroutine, blocks main function with `<-make(chan struct{})`

### Node Label Watcher

```go
type NodeLabelChangeCallback func()

func NodeLabelWatcher(kc interface{}, nodeName string, labelKey string, onLabelChange NodeLabelChangeCallback)
```

- Uses K8s informer pattern via `GetNodeInformer(nodeName)`
- Detects label changes via `reflect.DeepEqual(oldNode.Labels, newNode.Labels)`
- Only triggers callback when watched `labelKey` changes to a non-empty value
- Timer-based: informer runs for 100 seconds per invocation
- Blocks indefinitely after startup

**Label Change Detection** (`PrintAndApplyLabelChanges`):
- Checks added/updated labels: triggers if `labelKey` value changed or newly added with non-empty value
- Checks removed labels: logs removal but does not trigger callback
- Logs: `"\nNEW TRIGGER ALERT FROM NODE LABELS\n"`

### systemd/D-Bus Integration

**D-Bus Connection**:
```go
func getSystemdConn() (*dbus.Conn, error) {
    return dbus.SystemBus()
}
```

**Service Control**:
```go
func controlService(action, serviceName string) error
```
- D-Bus path: `org.freedesktop.systemd1` at `/org/freedesktop/systemd1`
- Interface: `org.freedesktop.systemd1.Manager`
- Methods: `StartUnit` and `StopUnit` with mode `"replace"`

**Start/Stop Functions**:
```go
func StartService(name string) error    // Checks if already active, skips if so
func StopService(name string) error     // Checks if unit exists, skips if not
```

**Unit Status**:
```go
func UnitExists(unitName string) bool       // Sleeps 2s, checks via GetUnit call
func CheckUnitStatus(name string) string    // Sleeps 10s, reads ActiveState property
func CheckUnitStatusHandler(svc string, exp_status string) bool
```

- Property: `org.freedesktop.systemd1.Unit.ActiveState`
- Returns: "active", "inactive", "failed", "not-loaded", or "" on error
- Detects `org.freedesktop.systemd1.NoSuchUnit` error → returns "not-loaded"

**Pre-State Tracking**:

```go
type ServicePreState struct {
    Name      string
    State     string    // "active", "inactive", "failed", "not-loaded"
    Timestamp time.Time
    Comment   string
}
var PreStateDB = make(map[string]ServicePreState)
```

**StopServiceHandler(services []string)**:
1. Ensures `.service` suffix
2. Records current state before stopping (only if not already recorded)
3. Only stops if current state is "active"
4. Validates stopped state is "not-loaded"

**StartServiceHandler(services []string)**:
1. Only restarts if pre-state was "active"
2. Validates final state is "active"
3. Calls `CleanupPreState()` after all services

### Kubernetes Detection

```go
func IsKubernetes() bool
```
- Checks `KUBERNETES_SERVICE_HOST` env var
- Non-empty → true (K8s mode), empty → false (Debian mode)

### CSV Helper

```go
func IntsToCSV(values []uint32) string  // [1,2,3] → "1,2,3"
```

---

## Global Constants (`pkg/globals/constants.go`)

### File Paths

```go
JsonFilePath      = "/etc/config-manager/config.json"
JsonFilePathAinic = "/etc/config-manager-ainic/ainic_config.json"
```

### Label Keys

```go
LabelKey             = "dcm.amd.com/gpu-config-profile"
StateLabelKey        = "dcm.amd.com/gpu-config-profile-state"
AinicLabelKey        = "dcm.amd.com/nic-config-profile"
AinicStateLabelKey   = "dcm.amd.com/nic-config-profile-state"
RebootNeededLabelKey = "dcm.amd.com/reboot-needed"
```

### Partition Defaults

```go
DefaultComputePartition = "SPX"
DefaultMemoryPartition  = "NPS1"
DefaultProfileName      = "default"
ValidComputePartitions  = []string{"SPX", "CPX", "DPX", "QPX"}
ValidMemoryPartitions   = []string{"NPS1", "NPS2", "NPS4", "NPS8"}
```

### KMM Recovery Timeouts

```go
KMMDriverRecoveryUnloadTimeout = 30 * time.Second
KMMDriverRecoveryTimeout       = 5 * time.Minute
KMMDriverRecoveryCheckInterval = 5 * time.Second
```

### Event Source

```go
EventSourceComponentName = "amd-device-config-manager"
```

---

## Status Types (`pkg/interface/types.go`)

```go
type PartitionStatus struct {
    SelectedProfile string
    FinalStatus     string
    Reason          string
    GPUStatus       []GPUPartitionStatus
}

type GPUPartitionStatus struct {
    GpuID         int
    PartitionType string
    Status        string
    Message       string
}

type AINICStatus struct {
    SelectedProfile string
    FinalStatus     string
    Reason          string
    CardStatus      []AINICCardStatus
}

type AINICCardStatus struct {
    CardID        string
    NICProfile    string
    Status        string
    Message       string
    LastOperation string
}
```
