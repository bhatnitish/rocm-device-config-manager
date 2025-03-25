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
#cgo CFLAGS: -I/device-config-manager/assets/amd_smi_lib/x86_64/jammy/lib/amd_smi
#cgo LDFLAGS: -L/device-config-manager/assets/amd_smi_lib/x86_64/jammy/lib -lamd_smi -ldrm_amdgpu -ldrm
#include "/device-config-manager/assets/amd_smi_lib/x86_64/jammy/lib/amdsmi.h"
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
	log_e "github.com/sirupsen/logrus"
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
			log.Printf("No profile selected yet, please add the label to select profile")
		} else {
			selectedProfile = gpuConfigProfileNodeLabel
		}

		log.Printf("Selected profile name: %+v\n", selectedProfile)
	} else {
		log.Printf("No labels present on node, unusual\n")
	}
	return selectedProfile, nil
}

func StartFileWatcher(selectedProfile string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

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
					selectedProfile, err := GetPartitionProfile()
					if err != nil {
						log_e.Errorf("err: %+v", err)
					}
					PartitionGPU(selectedProfile)
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

func convertComputePartitonType(partitionType string) C.amdsmi_compute_partition_type_t {
	switch partitionType {
	case "CPX":
		return C.AMDSMI_COMPUTE_PARTITION_CPX
	case "SPX":
		return C.AMDSMI_COMPUTE_PARTITION_SPX
	case "DPX":
		return C.AMDSMI_COMPUTE_PARTITION_DPX
	case "QPX":
		return C.AMDSMI_COMPUTE_PARTITION_QPX
	default:
		log_e.Errorf("Unknown compute partition type: %s, using default type SPX", partitionType)
		return C.AMDSMI_COMPUTE_PARTITION_SPX // default value
	}
}

func convertMemoryPartitionType(memoryPartition string) C.amdsmi_memory_partition_type_t {
	switch memoryPartition {
	case "NPS1":
		return C.AMDSMI_MEMORY_PARTITION_NPS1
	case "NPS4":
		return C.AMDSMI_MEMORY_PARTITION_NPS4
	default:
		log_e.Errorf("Unknown memory partition type: %s, using default type NPS1", memoryPartition)
		return C.AMDSMI_MEMORY_PARTITION_NPS1 // default value
	}
}

func amdsmiGetSocketHandles() ([]C.amdsmi_socket_handle, int) {
	var socketCount C.uint32_t
	ret := C.amdsmi_get_socket_handles(&socketCount, nil)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get socket count")
		return nil, 0
	}

	// allocating the memory for the sockets
	sockets := make([]C.amdsmi_socket_handle, socketCount)

	// get the actual socket handles
	ret = C.amdsmi_get_socket_handles(&socketCount, &sockets[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get socket handles")
		return nil, 0
	}

	// return the socket handles and the count
	return sockets, int(socketCount)
}

func amdsmiGetProcessorHandles(socket C.amdsmi_socket_handle) ([]C.amdsmi_processor_handle, int) {
	var device_count C.uint32_t
	ret := C.amdsmi_get_processor_handles(socket, &device_count, nil)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get device count")
		return nil, 0
	}

	// allocating the memory for the processor
	processors := make([]C.amdsmi_processor_handle, device_count)

	ret = C.amdsmi_get_processor_handles(socket, &device_count, &processors[0])
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get processor handles")
		return nil, 0
	}

	// return the socket handles and the device count
	return processors, int(device_count)
}

func amdsmiGetProcessorType(processor_handle C.amdsmi_processor_handle) (C.processor_type_t, error) {
	var processor_type C.processor_type_t
	ret := C.amdsmi_get_processor_type(processor_handle, &processor_type)
	if ret != 0 {
		log.Printf("Error: %d\n", ret)
		err := errors.New("AMD SMI get processor type failed")
		return processor_type, err
	}
	return processor_type, nil
}

func createGPUIDList(filter_ids []uint32, totalGPUCount int) []int {
	result := []int{}
outer:
	for i := range totalGPUCount {
		for _, fID := range filter_ids {
			if int(fID) == i {
				continue outer
			}
		}
		result = append(result, i)
	}
	return result
}

