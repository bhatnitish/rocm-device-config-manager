# device-config-manager
Device config manager(DCM) is a component of the GPU Operator which is used to handle AMD Devices' configuration. To begin with, we will be handling the GPU partitioning configurations, but it will be flexible to support any kind of GPU configurations (or AINIC configurations) in the future.
Users will provide the GPU configurations using a K8s config-map. The config-map will be associated with the DCM daemonset.

# Steps for partitioning using config map

_Kubernetes Node labels for GPU partitioning_
```bash
dcm.amd.com/gpu-config-profile=<profile_name>
```

-  Create a config map and apply it on the node.
-  Once applied, user has to add the label dcm.amd.com/gpu-config-profile to specify the profile name to be used from the config map.
-  This will trigger the partition using that profile's config.

```bash
dcm.amd.com/gpu-config-profile=profile-1
profile-1 : name of profile created in the configmap
kubectl label node node1 dcm.amd.com/gpu-config-profile=profile-1 
```

-  To change the profile, user can re-apply the `dcm.amd.com/gpu-config-profile` node label with --overwrite=true option
-  The partition status of a node is indicated by the `dcm.amd.com/gpu-config-profile-state` label. This label reflects the state of the partition operation, reporting `success` when partitioning completes successfully and `failure` if an issue occurs during the process.

## ConfigMap

- Please find an example config map in [_example/configmap.yaml_](https://github.com/ROCm/device-config-manager/blob/main/example/configmap.yaml#L1)
- Example config map and it's meaning

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
              "skippedGPUs": {
                  "ids": []
              },
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
      }
    }

```

- `gpu-config-profiles` defines a set of partitioning config profiles from which the user can choose the profile he wants to apply.
- `default` and `profile-1` are example profile names.
- `skippedGPUs` (Optional) list of GPU IDs to skip partitioning
- `computePartition` compute partition type
- `memoryPartition` memory partition type
- `numGPUsAssigned` number of GPUs to be partitioned on the node
- NOTE: User can also create a heterogenous partitioning config profile by mentioning different sets, each set having info about compute/memory types and the number of GPUs to have that partition (refer `default` profile example)

## Configmap Profile Checks

- Let's assume a node with 8 GPUs in it.
### List of profiles checks
- Total number of all `numGPUsAssigned` values of a single profile must be equal to the total number of GPUs on the node.
    - In `default` profile, you can observe that, we are requesting 6 GPUs of type CPX-NPS1 and 2 GPUs of SPX-NPS1 which is valid since it comes to a total of 8 GPUs
    - If `skippedGPUs` field is present, we need to account for those IDs as well.
    - Hence, `Sum of numGPUsAssigned + len(skippedGPUs) = TotalGPUCount`
- `skippedGPUs` field
    - GPU IDs in the list can range from `0` to `total number of GPUs - 1`
    - Length of list must be equal to `total number of GPUs` - `sum of numGPUsAssigned` in that profile
        - Example, in `profile-1`, we have 5 GPUs set to CPX-NPS1 and exactly 3 more GPU IDs mentioned in the skip list
- Compute types supported are SPX and CPX.
    - Beta stage: DPX, QPX
- Memory types supported are NPS1 and NPS4
    - NPS4 is supported only for CPX compute type
    - Combination of NPS1 and NPS4 memory types cannot be used in a single profile


## Supported Platforms
  - Ubuntu 22.04

## RDC version
  - ROCM 6.3, ROCM 6.4

## Build and Run Instructions

### Build dcm binary and bring up dcm container
-  Run the following make target in the TOP directory:
```bash
cd $TOPDIR
make all
```
### Build amddcm application binary only
-  Run the following make target in the TOP directory. This will also generate the required protos to build the DCM application
   	binary.
```bash
cd $TOPDIR
make dcm
```

### Build dcm container only
-  Run the following make target in the TOP directory:
```bash
cd $TOPDIR
make dcm-docker
```

### Partitioning GPUs using DCM
-  GPU on the node cannot be partitioned on the go, we need to bring down all daemonsets using the GPU resource before partitioning. Hence we need to taint the node and the partition.
- DCM pod comes with a toleration
    - `key: amd-dcm , value: up , Operator: Equal, effect: NoExecute `
    - User can specify additional tolerations if required

### Steps for deploying DCM pod
- Add tolerations to the required pods
- Taint the node
- Deploy the DCM pod using a custom resource file
- Once partition is done, untaint the node

#### Taint
-  To TAINT a specific node for partitioning the GPU:
```bash
kubectl taint nodes asrock-126-b3-3b amd-dcm=up:NoExecute
```

#### Add toleration for the taint
-  Since tainting a node will bring down all pods/daemonsets, we need to add toleration to the pods to prevent it from getting evicted.
-  Add toleration to system level pods as well like flannel, proxy etc before tainting the node.
```bash
Example:
kubectl get ds -n kube-flannel kube-flannel-ds -o yaml > fnl.yaml

amd@asrock-126-b3-3b:~$ vi fnl.yaml

#Add this under the spec.template.spec.tolerations object
tolerations:
      - key: "amd-dcm"
        operator: "Equal"
        value: "up"
        effect: "NoExecute"
amd@asrock-126-b3-3b:~$ kubectl apply -f nfd.yaml
```
#### Deploy DCM using a custom resource file
-  Create a CR to bring up the DCM daemonset.
-  Sample CR can be found in [_example/deviceConfigs_example.yaml_](https://github.com/ROCm/device-config-manager/blob/main/example/deviceConfigs_example.yaml#L1)

#### Untaint
```bash
kubectl taint nodes asrock-126-b3-3b amd-dcm:NoExecute-
```

## Deploying Standalone DCM on a cluster
- Create a cluster and setup a worker node to deploy DCM.
- DCM pod can be deployed using it's independent helm-charts as a standalone daemonset without the need of a GPU Operator.
- Steps to deploy:
    - Populate values.yaml to specify image name, tag , nodeSelector, etc.
        - Please find an example values.yaml file in [_helm-charts/values.yaml_](https://github.com/ROCm/device-config-manager/blob/main/helm-charts/values.yaml#L1)
    - Run the below command to build the helm-chart using the values.yaml.

```bash
make helm-install

cd /home/amd/user/device-config-manager/helm-charts; helm lint
==> Linting .
[INFO] Chart.yaml: icon is recommended

1 chart(s) linted, 0 chart(s) failed
helm package helm-charts/ --destination ./helm-charts
Successfully packaged chart and saved it to: helm-charts/device-config-manager-charts-v1.0.0.tgz
cd /home/amd/user/device-config-manager/helm-charts; helm install amd-gpu-operator ./device-config-manager-charts-v1.0.0.tgz -n kube-amd-gpu --create-namespace -f values.yaml
NAME: amd-gpu-operator
LAST DEPLOYED: Thu Apr 3 04:57:29 2025
NAMESPACE: kube-amd-gpu
STATUS: deployed
REVISION: 1
TEST SUITE: None
```
- This internally builds the helm-charts of DCM and then installs the charts in `kube-amd-gpu` namespace.
- DCM daemonset pod is now up and users can perform the partitioning using the labels approach as mentioned above.
- Users can also try the `make helm-build` command to build the helm-charts.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.