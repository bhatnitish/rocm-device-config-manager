# Protobuf Schemas

## Compilation

```bash
make gen              # All protos
make gen-ainic-proto  # protoc --proto_path=proto --go-grpc_out=. --go_out=. proto/ainic.proto
make gen-gpu-proto    # protoc --proto_path=proto --go-grpc_out=. --go_out=. proto/partition.proto
```

Tools: protoc v3.12.4, protoc-gen-go v1.34.2, protoc-gen-go-grpc

## partition.proto

**Package**: `partition`
**Go package**: `gen/partition`

### Enums

```protobuf
enum GPUComputePartitionType {
  GPU_COMPUTE_TYPE_PARTITION_SPX = 0;
  GPU_COMPUTE_TYPE_PARTITION_DPX = 1;
  GPU_COMPUTE_TYPE_PARTITION_QPX = 2;
  GPU_COMPUTE_TYPE_PARTITION_CPX = 3;
}

enum GPUMemoryPartitionType {
  GPU_MEMORY_TYPE_PARTITION_NPS1 = 0;
  GPU_MEMORY_TYPE_PARTITION_NPS4 = 1;
  GPU_MEMORY_TYPE_PARTITION_NPS2 = 2;
}
```

Note: These enums are defined but not used in message fields — config uses string types instead.

### Messages

```protobuf
message ProfileConfig {
  string ComputePartition = 1;   // JSON: "computePartition"
  string MemoryPartition  = 2;   // JSON: "memoryPartition"
  uint32 NumGPUsAssigned  = 3;   // JSON: "numGPUsAssigned"
}

message SkippedGPUs {
  repeated uint32 Id = 1;        // JSON: "ids"
}

message GPUConfigProfile {
  SkippedGPUs Filters = 1;             // JSON: "skippedGPUs"
  repeated ProfileConfig Profiles = 2; // JSON: "profiles"
}

message GPUConfigProfiles {
  map<string, GPUConfigProfile> ProfilesList = 1; // JSON: "gpu-config-profiles"
}

message GPUServiceList {
  repeated string Names = 1;    // JSON: "names"
}

message GPUClientSystemdServices {
  GPUServiceList List = 1;      // JSON: "gpuClientSystemdServices"
}
```

### JSON Example

```json
{
  "gpu-config-profiles": {
    "default": {
      "profiles": [
        { "computePartition": "SPX", "memoryPartition": "NPS1" }
      ]
    },
    "heterogeneous": {
      "skippedGPUs": { "ids": [0, 1] },
      "profiles": [
        { "computePartition": "CPX", "memoryPartition": "NPS1", "numGPUsAssigned": 2 },
        { "computePartition": "SPX", "memoryPartition": "NPS1" }
      ]
    }
  },
  "gpuClientSystemdServices": {
    "names": ["amd-metrics-exporter.service"]
  }
}
```

---

## ainic.proto

**Package**: `nicconfig`
**Go package**: `gen/ainic`

### Enums

```protobuf
enum ClassificationType {
  CLASSIFICATION_TYPE_UNSPECIFIED = 0;
  dscp = 1;
  pcp = 2;
}

enum DeviceType {
  DEVICE_TYPE_UNSPECIFIED = 0;
  PF_ALL = 1;
  VF_ALL = 2;
  ALL = 3;
}
```

### Messages

