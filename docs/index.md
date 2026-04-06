# Device Config Manager

Device Config Manager (DCM) is part of the GPU Operator. It applies AMD device configuration from Kubernetes, starting with GPU partitioning and AINIC support. Configuration is supplied via a `ConfigMap` mounted on the DCM DaemonSet.

## Configure Device Config Manager

Set fields under `spec.configManager` in the DeviceConfig custom resource (CR).

```yaml
  configManager:
    enable: True

    image: "rocm/device-config-manager:v1.4.0"

    imagePullPolicy: Always

    # GPU profile ConfigMap (volume mount). If omitted or name is empty, the GPU
    # Operator uses "default-dcm-config". Set name to use another ConfigMap.
    # The operator does not create the object; create it in the namespace.
    config:
      name: "default-dcm-config"

    # Default toleration: key amd-dcm, value up, effect NoExecute. Optional extra tolerations:
    configManagerTolerations:
      - key: "key1"
        operator: "Equal"
        value: "value1"
        effect: "NoExecute"

```

The device-config-manager pod starts after the DeviceConfig CR is updated.
