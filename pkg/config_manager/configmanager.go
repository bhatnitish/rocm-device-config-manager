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

package configmanager

/*
#cgo CFLAGS: -I/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi
#cgo LDFLAGS: -L/home/vm/device-config-manager/assets/amd_smi_lib -lamd_smi
#include "/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi.h"
*/
import "C"
import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"time"
	"unsafe"

	"github.com/fsnotify/fsnotify"
	partition_pb "github.com/pensando/device-config-manager/gen/partition"
	"github.com/pensando/device-config-manager/pkg/amdgpu/k8sclient"
	"github.com/pensando/device-config-manager/pkg/config_manager/globals"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var kc *k8sclient.K8sClient = k8sclient.NewClient(context.Background())

func GetPartitionProfile() (string, error) {

	var selectedProfile string
	nodeName := k8sclient.GetNodeName()
	if nodeName == "" {
		err := errors.New("not a k8s deployment")
		return "", err
	}
	labels, err := kc.GetNodeLabel(nodeName)
	if err != nil {
		return "", err
	}

	if len(labels) != 0 {
		gpuConfigProfileNodeLabel := labels[globals.LabelKey]

		if gpuConfigProfileNodeLabel == "" {
			selectedProfile = globals.DefaultProfileName
		} else {
			selectedProfile = gpuConfigProfileNodeLabel
		}

		log.Printf("\nSelected profile name: %+v\n", selectedProfile)
	} else {
		log.Printf("No labels present on node, unusual\n")
	}
	return selectedProfile, nil
}

func checkDaemonSetCount() bool {
	// list all daemon sets and check for ME, NL, TR,
	log.Print("DaemonSets in the cluster:")
	daemonsetlist, partition_alert := kc.GetDaemonSets()
	log.Printf("daemonsetlist %v, partitionalert %v\n", daemonsetlist, partition_alert)
	return partition_alert
}

func StartFileWatcher(selectedProfile string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if checkDaemonSetCount() {
		log.Printf("Cannot partition GPU, please taint the node and then continue")
		return
	}
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
					log.Print("Detected changes in config.json, re-reading the file.")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Print("Error:", err)
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
		log.Print("Failed to get socket count")
		return nil, 0
	}

	// allocating the memory for the sockets
	sockets := make([]C.amdsmi_socket_handle, socketCount)

	// get the actual socket handles
	ret = C.amdsmi_get_socket_handles(&socketCount, &sockets[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to get socket handles")
		return nil, 0
	}

	// return the socket handles and the count
	return sockets, int(socketCount)
}

func amdsmiGetProcessorHandles(socket C.amdsmi_socket_handle) ([]C.amdsmi_processor_handle, int) {
	var device_count C.uint32_t
	ret := C.amdsmi_get_processor_handles(socket, &device_count, nil)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to get device count")
		return nil, 0
	}

	// allocating the memory for the processor
	processors := make([]C.amdsmi_processor_handle, device_count)

	ret = C.amdsmi_get_processor_handles(socket, &device_count, &processors[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to get processor handles")
		return nil, 0
	}

	// return the socket handles and the device count
	return processors, int(device_count)
}

func amdSMIHelper() C.amdsmi_processor_handle {

	log.Print("AMD SMI Initialized successfully.")
	sockets, _ := amdsmiGetSocketHandles()
	var processor_handles []C.amdsmi_processor_handle
	var device_count int

	for i := 0; i < len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		for j := 0; j < device_count; j++ {
			var processor_type C.processor_type_t
			ret := C.amdsmi_get_processor_type(processor_handles[j], &processor_type)
			if ret != 0 {
				log.Printf("Error: %d\n", ret)
			}
			if processor_type != C.AMDSMI_PROCESSOR_TYPE_AMD_GPU {
				log.Print("Expect AMDSMI_PROCESSOR_TYPE_AMD_GPU device type!\n", ret)
			}
		}
	}

	return processor_handles[0]
}

func getActualGPUComputePartition(processor_handle C.amdsmi_processor_handle) string {
	var len C.uint32_t = 4
	computePartition := make([]C.char, len)
	ret := C.amdsmi_get_gpu_compute_partition(processor_handle, &computePartition[0], len)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to get compute partition", ret)
		return ""
	}
	cStr := (*C.char)(unsafe.Pointer(&computePartition[0]))
	return C.GoString(cStr)
}

func shutDownAMDSMI() {
	ret := C.amdsmi_shut_down()
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to shutdown AMD SMI!")
	} else {
		log.Printf("AMD SMI shutdown successfully\n")
	}

	return
}

func generatek8event(err error) {
	k8sPodNamespace := k8sclient.GetPodNameSpace()
	k8sPodName := k8sclient.GetPodName()
	currTime := time.Now().UTC()
	evtObj := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: globals.K8EventPrefixName,
			Namespace:    k8sPodNamespace,
		},
		FirstTimestamp: metav1.Time{
			Time: currTime,
		},
		LastTimestamp: metav1.Time{
			Time: currTime,
		},
		Count:   1,
		Type:    v1.EventTypeWarning,
		Reason:  err.Error(),
		Message: string(err.Error()),
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Namespace: k8sPodNamespace,
			Name:      k8sPodName,
		},
		Source: v1.EventSource{
			Host:      k8sclient.GetNodeName(),
			Component: globals.EventSourceComponentName,
		},
	}
	kc.CreateEvent(evtObj)
}

