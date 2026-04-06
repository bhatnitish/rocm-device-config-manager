# Kubernetes configuration

On Kubernetes, GPU profiles come from a `ConfigMap` mounted at `/etc/config-manager/config.json`.

## Helm default

The chart **always** creates the GPU profile `ConfigMap` from `helm-charts/templates/configmap.yaml` and mounts it at `/etc/config-manager/`. The effective name is **`default-dcm-config`** when the `configMap` field is **omitted** from values (or empty). Set `configMap` in overrides only to use a different object name (same chart template content).

Reference copies of the same payload: [docs/examples/default-dcm-config.yaml](https://github.com/ROCm/device-config-manager/blob/main/docs/examples/default-dcm-config.yaml), [helm-charts/default-dcm-config.yaml](https://github.com/ROCm/device-config-manager/blob/main/helm-charts/default-dcm-config.yaml) (for applying or comparing outside Helm).

## Parameters

- Larger sample (test-style profiles): [example/configmap.yaml](https://github.com/ROCm/device-config-manager/blob/main/example/configmap.yaml)

Example:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-manager-config
  namespace: kube-amd-gpu
data:
  config.json: |
    {
      "gpu-config-profiles":
      {
          "default":
          {
              "profiles": [
                  {
                      "computePartition": "CPX",
                      "memoryPartition": "NPS1",
                      "numGPUsAssigned": 6
                  },
                  {
                      "computePartition": "SPX",
                      "memoryPartition": "NPS1",
                      "numGPUsAssigned": 2
                  }
              ]
          },
          "profile-1":
          {
              "skippedGPUs": {
                  "ids": [0, 1, 2]
              },
              "profiles": [
                  {
                      "computePartition": "CPX",
                      "memoryPartition": "NPS1",
                      "numGPUsAssigned": 5
                  }
              ]
          }
      },
      "gpuClientSystemdServices": {
           "names": ["amd-metrics-exporter", "gpuagent"]
       }
    }

```

The numbers above are illustrative (`default` uses 6+2 GPUs; `profile-1` uses 5 assigned plus 3 skipped). They must match **`TotalGPUCount`** on the node (any count: 1, 4, 8, etc.).

- `gpu-config-profiles`: named profiles; the node label selects which profile to apply.
- `skippedGPUs` / `skippedGPUs.ids`: optional. Omit the whole `skippedGPUs` object, or use `{}` / omit `ids`, for no skipped GPUs.
- `computePartition`, `memoryPartition`: partition types.
- `numGPUsAssigned`: see below.
- `gpuClientSystemdServices.names`: systemd units to stop/start around partitioning.

### `numGPUsAssigned` (remainder)

Omitted or `0` is treated as a **remainder** slot (not a second meaning for “all” vs “none”).

- `usable` = `TotalGPUCount - len(skippedGPUs.ids)`
- `remainder` = `usable - sum(explicit numGPUsAssigned)` (explicit = strictly positive counts).
- At most one entry may omit `numGPUsAssigned`; more than one is invalid.

If there is a single profile entry and it omits `numGPUsAssigned`, `remainder = usable` (homogeneous). If several entries exist and explicit counts already equal `usable`, then `remainder = 0` for the omitted row (no GPUs for that row).

The chart default includes a `heterogeneous_example` profile: [docs/examples/default-dcm-config.yaml](https://github.com/ROCm/device-config-manager/blob/main/docs/examples/default-dcm-config.yaml).

## Validation

Rules use **`TotalGPUCount`** (GPUs reported on the node). No fixed GPU count is assumed.

- Per profile: `sum(explicit) + remainder = usable`, with `usable = TotalGPUCount - len(skippedGPUs.ids)`.
- `skippedGPUs`: indices `0` … `TotalGPUCount - 1`; count must match GPUs not covered by the profile assignments.
- Compute: SPX, CPX; beta: DPX, QPX.
- Memory: NPS1, NPS2, NPS4 (one memory type per profile; NPS4 with CPX; NPS2 with DPX).
