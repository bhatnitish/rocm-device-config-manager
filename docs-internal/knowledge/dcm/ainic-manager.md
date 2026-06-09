# AINIC Manager Implementation

**File**: `pkg/ainic_manager/ainic_config_manager.go` (~1958 lines)

## Overview

Manages AMD AINIC (AI Network Interface Card) configuration in both Kubernetes and Debian modes. Uses the `nicctl` CLI tool for hardware operations.

## Command Execution

### Execution Methods

```go
func NicctlExecute(args ...string) (string, error)   // bash -c "nicctl <args>"
func DryNicctlExecute(args ...string) (string, error) // exec.Command("nicctl", args...)
func BashExecute(args ...string) (string, error)      // bash -c "nicctl <args>" (same as NicctlExecute)
func DryBashExecute(args ...string) (string, error)   // Direct exec
```

`NicctlExecute` wraps commands as: `bash -c "nicctl " + strings.Join(args, " ")`

### All nicctl Commands Used

**Discovery commands:**
- `nicctl show card --json | jq .` — Get card info
- `nicctl show card device --json | jq .` — Get LIF/device info (RoCE interfaces)
- `nicctl show port --json | jq .` — Get port information
- `nicctl show card profile --json | jq .` — Get applied card profiles

**Card configuration:**
- `nicctl update card profile -p <profile_name> --card <card_id>` — Apply card profile
- `nicctl update card config-preference <--provider|--workload> --card <card_id>` — Set config preference

**Port configuration:**
- `nicctl update port [--all|-p <port_uuid>] --mtu <value>` — Set MTU
- `nicctl update port [--all|-p <port_uuid>] --pause-type <type> --rx-pause <val> --tx-pause <val>` — Set pause settings

**QoS configuration:**
- `nicctl update qos --classification-type <type>` — Set global QoS classification (DSCP/PCP)
- `nicctl update qos [-p <port_uuid>] dscp-to-priority --dscp <csv_values> --priority <level>` — Map DSCP to priority
- `nicctl update qos [-p <port_uuid>] pfc --priority <level> --no-drop <enable|disable>` — Configure PFC
- `nicctl update qos [-p <port_uuid>] scheduling --priority <csv> --rate-limit <csv> --dwrr <csv>` — Set scheduling

**DCQCN configuration:**
- `nicctl update dcqcn --roce-device <device> --profile-id <id> [options]` — Apply DCQCN profile

## ConfigureAINICs() Main Flow

Complete execution sequence:

1. `initializeAINICStatus()` — Clear previous status
2. `ioutil.ReadFile(globals.JsonFilePathAinic)` — Read config file
3. `protojson.Unmarshal(file, &config)` — Parse protobuf JSON
4. **Profile selection**:
   - K8s: read node label `globals.AinicLabelKey` (`dcm.amd.com/nic-config-profile`)
   - Debian: read `config.SelectednodeProfile` from config JSON
   - Empty profile → success (no-op)
5. `ValidateProfile()` — Validates card/port/DCQCN coverage
6. `ApplyCardProfile()` — Apply card profiles via nicctl
7. `VerifyCardProfile()` — Check if profiles applied correctly
   - If not applied: set reboot-needed label, return failure
   - If applied: delete reboot-needed label
8. `ApplyVFtoDevice()` — Write SR-IOV VF count to sysfs
9. `ApplyCardConfigPreference()` — Set workload/provider preference
10. `ParseAndApplyProfile()` — Apply port profiles and DCQCN configs
11. Final success: K8s event + label

Each step generates K8s events and sets node labels on failure.

## Profile Matching Logic

### Card Matching: `getMatchingCards()`

Filters cards using AND logic across filter criteria:

1. **Card ID filter**: If `filter.CardId == "all"`, matches all cards. Otherwise fuzzy match: `strings.Contains(normalize(productName), normalize(filter.CardId))`
2. **PCIe Address filter**: Exact match on normalized BDF (Bus:Device.Function)
3. **Intersection**: First filter sets initial match set, subsequent filters intersect (AND logic)
4. **Normalize**: `strings.ToLower(strings.TrimSpace(s))`

### Validation Sequence: `ValidateProfile()`

1. Node profile exists in config
2. All referenced NIC profiles exist
3. All referenced port profiles exist
4. Port profile field validation (DSCP, PFC, scheduling)
5. NIC profile has match filters
6. DCQCN profile count matches config type:
   - **Workload mode**: 8 profiles per device
   - **Provider mode**: 1 profile per device
7. Card coverage: all discovered cards claimed by exactly one NIC profile
8. Port coverage: all discovered ports claimed by exactly one port profile
9. DCQCN coverage: all RoCE devices covered by DCQCN entries