func ValidateList(config string, validlist []string) bool {
	for _, ctype := range validlist {
		if ctype == config {
			return true
		}
	}
	return false
}

func checkInvalidPartitionType(computeType string, memoryType string) error {

	if !ValidateList(computeType, globals.ValidComputePartitions) {
		err := errors.New("not a valid profile. Invalid compute type.")
		generatek8event(err)
		return err
	}
	if !ValidateList(memoryType, globals.ValidMemoryPartitions) {
		err := errors.New("not a valid profile. Invalid compute type.")
		generatek8event(err)
		return err
	}
	return nil
}

func paritionGPU(selectedProfile string) {

	var currentCompute string
	var currentMemory string
	log.Printf("Paritioning the GPU\n")
	configmap_exist := false
	if _, err := os.Stat(globals.JsonFilePath); os.IsNotExist(err) {
		log.Printf("ConfigMap not present, using default profile")
	} else {
		log.Printf("Reading configmap: %v\n", globals.JsonFilePath)
		configmap_exist = true
	}

	if configmap_exist {
		var profiles partition_pb.GPUConfigProfiles
		file, _ := ioutil.ReadFile(globals.JsonFilePath)
		err := json.Unmarshal(file, &profiles)
		if err != nil {
			log.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		profile, exists := profiles.Profiles[selectedProfile]
		if !exists {
			log.Fatalf("Profile %s not found", selectedProfile)
		}
		currentCompute = profile.ComputePartition
		currentMemory = profile.MemoryPartition
	} else {
		profiles := &partition_pb.GPUConfigProfiles{
			Profiles: make(map[string]*partition_pb.GPUConfigProfile),
		}
		profiles.Profiles[globals.DefaultProfileName] = &partition_pb.GPUConfigProfile{
			ComputePartition: globals.DefaultComputePartition,
			MemoryPartition:  globals.DefaultMemoryPartition,
		}
		profile := profiles.Profiles[globals.DefaultProfileName]
		currentCompute = profile.ComputePartition
		currentMemory = profile.MemoryPartition
	}

	// Initialize the AMD SMI library for GPU
	ret := C.amdsmi_init(C.AMDSMI_INIT_AMD_GPUS)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log.Print("Failed to initialize AMD SMI!")
		return
	}
	defer shutDownAMDSMI()

	processor_handle := amdSMIHelper()

	existingCompute := getActualGPUComputePartition(processor_handle)
	log.Print("Existing Compute Type ", existingCompute)

	if currentCompute == existingCompute {
		log.Printf("Nothing to do, GPU is already in desired compute state %s Selected Profile: %s\n", currentCompute, selectedProfile)
		return
	}

	err := checkInvalidPartitionType(currentCompute, currentMemory)
	if err != nil {
		log.Printf("Invalid compute type %v memory type %v combination", currentCompute, currentMemory)
		return
	}

	if currentCompute != existingCompute {
		log.Printf("Profile: %s, Updated ComputePartition: %s\n", selectedProfile, currentCompute)
		existingCompute = currentCompute
	}

	computeType := getComputePartitionType(currentCompute)

	ret_n := C.amdsmi_set_gpu_compute_partition(processor_handle, computeType)

	if ret_n != C.AMDSMI_STATUS_SUCCESS {
		log.Printf("Failed to partition %v \n", ret_n)
	}

	return
}

func printAndApplyLabelChanges(oldLabels, newLabels map[string]string) {
	// Check for added or updated labels
	for key, newVal := range newLabels {
		if key == globals.TriggerLabelKey && newVal != "" {
			if oldVal, exists := oldLabels[key]; !exists || oldVal != newVal {
				log.Printf("Label changed: %s\nOld value: %s\nNew value: %s\n", key, oldVal, newVal)
				selectedProfile, err := GetPartitionProfile()
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
			log.Printf("Label removed: %s\nOld value: %s\n", key, oldVal)
		}
	}
}

func NodeLabelWatcher() {

	nodeInformer := kc.GetNodeInformer()

	// Set up event handlers for the node informer
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode := oldObj.(*v1.Node)
			newNode := newObj.(*v1.Node)
			if !reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
				printAndApplyLabelChanges(oldNode.Labels, newNode.Labels)
			}
		},
	})

	// Start the informer
	stopCh := make(chan struct{})
	defer close(stopCh)

	go func() {
		// Creating a timer to prevent blockage of code execution
		timer := time.NewTimer(100 * time.Second)
		<-timer.C
		// Stop the Node Informer after the timer expires
	}()

	go nodeInformer.Run(stopCh)

	// Wait for the informer to sync
	if !cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced) {
		log.Fatalf("Failed to sync informers")
	}

	log.Print("Node Informer started and will run for 100 seconds.")
	// Keep the function running
	<-make(chan struct{})
}
