# GPU Config Manager Implementation

**File**: `pkg/config_manager/gpu_config_manager.go` (~1069 lines)

## CGo Bindings and AMD SMI Library

### Compiler Directives

```c
#cgo CFLAGS: -I/device-config-manager/build/assets/amd_smi
#cgo LDFLAGS: -L/device-config-manager/build/assets -lamd_smi -ldrm_amdgpu -ldrm
#include "/device-config-manager/build/assets/amdsmi.h"
```

**Libraries linked:**
- `lamd_smi` — AMD System Management Interface library
- `ldrm_amdgpu` — DRM AMDGPU driver library
- `ldrm` — DRM base library

**Include path**: `/device-config-manager/build/assets/amd_smi`
**Library path**: `/device-config-manager/build/assets`
**Header**: `/device-config-manager/build/assets/amdsmi.h`

### AMD SMI Functions Used

| C Function | Purpose |
|-----------|---------|
| `amdsmi_init(C.AMDSMI_INIT_AMD_GPUS)` | Initialize AMD SMI for GPU handling |
| `amdsmi_shut_down()` | Shutdown AMD SMI |
| `amdsmi_get_socket_handles(&count, nil/&sockets[0])` | Two-step: get count, then get handles |
| `amdsmi_get_processor_handles(socket, &count, nil/&handles[0])` | Two-step: get count per socket, then get handles |
| `amdsmi_get_processor_type(handle)` | Verify type is `AMDSMI_PROCESSOR_TYPE_AMD_GPU` |
| `amdsmi_get_gpu_compute_partition(handle, &buf[0], len)` | Get current compute partition (buf=4 bytes) |
| `amdsmi_get_gpu_memory_partition(handle, &buf[0], len)` | Get current memory partition (buf=5 bytes) |
| `amdsmi_get_gpu_memory_partition_config(handle)` | Get supported memory partition types |
| `amdsmi_set_gpu_compute_partition(handle, type)` | Set compute partition |
| `amdsmi_set_gpu_memory_partition(handle, type)` | Set memory partition |
| `amdsmi_gpu_driver_reload(handle)` | Reload GPU drivers (post memory partition change) |

**Return type**: All return `C.AMDSMI_STATUS_SUCCESS` (0) on success.

### AMD SMI Status Codes

Defined in `globals.AmdsmiStatusStrings` map:
- 0: "Call succeeded."
- 1: "Invalid parameters."
- 2: "Command not supported."
- 30: "Device busy." (`AMDSMI_STATUS_BUSY`)
- 31: "Device not found."
- 55: "Setting is not available." (`AMDSMI_STATUS_SETTING_NOT_AVAILABLE` — unsupported partition combination)
- 0xFFFFFFFE: "The internal library error did not map to a status code."
- 0xFFFFFFFF: "An unknown error occurred."

## Partition Types

### Compute Partitions

```go
case "CPX": return C.AMDSMI_COMPUTE_PARTITION_CPX
case "SPX": return C.AMDSMI_COMPUTE_PARTITION_SPX
case "DPX": return C.AMDSMI_COMPUTE_PARTITION_DPX
case "QPX": return C.AMDSMI_COMPUTE_PARTITION_QPX
```

Valid list: `globals.ValidComputePartitions = []string{"SPX", "CPX", "DPX", "QPX"}`
Default: `globals.DefaultComputePartition = "SPX"`

### Memory Partitions

```go
case "NPS1": return C.AMDSMI_MEMORY_PARTITION_NPS1
case "NPS2": return C.AMDSMI_MEMORY_PARTITION_NPS2
case "NPS4": return C.AMDSMI_MEMORY_PARTITION_NPS4
case "NPS8": return C.AMDSMI_MEMORY_PARTITION_NPS8
```

Valid list: `globals.ValidMemoryPartitions = []string{"NPS1", "NPS2", "NPS4", "NPS8"}`
Default: `globals.DefaultMemoryPartition = "NPS1"`

**Partition format stored as**: `"ComputeType-MemoryType"` (e.g., "SPX-NPS1", "CPX-NPS2")

## Worker and Retry Logic

### Global State

```go
var (
    mu         sync.Mutex
    cancelFunc context.CancelFunc
    retryCh    = make(chan string, 1) // Buffered channel, capacity 1
    wg         sync.WaitGroup
)
var kc *k8sclient.K8sClient
var nodeName string
var kmmDriverEnabled bool  // from k8sclient.IsKMMDriverEnabled()
var sockets []C.amdsmi_socket_handle
var totalGPUCount int
var partition_failed bool
var partStatus types.PartitionStatus
```