func validateProfile(profile *partition_pb.GPUConfigProfile, totalGPUCount int) error {
	devices_conf_count := len(profile.Profiles)
	profiles := profile.Profiles
	total_devices := 0
	devicefilter := profile.Filters
	if len(devicefilter.Id) > totalGPUCount {
		log.Printf("Device filter count %d exceeding existing GPU count %d in node", len(devicefilter.Id), totalGPUCount)
		err := errors.New("GPU ID list specified in the device filter is invalid, list length is exceeding the total number of GPUs available on this node")
		log.Printf("ERROR %v", err)
		return err
	}

	for _, id := range devicefilter.Id {
		if int(id)+1 > totalGPUCount {
			log.Printf("Invalid GPU ID specified in skippedGPUs list: %v Valid GPU indices : 0 - %v", id, totalGPUCount-1)
			err := errors.New("invalid gpu id")
			return err
		}
	}

	for i := range devices_conf_count {
		nod := profiles[i].NumGPUsAssigned
		total_devices = total_devices + int(nod)
	}
	if total_devices+len(devicefilter.Id) != totalGPUCount {
		err := errors.New("The total of all numGPUsAssigned values across profiles, combined with the count of IDs in the skippedGPUs list, does not equal the total number of GPUs available on this node.")
		log.Printf("ERROR %v", err)
		return err
	}
	gpu_ids_list := createGPUIDList(devicefilter.Id, totalGPUCount)
	log.Printf("Usable GPU IDs for partitioning %v", gpu_ids_list)
	currentMemory := profiles[0].MemoryPartition
	for i := 0; i < devices_conf_count; i++ {
		currentCompute := profiles[i].ComputePartition
		err := checkInvalidPartitionType(currentCompute, profiles[i].MemoryPartition)
		if err != nil {
			log.Printf("Invalid partition types %v %v", currentCompute, currentMemory)
			return err
		}
		if currentMemory != profiles[i].MemoryPartition {
			log.Printf("All profiles must have a common memory type NPS1 or NPS4")
			err := errors.New("Profile cannot have combination of NPS1 and NPS4 memory types")
			return err
		}
		nod := profiles[i].NumGPUsAssigned
		log.Printf("Partitioning %v devices with compute partition type %v and memory type %v", nod, currentCompute, currentMemory)
	}
	return nil
}

func getCurrentGPUComputePartition(processor_handle C.amdsmi_processor_handle) string {
	var len C.uint32_t = 4
	computePartition := make([]C.char, len)
	ret := C.amdsmi_get_gpu_compute_partition(processor_handle, &computePartition[0], len)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get compute partition %v", ret)
		return ""
	}
	cStr := (*C.char)(unsafe.Pointer(&computePartition[0]))
	return C.GoString(cStr)
}

func getCurrentGPUMemoryPartition(processor_handle C.amdsmi_processor_handle) string {
	var len C.uint32_t = 5
	memoryPartition := make([]C.char, len)
	ret := C.amdsmi_get_gpu_memory_partition(processor_handle, &memoryPartition[0], len)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to get memory partition %v", ret)
		return ""
	}
	cStr := (*C.char)(unsafe.Pointer(&memoryPartition[0]))
	return C.GoString(cStr)
}

