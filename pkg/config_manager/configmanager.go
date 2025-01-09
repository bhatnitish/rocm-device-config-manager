
/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

/*
#cgo CFLAGS: -I/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi
#cgo LDFLAGS: -L/home/vm/device-config-manager/assets/amd_smi_lib -lamd_smi
#include "/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi.h"
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"

	"github.com/fsnotify/fsnotify"
	partition_pb "github.com/pensando/device-config-manager/gen/partition"
	"github.com/pensando/device-config-manager/pkg/amdgpu/k8sclient"
	"github.com/pensando/device-config-manager/pkg/config_manager/globals"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var existingCompute string = "SPX"

func getPartitionProfile() (string, error) {

	var selectedProfile string
	kc := k8sclient.NewClient()
	nodeName := k8sclient.GetNodeName()
	if nodeName == "" {
		err := errors.New("not a k8s deployment")
		return "", err
	}
	labels, err := kc.GetNodelLabel(nodeName)
	if err != nil {
		return "", err
	}

	if len(labels) != 0 {
		gpuConfigProfileNodeLabel := labels[globals.LabelKey]

		if gpuConfigProfileNodeLabel == "" {
			selectedProfile = "default"
		} else {
			selectedProfile = gpuConfigProfileNodeLabel
		}

		fmt.Printf("\nSelected profile name: %+v\n", selectedProfile)
	} else {
		fmt.Printf("No labels present on node, unusual\n")
	}
	return selectedProfile, nil
}

func startFileWatcher(selectedProfile string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// Initial read
	paritionGPU(selectedProfile)

	if _, err := os.Stat(globals.JsonFilePath); os.IsNotExist(err) {
		<-make(chan struct{})
	}
	// Add the JSON file to the watcher
	err = watcher.Add(globals.JsonFilePath)
	if err != nil {
		log.Fatal(err)
	}

	// Watch for changes
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					log.Println("Detected changes in config.json, re-reading the file.")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error:", err)
			}
		}
	}()

	// Keep the program running
	<-make(chan struct{})
}

func getComputePartitionType(partitionType string) C.amdsmi_compute_partition_type_t {
	switch partitionType {
	case "CPX":
		return C.AMDSMI_COMPUTE_PARTITION_CPX
	case "SPX":
		return C.AMDSMI_COMPUTE_PARTITION_SPX
	default:
		log.Fatalf("Unknown compute partition type: %s", partitionType)
		return C.AMDSMI_COMPUTE_PARTITION_CPX // default value
	}
}

func getMemoryPartitionType(memoryPartition string) C.amdsmi_memory_partition_type_t {
	switch memoryPartition {
	case "NPS1":
		return C.AMDSMI_MEMORY_PARTITION_NPS1
	case "NPS4":
		return C.AMDSMI_MEMORY_PARTITION_NPS4
	default:
		log.Fatalf("Unknown memory partition type: %s", memoryPartition)
		return C.AMDSMI_MEMORY_PARTITION_NPS1 // default value
	}
}

func amdsmiGetSocketHandles() ([]C.amdsmi_socket_handle, int) {
	var socketCount C.uint32_t
	ret := C.amdsmi_get_socket_handles(&socketCount, nil)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to get socket count")
		return nil, 0
	}

	// allocating the memory for the sockets
	sockets := make([]C.amdsmi_socket_handle, socketCount)

	// get the actual socket handles
	ret = C.amdsmi_get_socket_handles(&socketCount, &sockets[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to get socket handles")
		return nil, 0
	}

	// return the socket handles and the count
	return sockets, int(socketCount)
}

func amdsmiGetProcessorHandles(socket C.amdsmi_socket_handle) ([]C.amdsmi_processor_handle, int) {
	var device_count C.uint32_t
	ret := C.amdsmi_get_processor_handles(socket, &device_count, nil)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to get device count")
		return nil, 0
	}

	// allocating the memory for the processor
	processors := make([]C.amdsmi_processor_handle, device_count)

	ret = C.amdsmi_get_processor_handles(socket, &device_count, &processors[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to get processor handles")
		return nil, 0
	}

	// return the socket handles and the device count
	return processors, int(device_count)
}

func paritionGPU(selectedProfile string) {

	var currentCompute string
	fmt.Printf("Paritioning the GPU\n")
	configmap_exist := false
	if _, err := os.Stat(globals.JsonFilePath); os.IsNotExist(err) {
		fmt.Printf("failed to read file %v: %v", globals.JsonFilePath, err)
	} else {
		fmt.Printf("Reading file: %v\n", globals.JsonFilePath)
		configmap_exist = true
	}

	// Convert the map to the Protobuf structure
	profiles := &partition_pb.GPUConfigProfiles{
		Profile: make(map[string]*partition_pb.GPUConfigProfile),
	}

	if configmap_exist {
		// Unmarshal the JSON data into a map
		var data map[string]map[string]map[string]string
		file, _ := ioutil.ReadFile(globals.JsonFilePath)
		err := json.Unmarshal(file, &data)
		if err != nil {
			log.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		for key, value := range data["gpu-config-profiles"] {
			profiles.Profile[key] = &partition_pb.GPUConfigProfile{
				ComputePartition: value["compute-partition"],
				MemoryPartition:  value["memory-partition"],
			}
		}

		profile, exists := profiles.Profile[selectedProfile]
		if !exists {
			log.Fatalf("Profile %s not found", selectedProfile)
		}
		currentCompute = profile.ComputePartition
	} else {
		profiles.Profile["default"] = &partition_pb.GPUConfigProfile{
			ComputePartition: globals.DefaultComputePartition,
			MemoryPartition:  globals.DefaultMemoryPartition,
		}
		profile := profiles.Profile["default"]
		currentCompute = profile.ComputePartition
	}

	if currentCompute == existingCompute {
		fmt.Printf("Nothing to do, GPU is already in desired compute state %s\nSelected Profile %s\n", currentCompute, selectedProfile)
		return
	}

	if currentCompute != existingCompute {
		fmt.Printf("Profile: %s, Updated ComputePartition: %s\n", selectedProfile, currentCompute)
		existingCompute = currentCompute
	}

	computeType := getComputePartitionType(currentCompute)
	// Initialize the AMD SMI library for GPU
	ret := C.amdsmi_init(C.AMDSMI_INIT_AMD_GPUS)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to initialize AMD SMI!")
		return
	}

	fmt.Println("AMD SMI Initialized successfully.")
	sockets, _ := amdsmiGetSocketHandles()
	var processor_handles []C.amdsmi_processor_handle
	var device_count int

	for i := 0; i < len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Println("Failed to get socket count")
		}
		for j := 0; j < device_count; j++ {
			var processor_type C.processor_type_t
			ret := C.amdsmi_get_processor_type(processor_handles[j], &processor_type)
			if ret != 0 {
				fmt.Printf("Error: %d\n", ret)
				return
			}
			if processor_type != C.AMDSMI_PROCESSOR_TYPE_AMD_GPU {
				fmt.Println("Expect AMDSMI_PROCESSOR_TYPE_AMD_GPU device type!\n", ret)
			}
		}
	}

	ret_n := C.amdsmi_set_gpu_compute_partition(processor_handles[0], computeType)

	if ret_n != C.AMDSMI_STATUS_SUCCESS {
		fmt.Printf("Failed to partition %v \n", ret_n)
	}

	for i := 0; i < len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Println("Failed to get socket count")
		}
	}

	// Initialize the AMD SMI library for GPU
	ret = C.amdsmi_shut_down()
	if ret != C.AMDSMI_STATUS_SUCCESS {
		fmt.Println("Failed to shutdown AMD SMI!")
	} else {
		fmt.Printf("Successfully configured compute partition\n")
	}
}

func printAndApplyLabelChanges(oldLabels, newLabels map[string]string) {
	// Check for added or updated labels
	for key, newVal := range newLabels {
		if key == globals.TriggerLabelKey && newVal != "" {
			if oldVal, exists := oldLabels[key]; !exists || oldVal != newVal {
				fmt.Printf("Label changed: %s\nOld value: %s\nNew value: %s\n", key, oldVal, newVal)
				selectedProfile, err := getPartitionProfile()
				if err != nil {
					log.Fatalf("err: %+v", err)
				}
				paritionGPU(selectedProfile)
			}
		}
	}

	// Check for removed labels
	for key, oldVal := range oldLabels {
		if _, exists := newLabels[key]; !exists {
			fmt.Printf("Label removed: %s\nOld value: %s\n", key, oldVal)
		}
	}
}

func nodeLabelWatcher() {

	kc := k8sclient.NewClient()
	nodeInformer := kc.GetNodeInformer()

	// Set up event handlers for the node informer
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode := oldObj.(*v1.Node)
			newNode := newObj.(*v1.Node)
			if !reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
				printAndApplyLabelChanges(oldNode.Labels, newNode.Labels)
			} else {
				fmt.Printf("Node %s updated but labels are unchanged.\n", newNode.Name)
			}
		},
	})

	// Start the informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	go nodeInformer.Run(stopCh)

	// Wait for the informer to sync
	if !cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced) {
		panic("Failed to sync informers")
	}

	fmt.Println("Informer is running and synced.")
	// Keep the function running
	<-make(chan struct{})
}

func main() {

	//Read profile from node labeller
	selectedProfile, err := getPartitionProfile()
	if err != nil {
		log.Fatalf("err: %+v", err)
	}

	// starting a seperate go routine for file watcher
	go startFileWatcher(selectedProfile)

	go nodeLabelWatcher()

	// Keep the program running
	<-make(chan struct{})
}