### Worker()

Reads from `retryCh` channel. If a previous operation is running, cancels it via context, waits for WaitGroup, then starts new `RetryPartition` goroutine.

```go
func Worker() {
    for prof := range retryCh {
        mu.Lock()
        if cancelFunc != nil {
            cancelFunc()    // Cancel previous
            mu.Unlock()
            wg.Wait()       // Wait for completion
            mu.Lock()
        }
        ctx, cancel := context.WithCancel(context.Background())
        cancelFunc = cancel
        wg.Add(1)
        go RetryPartition(ctx, prof)
        mu.Unlock()
    }
}
```

### TriggerRetryLoop()

Sends profile name to `retryCh` (non-blocking due to buffer size 1).

### RetryPartition()

1. Sets 30-minute expiration
2. Reads config JSON from `globals.JsonFilePath`
3. Extracts service list from JSON (`gpu-client-systemd-services`)
4. Retry loop:
   - Checks context cancellation
   - Checks 30-minute expiration → generates `K8EventPartitionFailed` event, restarts services, returns
   - Stops services via `utils.StopServiceHandler(serviceList)`
   - Calls `PartitionGPU(selectedProfile)`
   - On failure: waits 1 minute (context-aware), retries
   - On first failure: generates `K8EventPartitionRetrying` event
   - On success: restarts services, returns

**Timing**: 30-minute total timeout, 1-minute retry intervals.

## PartitionGPU() Flow

1. Read and parse config file (protobuf JSON from `/etc/config-manager/config.json`)
2. Detect duplicate profile keys via `DetectDuplicateKeys()`
3. Look up selected profile in `profiles.ProfilesList[selectedProfile]`
4. Initialize AMD SMI: `amdsmi_init(C.AMDSMI_INIT_AMD_GPUS)`
5. Discover GPU sockets and processors
6. Validate profile against total GPU count
7. For each GPU (skipping filtered IDs):
   - Get current compute and memory partitions
   - Skip if already matching requested partition
   - Set memory partition first (if different)
   - Handle KMM recovery if memory partition fails with KMM enabled
   - Reload drivers (if not KMM): `amdsmi_gpu_driver_reload()`
   - Set compute partition
8. Generate K8s event with final status
9. Set node label: `dcm.amd.com/gpu-config-profile-state` = "success" or "failure"
10. Shutdown AMD SMI: `amdsmi_shut_down()`

### Profile Validation

- Normalizes `SkippedGPUs` if missing
- Validates filter GPU IDs are within range (0 to totalGPUCount-1)
- Validates `numGPUsAssigned`:
  - All explicit: sum must equal usable GPU count
  - One omitted (value 0): gets remainder
  - Multiple omitted: ERROR
- Validates memory partition type is consistent across all profiles
- Validates compute and memory partition types are in valid lists

### Existing Partition Check

Before modifying each GPU, checks current state. If existing compute AND memory match requested, skips with "Success - Partition not required" status.

## Error Handling

**Non-retryable errors** (return immediately with K8s event + failure label):
- ConfigMap not found
- Invalid JSON
- Duplicate profile names
- Profile not found
- AMD SMI initialization failure
- Invalid profile structure
- Unsupported memory partition type
- Unsupported compute-memory combination
- No sockets found

**Retryable errors** (set `partition_failed = true`, retry loop continues):
- Memory partition fails with `AMDSMI_STATUS_BUSY` (code 30)
- Compute partition fails with `AMDSMI_STATUS_BUSY`
- General partition operation failures

## KMM (Kernel Module Management) Recovery

### Detection

```go
var kmmDriverEnabled = k8sclient.IsKMMDriverEnabled()
// Checks env var KMM_DRIVER_ENABLED == "true" (case-insensitive)
```

### Behavior Differences with KMM

1. **Driver reload**: When KMM is **not** enabled, calls `amdsmi_gpu_driver_reload()` after memory partition change. When KMM **is** enabled, skips this call (KMM handles it).

2. **Recovery on memory partition failure**: When KMM is enabled and memory partition fails:
   - Calls `memoryPartitionHandling()`:
     - Runs `modprobe -rv amdgpu` with 5-minute timeout
     - Deletes KMM `NodeModulesConfig` CR via dynamic client:
       - Group: `kmm.sigs.x-k8s.io`
       - Version: `v1beta1`
       - Resource: `nodemodulesconfigs`
   - Waits up to 5 minutes for memory partition to reach expected value (5-second check intervals)
   - Generates intermediate K8s event: "Waiting up to 5 minutes for kmm drivers..."