func amdSMIHelper(selectedProfile string, profile *partition_pb.GPUConfigProfile) {

	log.Print("AMD SMI Initialized successfully.")
	sockets, _ := amdsmiGetSocketHandles()
	var processor_handles []C.amdsmi_processor_handle
	var device_count int

	totalGPUCount := len(sockets)
	nodeName := k8sclient.GetNodeName()
	log.Print("Total number of GPUs in the node ", totalGPUCount)
	log.Printf("Skipped GPU IDs for partitioning %v", profile.Filters.Id)
	profiles := profile.Profiles
	idx := 0
	err := validateProfile(profile, totalGPUCount)
	if err != nil {
		generateK8sEvent(err, globals.K8EventInvalidProfile)
		err = kc.AddNodeLabel(nodeName, "dcm.amd.com/gpu-config-profile-state", "failure")
		if err != nil {
			log.Printf("Error adding status node label: %s\n", err.Error())
		}
		return
	}
	gpu_ids_list := createGPUIDList(profile.Filters.Id, totalGPUCount)
	for i := 0; i < len(profile.Profiles); i++ {
		currentCompute := profiles[i].ComputePartition
		currentMemory := profiles[i].MemoryPartition
		nod := profiles[i].NumGPUsAssigned
		for j := 0; j < int(nod); j++ {
			log.Printf("Partitioning GPU ID %d with compute partition %v and memory partition %v", gpu_ids_list[idx], currentCompute, currentMemory)
			processor_handles, device_count = amdsmiGetProcessorHandles(sockets[gpu_ids_list[idx]])
			log.Printf("Device count for GPU ID %d : %d", gpu_ids_list[idx], device_count)
			idx = idx + 1
			processor_handle := processor_handles[0]
			processor_type, err := amdsmiGetProcessorType(processor_handle)
			if err != nil {
				log_e.Errorf("Error %v", err)
				return
			}
			if processor_type != C.AMDSMI_PROCESSOR_TYPE_AMD_GPU {
				log.Print("Expected AMDSMI_PROCESSOR_TYPE_AMD_GPU device type!\n")
				continue
			}

			existingCompute := getCurrentGPUComputePartition(processor_handle)
			existingMemory := getCurrentGPUMemoryPartition(processor_handle)

			if (currentCompute == existingCompute) && (currentMemory == existingMemory) {
				continue
			}

			if currentMemory != existingMemory {
				log.Printf("Profile: %s, Existing MemoryPartition: %s\n", selectedProfile, existingMemory)

				memoryType := convertMemoryPartitionType(currentMemory)
				ret_n := C.amdsmi_set_gpu_memory_partition(processor_handle, memoryType)
				time.Sleep(15 * time.Second)
				updatedMemory := getCurrentGPUMemoryPartition(processor_handle)
				if ret_n != C.AMDSMI_STATUS_SUCCESS || (updatedMemory == existingMemory) {
					log_e.Errorf("Failed to memory partition %v \n", ret_n)
					if ret_n == C.AMDSMI_STATUS_BUSY {
						daemonsetList := kc.GetDaemonSets()
						log_e.Errorf("There are existing daemonsets on the cluster %v.\n Please remove the daemonsets keeping the GPU resource busy and retry.", daemonsetList)
						err := errors.New("Taint node and then partition.")
						generateK8sEvent(err, globals.K8EventNoPartition)
						err = kc.AddNodeLabel(nodeName, "dcm.amd.com/gpu-config-profile-state", "failure")
						if err != nil {
							log.Printf("Error adding status node label: %s\n", err.Error())
						}
						return
					}
				} else {
					log.Printf("Successfully partitioned GPU ID %d ", gpu_ids_list[idx])
					log.Printf("Updated Memory Type %v", updatedMemory)
				}
			}

			existingCompute = getCurrentGPUComputePartition(processor_handle)

			if currentCompute != existingCompute {
				log.Printf("Profile: %s, Existing ComputePartition %s\n", selectedProfile, existingCompute)
				existingCompute = currentCompute

				computeType := convertComputePartitonType(currentCompute)

				ret_n := C.amdsmi_set_gpu_compute_partition(processor_handle, computeType)

				updatedCompute := getCurrentGPUComputePartition(processor_handle)
				if ret_n != C.AMDSMI_STATUS_SUCCESS {
					log_e.Errorf("Failed to partition %v \n", ret_n)
					if ret_n == C.AMDSMI_STATUS_BUSY {
						daemonsetList := kc.GetDaemonSets()
						log_e.Errorf("There are existing daemonsets on the cluster %v.\n Please remove the daemonsets keeping the GPU resource busy and retry.", daemonsetList)
						err := errors.New("Taint node and then partition.")
						generateK8sEvent(err, globals.K8EventNoPartition)
						err = kc.AddNodeLabel(nodeName, "dcm.amd.com/gpu-config-profile-state", "failure")
						if err != nil {
							log.Printf("Error adding status node label: %s\n", err.Error())
						}
						return
					}
				} else {
					log.Printf("Successfully partitioned GPU ID %d ", gpu_ids_list[idx])
					log.Printf("Updated Compute Type %v", updatedCompute)
				}
			}
		}
		log.Printf("Successfully Partitioned GPUs of profile %d", i+1)
	}

	log.Printf("Partition completed successfully")
	generateK8sEvent(nil, globals.K8EventSuccessfullyPartitioned)

	err = kc.AddNodeLabel(nodeName, "dcm.amd.com/gpu-config-profile-state", "success")
	if err != nil {
		log.Printf("Error adding status node label: %s\n", err.Error())
		return
	}
}

