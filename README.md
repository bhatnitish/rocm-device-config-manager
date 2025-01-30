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

### Build amddcm application binary
-  Run the following make target in the TOP directory. This will also generate the required protos to build the amddcm application
   	binary.
   	```
    cd $TOPDIR
    make amddcm
    ```
   	
### Build dcm container
-  Run the following make target in the TOP directory:
   	```
    cd $TOPDIR
    make docker
    ```

### PARTITION GPU
-  GPU on the node cannot be partitioned on the go, we need to bring down all daemonsets before partitioning. Hence we need to taint the node and add a toleration only to DCM node.
-  TAINT the node where you want to partition the GPU.
kubectl taint nodes asrock-126-b3-3b dcm=up:NoExecute
-  Add toleration to amd-gpu-operator-node-feature-discovery-worker daemonset 
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