## Port Configuration

Applied in order by `ApplyPortProfile()`:

1. **MTU**: `nicctl update port [--all|-p <uuid>] --mtu <value>`
2. **Pause Type**: `nicctl update port [--all|-p <uuid>] --pause-type <type> --rx-pause <val> --tx-pause <val>`
3. **Classification Type**: `nicctl update qos --classification-type <type>` (global setting)
4. **DSCP to Priority**: Comma-separated DSCP values with single priority (0-7)
5. **PFC**: Per-priority PFC configuration with no-drop enable/disable
6. **Scheduling**: CSV-formatted arrays for priority, rate_limit, dwrr

### Port Profile Validation Rules

- **DSCP**: Values 0-63, supports ranges ("10-15"), all 64 values must be covered, no duplicates
- **Priority**: Values 0-7 for all priority fields
- **Scheduling**: Three parallel arrays (priority, rate_limit, dwrr) must be same length

## SR-IOV Configuration

`ApplyVFtoDevice()`:
- Reads `nicProf.VfCount` from NIC profile
- Writes to sysfs: `echo <vf_count> > /sys/bus/pci/devices/<card_bdf>/sriov_numvfs`
- Uses `BashExecute()` for the write operation

## DCQCN Configuration

### Profile Application: `ApplyDcqcnProfile()`

Base command: `nicctl update dcqcn --roce-device <device> --profile-id <index>`

**Conditional parameters** (only appended if value > 0 or specific condition):
- `--disable` (if `Disable == true`)
- `--token-bucket-size <value>`
- `--rate-increase-byte-count <value>`
- `--clamp-target-rate enable` (if `ClampTargetRateEn == true`)
- `--rate-reduce-monitor-period <value>`
- `--initial-alpha-value <value>`
- `--alpha-update-g <value>`
- `--ai-rate <value>` (Additive Increase Rate)
- `--hai-rate <value>` (Hyperaggressive Increase Rate)
- `--alpha-update-interval <value>`
- `--min-rate <value>`
- `--rate-increase-threshold <value>`
- `--rate-increase-interval <value>`
- `--cnp-dscp <value>`

### Multi-Profile Application: `ApplyDCQCNConfigs()`

For each DCQCN entry in NIC profile:
1. Get device list from match filter
2. If `dev_id == "all"`: applies to all RoCE devices in NIC profile
3. For each device: applies ALL profiles in sequence
4. Profile index is **1-indexed**

## RoCE Device Discovery

`ExtractLifIDs()` parses nicctl JSON to extract:
- `lifDB`: NIC ID → LIF IDs
- `lifNameDB`: NIC ID → Ethernet interface names (from `ethernet_interface` field)
- `lifRoceDB`: NIC ID → RoCE interface names (from `roce_interface` field)

`GetLifsPerNicProfile()`: Maps NIC profile name → list of RoCE devices across all cards in that profile.

## K8s Events for AINIC

Events include profile context in message: `"Profile: <context> | <JSON status>"`

### AINIC Event Reasons

| Constant | Value |
|----------|-------|
| `K8EventAINICConfigurationStarted` | "AINICConfigurationStarted" |
| `K8EventAINICConfigurationSuccessful` | "AINICConfigurationSuccessful" |
| `K8EventAINICProfileValidationFailed` | "AINICProfileValidationFailed" |
| `K8EventAINICCardCoverageValidationFailed` | "AINICCardCoverageValidationFailed" |
| `K8EventAINICPortCoverageValidationFailed` | "AINICPortCoverageValidationFailed" |
| `K8EventAINICPortProfileValidationFailed` | "AINICPortProfileValidationFailed" |
| `K8EventAINICDCQCNCoverageValidationFailed` | "AINICDCQCNCoverageValidationFailed" |
| `K8EventAINICCardProfileApplyFailed` | "AINICCardProfileApplyFailed" |
| `K8EventAINICCardProfileApplySuccess` | "AINICCardProfileApplySuccess" |
| `K8EventAINICRebootRequired` | "AINICRebootRequired" |
| `K8EventAINICVFApplyFailed` | "AINICVFApplyFailed" |
| `K8EventAINICVFApplySuccess` | "AINICVFApplySuccess" |
| `K8EventAINICConfigPreferenceApplyFailed` | "AINICConfigPreferenceApplyFailed" |
| `K8EventAINICConfigPreferenceApplySuccess` | "AINICConfigPreferenceApplySuccess" |
| `K8EventAINICPortProfileApplyFailed` | "AINICPortProfileApplyFailed" |
| `K8EventAINICPortProfileApplySuccess` | "AINICPortProfileApplySuccess" |
| `K8EventAINICDCQCNProfileApplyFailed` | "AINICDCQCNProfileApplyFailed" |
| `K8EventAINICDCQCNProfileApplySuccess` | "AINICDCQCNProfileApplySuccess" |
| `K8EventAINICNicctlCommandFailed` | "AINICNicctlCommandFailed" |