func shutDownAMDSMI() {
	ret := C.amdsmi_shut_down()
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to shutdown AMD SMI!")
	} else {
		log.Printf("AMD SMI shutdown successfully\n")
	}

	return
}

func generateK8sEvent(err error, event_n string) {
	k8sPodNamespace := k8sclient.GetPodNameSpace()
	k8sPodName := k8sclient.GetPodName()
	currTime := time.Now().UTC()

	eventType := v1.EventTypeNormal
	reason := event_n
	message := "Partition completed successfully."

	if err != nil {
		eventType = v1.EventTypeWarning
		message = err.Error()
	}

	evtObj := createEventObject(event_n, k8sPodNamespace, k8sPodName, currTime, eventType, reason, message)
	kc.CreateEvent(evtObj)
}

func createEventObject(event_n, k8sPodNamespace, k8sPodName string, currTime time.Time, eventType, reason, message string) *v1.Event {
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: event_n,
			Namespace:    k8sPodNamespace,
		},
		FirstTimestamp: metav1.Time{
			Time: currTime,
		},
		LastTimestamp: metav1.Time{
			Time: currTime,
		},
		Count:   1,
		Type:    eventType,
		Reason:  reason,
		Message: message,
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
		generateK8sEvent(err, globals.K8EventInvalidComputeType)
		return err
	}
	if !ValidateList(memoryType, globals.ValidMemoryPartitions) {
		err := errors.New("not a valid profile. Invalid memory type.")
		generateK8sEvent(err, globals.K8EventInvalidMemoryType)
		return err
	}
	return nil
}

func PartitionGPU(selectedProfile string) {

	var profile *partition_pb.GPUConfigProfile
	var exists bool

	log.Printf("Paritioning the GPU\n")
	if _, err := os.Stat(globals.JsonFilePath); os.IsNotExist(err) {
		log.Printf("ConfigMap not present, please configure a configmap to proceed")
		return
	} else {
		log.Printf("Reading configmap: %v\n", globals.JsonFilePath)
	}

	var profiles partition_pb.GPUConfigProfiles
	file, _ := ioutil.ReadFile(globals.JsonFilePath)
	err := json.Unmarshal(file, &profiles)
	if err != nil {
		log_e.Errorf("Failed to unmarshal JSON: %v", err)
		return
	}

	profile, exists = profiles.ProfilesList[selectedProfile]
	if exists {
		log.Printf("Profile found: %v\n", profile)
	} else {
		log.Printf("Profile %v not found.\n", selectedProfile)
		return
	}

	// Initialize the AMD SMI library for GPU
	ret := C.amdsmi_init(C.AMDSMI_INIT_AMD_GPUS)
	if ret != C.AMDSMI_STATUS_SUCCESS {
		log_e.Errorf("Failed to initialize AMD SMI!")
		return
	}
	defer shutDownAMDSMI()

	amdSMIHelper(selectedProfile, profile)
}

func printAndApplyLabelChanges(oldLabels, newLabels map[string]string) {
	// Check for added or updated labels
	for key, newVal := range newLabels {
		if key == globals.LabelKey && newVal != "" {
			if oldVal, exists := oldLabels[key]; !exists || oldVal != newVal {
				log.Printf("Label changed: %s\nOld value: %s\nNew value: %s\n", key, oldVal, newVal)
				selectedProfile, err := GetPartitionProfile()
				if err != nil {
					log_e.Errorf("err: %+v", err)
				}
				PartitionGPU(selectedProfile)
			}
		}
	}

	// Check for removed labels
	for key, oldVal := range oldLabels {
		if _, exists := newLabels[key]; !exists {
			if key == globals.LabelKey {
				log.Printf("Label removed: %s\nOld value: %s\n", key, oldVal)
			}
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
		log_e.Errorf("Failed to sync informers")
	}

	log.Print("Node Informer started and will run for 100 seconds.")
	// Keep the function running
	<-make(chan struct{})
}