```protobuf
message DCQCNProfile {
  uint64 TokenBucketSize       = 1;   // JSON: "tokenBucketSize"
  uint32 RateIncreaseByteCount = 2;   // JSON: "rateIncreaseByteCount"
  uint32 RateReduceMonitorPeriod = 3; // JSON: "rateReduceMonitorPeriod"
  uint32 InitialAlphaValue     = 4;   // JSON: "initialAlphaValue"
  uint32 AlphaUpdateG          = 5;   // JSON: "alphaUpdateG"
  uint32 AiRate                = 6;   // JSON: "aiRate"
  uint32 HaiRate               = 7;   // JSON: "haiRate"
  uint32 AlphaUpdateInterval   = 8;   // JSON: "alphaUpdateInterval"
  uint32 CnpDscp               = 9;   // JSON: "cnpDscp"
  bool   Disable               = 10;  // JSON: "disable"
  bool   ClampTargetRateEn     = 11;  // JSON: "clampTargetRateEn"
  uint32 MinRate               = 12;  // JSON: "minRate"
  uint32 RateIncreaseThreshold = 13;  // JSON: "rateIncreaseThreshold"
  uint32 RateIncreaseInterval  = 14;  // JSON: "rateIncreaseInterval"
}

message DscpToPriorityRule {
  repeated string Dscp = 1;  // JSON: "dscp" — values or ranges ("10", "10-15")
  int32 Priority = 2;        // JSON: "priority" — 0-7
}

message PfcConfig {
  int32  Priority = 1;       // JSON: "priority" — 0-7
  string NoDrop   = 2;       // JSON: "no_drop" — "enable"/"disable"
}

message SchedulingConfig {
  repeated uint32 Priority  = 1;  // JSON: "priority"
  repeated uint32 RateLimit = 2;  // JSON: "rate_limit"
  repeated uint32 Dwrr      = 3;  // JSON: "dwrr"
}

message PortProfile {
  string PauseType                    = 1;   // JSON: "pause_type"
  int32  Mtu                          = 2;   // JSON: "mtu"
  string AdminState                   = 3;   // JSON: "admin_state"
  string AutoNeg                      = 4;   // JSON: "auto_neg"
  string FecType                      = 5;   // JSON: "fec_type"
  string RxPause                      = 6;   // JSON: "rx_pause"
  string TxPause                      = 7;   // JSON: "tx_pause"
  string Speed                        = 8;   // JSON: "speed"
  ClassificationType ClassificationType = 9; // JSON: "classification_type"
  repeated DscpToPriorityRule DscpToPriority = 10; // JSON: "dscp_to_priority"
  PfcConfig Pfc                       = 11;  // JSON: "pfc"
  SchedulingConfig Scheduling         = 12;  // JSON: "scheduling"
}

message NicMatchFilter {
  string CardId            = 1;  // JSON: "card_id" — "all" or product name substring
  repeated string PcieAddress = 2; // JSON: "pcie_address" — BDF addresses
}

message DcqcnMatchFilter {
  string DevId = 1;  // JSON: "dev_id" — "all" or specific RoCE device
}

message NicProfileDcqcnEntry {
  repeated DcqcnMatchFilter MatchFiltersDcqcn = 1; // JSON: "match_filters_dcqcn"
  repeated string Profiles = 2;                     // JSON: "profiles" — 1 for provider, 8 for workload
}

message NicProfile {
  repeated NicMatchFilter MatchFilters  = 1; // JSON: "match_filters"
  string CardProfile                    = 2; // JSON: "card_profile"
  string ConfigPreference               = 3; // JSON: "config_preference" — "workload"/"provider"
  map<string, string> PortProfile       = 4; // JSON: "port_profile" — port→profile mapping
  repeated NicProfileDcqcnEntry Dcqcn   = 5; // JSON: "dcqcn"
  int32 VfCount                         = 6; // JSON: "vf_count"
}

message NodeProfile {
  repeated string Nicprofiles = 1; // JSON: "nicprofiles"
  string RebootType           = 2; // JSON: "reboot_type"
}

message Config {
  map<string, DCQCNProfile> DcqcnProfiles   = 1; // JSON: "dcqcn_profiles"
  map<string, PortProfile> PortProfiles      = 2; // JSON: "port_profiles"
  map<string, NicProfile> NicProfiles        = 3; // JSON: "nicProfiles"
  map<string, NodeProfile> NodeProfiles      = 4; // JSON: "nodeProfiles"
  string SelectednodeProfile                 = 5; // JSON: "selectednodeProfile" — Debian mode only
}
```

### JSON Example (AINIC)

```json
{
  "dcqcn_profiles": {
    "p1": {
      "tokenBucketSize": 800000,
      "rateIncreaseByteCount": 431068,
      "rateReduceMonitorPeriod": 1,
      "initialAlphaValue": 64,
      "alphaUpdateG": 512,
      "aiRate": 160,
      "haiRate": 300,
      "alphaUpdateInterval": 1,
      "cnpDscp": 46,
      "clampTargetRateEn": false,
      "minRate": 1,
      "rateIncreaseThreshold": 5,
      "rateIncreaseInterval": 5
    }
  },
  "port_profiles": {
    "pp1": {
      "pause_type": "pfc",
      "mtu": 9064,
      "rx_pause": "enable",
      "tx_pause": "enable",
      "classification_type": "dscp",
      "dscp_to_priority": [
        { "dscp": ["46"], "priority": 6 },
        { "dscp": ["0-9", "11-45", "47-63"], "priority": 1 }
      ],
      "pfc": { "priority": 0, "no_drop": "enable" },
      "scheduling": {
        "priority": [0, 1, 7],
        "rate_limit": [0, 0, 10],
        "dwrr": [99, 1, 0]
      }
    }
  },
  "nicProfiles": {
    "nicprof1": {
      "match_filters": [{ "card_id": "all" }],
      "card_profile": "default",
      "config_preference": "workload",
      "port_profile": { "all": "pp1" },
      "vf_count": 1,
      "dcqcn": [{
        "match_filters_dcqcn": [{ "dev_id": "all" }],
        "profiles": ["p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"]
      }]
    }
  },
  "nodeProfiles": {
    "default": {
      "nicprofiles": ["nicprof1"],
      "reboot_type": "cold"
    }
  },
  "selectednodeProfile": "default"
}
```

## Generated Go Code

**Files**: `gen/partition/partition.pb.go`, `gen/ainic/ainic.pb.go`

Standard protobuf methods generated for each message:
- `Reset()`, `String()`, `ProtoMessage()`, `ProtoReflect()`
- `Get<Field>()` getters with nil-safety
- `Descriptor()` (deprecated)

Enum methods: `Enum()`, `String()`, `Number()`, `Type()`, `Descriptor()`

**Note on JSON serialization**: The GPU config uses `encoding/json` (`json.Unmarshal`) while AINIC config uses `protojson.Unmarshal` from `google.golang.org/protobuf/encoding/protojson`. This means JSON field names follow different conventions — GPU uses the proto-defined JSON names, AINIC uses protojson which follows proto field names with camelCase conversion.