### KMM Recovery Constants

```go
const (
    KMMDriverRecoveryUnloadTimeout = 30 * time.Second
    KMMDriverRecoveryTimeout       = 5 * time.Minute
    KMMDriverRecoveryCheckInterval = 5 * time.Second
)
```

## Profile Selection

`GetPartitionProfile()` reads node label `dcm.amd.com/gpu-config-profile` via `kc.GetNodeLabel(nodeName)` (10 retries, 30s sleep between attempts). Returns empty string if label not set (no partitioning triggered).

## Config File Format

**Path**: `/etc/config-manager/config.json` (from `globals.JsonFilePath`)

**Parsed via**: `json.Unmarshal(file, &profiles)` into `partition_pb.GPUConfigProfiles`

```json
{
  "gpu-config-profiles": {
    "profile-name": {
      "skippedGPUs": { "ids": [0, 1] },
      "profiles": [
        {
          "computePartition": "SPX",
          "memoryPartition": "NPS1",
          "numGPUsAssigned": 2
        },
        {
          "computePartition": "CPX",
          "memoryPartition": "NPS1"
        }
      ]
    }
  },
  "gpuClientSystemdServices": {
    "names": ["amd-metrics-exporter.service", "gpuagent.service"]
  }
}
```

## K8s Event Generation

Events created via `generateK8sEvent(err, event_n, partStatus)`:
- Source component: `"amd-device-config-manager"`
- InvolvedObject: Pod (from `POD_NAME`, `POD_NAMESPACE` env vars)
- Message: JSON-serialized `PartitionStatus`:

```json
{
  "SelectedProfile": "profile-name",
  "FinalStatus": "Success|Failure|Pending",
  "Reason": "Human-readable reason",
  "GPUStatus": [
    {
      "GpuID": 0,
      "PartitionType": "SPX-NPS1",
      "Status": "Success|Failure|Pending",
      "Message": "Detail message"
    }
  ]
}
```

### GPU Event Reasons

| Constant | Value |
|----------|-------|
| `K8EventInvalidProfile` | "InvalidProfileInfo" |
| `K8EventNonExistentProfile` | "NonExistentProfile" |
| `K8EventSuccessfullyPartitioned` | "SuccessfullyPartitioned" |
| `K8EventPartitionNotNeeded` | "RequestedPartitionConfigAlreadyExists" |
| `K8EventPartitionRetrying` | "PartitionRetrying" |
| `K8EventConfigMapNotPresent` | "ConfigMapNotPresent" |
| `K8EventInvalidJSONInConfigMap` | "InvalidJSONInConfigMap" |
| `K8EventAMDSMIAPIFailure` | "AMDSMIAPIFailure" |
| `K8EventDuplicateProfile` | "DuplicateProfileExists" |
| `K8EventPartitionFailed` | "PartitionFailure" |

## Node Labels

| Label Key | Values | Purpose |
|-----------|--------|---------|
| `dcm.amd.com/gpu-config-profile` | Profile name string | Selected GPU profile (set by user/operator) |
| `dcm.amd.com/gpu-config-profile-state` | "success" / "failure" | Partition result status |
| `dcm.amd.com/reboot-needed` | Set/deleted | Future use flag |

## File Watching

Uses `utils.StartFileWatcher(globals.JsonFilePath, callback)`:
- Library: `github.com/fsnotify/fsnotify`
- Events: Create, Write, Remove, Rename
- On change: reads profile from node label, triggers retry loop
- Re-adds file to watcher after each event (handles ConfigMap atomic replace)

## Node Label Watching

Uses `utils.NodeLabelWatcher(kc, nodeName, globals.LabelKey, callback)`:
- K8s informer with field selector for specific node
- Detects label changes via `reflect.DeepEqual(oldNode.Labels, newNode.Labels)`
- On change of `dcm.amd.com/gpu-config-profile`: triggers retry loop

## Exported Functions

```go
func GetPartitionProfile() (string, error)
func PartitionGPU(selectedProfile string) error
func ValidateList(config string, validlist []string) bool
func StartFileWatcher(selectedProfile string)
func NodeLabelWatcher()
func Worker()
func TriggerRetryLoop(selectedProfile string, funcname string)
func RetryPartition(ctx context.Context, selectedProfile string)
func DetectDuplicateKeys(rawJSON []byte, targetKey string) (error, []string)
```