## Node Labels

| Label Key | Values | Purpose |
|-----------|--------|---------|
| `dcm.amd.com/nic-config-profile` | Profile name | Selected AINIC profile |
| `dcm.amd.com/nic-config-profile-state` | "success" / "failure" | Configuration result |
| `dcm.amd.com/reboot-needed` | Set/deleted | Card profile requires reboot |

## AINIC Status Structure

```json
{
  "SelectedProfile": "profile-name",
  "FinalStatus": "Success|Failure",
  "Reason": "Description",
  "CardStatus": [
    {
      "CardID": "0",
      "NICProfile": "nicprof1",
      "Status": "Success|Failed",
      "Message": "Detail",
      "LastOperation": "card_profile|vf_count|config_preference|port_profile|dcqcn"
    }
  ]
}
```

## File and Label Watching

```go
func StartFileWatcher(isK8s bool) {
    utils.StartFileWatcher(globals.JsonFilePathAinic, ConfigureAINICs)
}

func NodeLabelWatcher() {
    utils.NodeLabelWatcher(kc, nodeName, globals.AinicLabelKey, ConfigureAINICs)
}
```

## Exported Functions

```go
// Main flow
func ConfigureAINICs()
func GetPartitionProfile() (string, error)

// Watchers
func StartFileWatcher(isK8s bool)
func NodeLabelWatcher()

// Execution
func NicctlExecute(args ...string) (string, error)
func DryNicctlExecute(args ...string) (string, error)
func BashExecute(args ...string) (string, error)
func DryBashExecute(args ...string) (string, error)

// Validation
func ValidateCardCoverage(nicProfiles []string, config *ainic_pb.Config, cardDict map[string][]string) (bool, map[string]string)
func ValidatePortCoverage(nicProfiles []string, config *ainic_pb.Config, nicToCards map[string][]string) (bool, map[string][]string)
func ValidatePortProfile(portProfile *ainic_pb.PortProfile, portProfileName string) error
func ValidateProfile(selectedNodeProfile string, config *ainic_pb.Config, cardDict map[string][]string) (bool, map[string][]string, map[string][]string)

// Application
func ApplyCardProfile(config *ainic_pb.Config, nicToCards map[string][]string, profDB map[string][]string) bool
func ApplyCardConfigPreference(config *ainic_pb.Config, nicToCards map[string][]string) bool
func ApplyVFtoDevice(config *ainic_pb.Config, nicToCards map[string][]string, cardDict map[string][]string) bool
func ApplyPortProfiles(portProfileMap map[string]string, nicToCards map[string][]string, nicProfile string, portProfiles map[string]*ainic_pb.PortProfile) bool
func ApplyPortProfile(portProfile *ainic_pb.PortProfile, portIdentifier string) bool
func ApplyDCQCNConfigs(nicProfile *ainic_pb.NicProfile, nicToLifs map[string][]string, name string, dcqcnProfiles map[string]*ainic_pb.DCQCNProfile) bool
func ApplyDcqcnProfile(profileName string, device string, dcqcnProfile *ainic_pb.DCQCNProfile, index int) bool
func ParseAndApplyProfile(selectedProfile string, config *ainic_pb.Config, nicToCards map[string][]string, nicToLifs map[string][]string) bool

// Discovery
func ExtractLifIDs(jsonStr string) (map[string][]string, map[string][]string, map[string][]string, error)
func ExtractFieldFromNics(jsonStr string, field string) ([]string, error)
func ExtractPortsFromNics(jsonStr string) (map[string][]string, error)
func ExtractCardPortToUUIDMapping(jsonStr string) (map[string]string, error)
func GetAllCardsJson() map[string][]string
func GetAllLifs() (map[string][]string, map[string][]string, map[string][]string)
func GetAllPortsJson() map[string][]string
func GetCardPortToUUIDMapping() map[string]string
func GetCardProfiles() map[string][]string
func GetNicProfToCardClaimMapping(cardClaimedByNic map[string]string) map[string][]string
func GetLifsPerNicProfile(nicToCards map[string][]string, lifRoceDevices map[string][]string) map[string][]string
func GetPortsPerNicProfile(nicToCards map[string][]string, cardToPorts map[string][]string) map[string][]string

// Verification
func VerifyCardProfile(config *ainic_pb.Config, nicToCards map[string][]string, profDB map[string][]string) bool
```
