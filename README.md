# device-config-manager
Device config manager(DCM) is a component of the GPU Operator which is used to handle AMD Devices' configuration. To begin with, we will be handling the GPU partitioning configurations, but it will be flexible to support any kind of GPU configurations (or AINIC configurations) in the future.
Users will provide the GPU configurations using a K8s config-map. The config-map will be associated with the DCM daemonset.

# Steps for partitioning using config map

_Kubernets Node labels for GPU partitioning_
```
dcm.amd.com/gpu-config-profile=<profile_name>
dcm.amd.com/apply-gpu-config-profile=<any_string>
```

-  Create a config map and apply it on the node.
-  Once applied, user has to add the label amd.com/gpu-config-profile to specify the profile name to be used from the config map.
-  Then, to trigger the partition using that profile's config, user should apply the amd.com/apply-gpu-config-profile with any string, eg. amd.com/apply-gpu-config-profile=apply
-  To change the configs, user can again apply the amd.com/gpu-config-profile node label with --overwrite=true option

## Supported Platforms
  - Ubuntu 22.04

## RDC version
  - ROCM 6.3

## Build and Run Instructions

### Build dcm binary and bring up dcm container
-  Run the following make target in the TOP directory:
   	```
    cd $TOPDIR
    make all
    ```
### Build amddcm application binary only
-  Run the following make target in the TOP directory. This will also generate the required protos to build the DCM application
   	binary.
   	```
    cd $TOPDIR
    make dcm
    ```
### Build dcm container only
-  Run the following make target in the TOP directory:
   	```
    cd $TOPDIR
    make dcm_docker
    ```
### PARTITION GPU
-  GPU on the node cannot be partitioned on the go, we need to bring down all daemonsets using the GPU resource before partitioning. Hence we need to taint the node and add a toleration only to DCM node.
-  TAINT that particular node where you want to partition the GPU.
kubectl taint nodes asrock-126-b3-3b dcm=up:NoExecute
-  Add toleration to amd-gpu-operator-node-feature-discovery-worker daemonset 
-  Add toleration to all other network related pods as well like flannel, proxy etc before tainting the node.
```
kubectl get ds -n kube-amd-gpu amd-gpu-operator-node-feature-discovery-worker -o yaml > nfd.yaml

amd@asrock-126-b3-3b:~$ vi nfd.yaml

#Add this under spec.template.spec object
tolerations:
      - key: "dcm"
        operator: "Equal"
        value: "up"
        effect: "NoExecute"
amd@asrock-126-b3-3b:~$ kubectl apply -f nfd.yaml
```
-  Create a CR to bring up the DCM daemonset along with the toleration for the taint
-  Taint the node
```
kubectl taint nodes asrock-126-b3-3b dcm=up:NoExecute
```

### Untaint
```
kubectl taint nodes asrock-126-b3-3b dcm:NoExecute-
```

### ConfigMap

- Please find an example config map in [_example/configmap.yaml_](https://github.com/pensando/device-config-manager/blob/main/example/configmap.yaml#L1)
- Example config map and it's meaning

```
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
                      "numGPUsAssigned": 1
                  },
                  {
                      "computePartition": "SPX", 
                      "memoryPartition": "NPS1",
                      "numGPUsAssigned": 4
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
                      "numGPUsAssigned": 8
                  }          
              ]
          }
      }
    }

```
- ```gpu-config-profiles``` defines a set of config profiles from which the user can choose the profile he wants to apply.
- ```default``` and ```profile-1``` are example profile names.
- ```skippedGPUs``` field is used to specify a list of GPU IDs to ignore during partioning, can be left blank as well.
- ```computePartition``` field is used to mention compute type of the GPU
- ```memoryPartition``` field is used to mention memory type of the GPU
- ```numGPUsAssigned``` field is used to mention the number of GPUs to be partitioned with the specified compute and memory config
- NOTE: User can also create a heterogenous partitioning config profile by mentioning different sets, each set having info about compute/memory types and the number of GPUs to have that partition (refer ```default``` profile example)