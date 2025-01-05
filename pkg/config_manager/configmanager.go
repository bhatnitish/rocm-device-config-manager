package main
/*
#cgo CFLAGS: -I/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi
#cgo LDFLAGS: -L/home/vm/device-config-manager/assets/amd_smi_lib -lamd_smi
#include "/home/vm/device-config-manager/assets/amd_smi_lib/amd_smi.h"
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"flag"
	"os"

	"github.com/fsnotify/fsnotify"
	partition_pb "github.com/pensando/device-config-manager/gen/partition"
	"github.com/pensando/device-config-manager/pkg/config_manager/globals"
)

// Global variables
var previousCompute string 
var selectedProfile string
var currentCompute string

func main() {

	// Read profile name from command line argument
	flag.StringVar(&selectedProfile, "profile", "default", "Partition Compute type to monitor")
	flag.Parse()

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Close()

    // Initial read
    paritionGPU()

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
                    paritionGPU()
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

func paritionGPU() {

	configmap_exist :=false
	if _, err := os.Stat(globals.JsonFilePath); os.IsNotExist(err) {
        fmt.Printf("failed to read file %v: %v", globals.JsonFilePath, err)
    } else {
		fmt.Printf("Reading file: %v\n", globals.JsonFilePath)
        configmap_exist = true
	}

	// Convert the map to the Protobuf structure
	profiles := &partition_pb.PartitionProfiles{
		Profile: make(map[string]*partition_pb.PartitionProfile),
	}

	if configmap_exist {
		// Unmarshal the JSON data into a map
		var data map[string]map[string]map[string]string
		file, _ := ioutil.ReadFile(globals.JsonFilePath)
		err := json.Unmarshal(file, &data)
		if err != nil {
			log.Fatalf("Failed to unmarshal JSON: %v", err)
		}
	
		for key, value := range data["partition-profiles"] {
			profiles.Profile[key] = &partition_pb.PartitionProfile{
				Compute: value["compute"],
				Memory:  value["memory"],
			}
		}
	
		profile, exists := profiles.Profile[selectedProfile]
		if !exists { 
			log.Fatalf("Profile %s not found", selectedProfile) 
		}
		currentCompute = profile.Compute
	} else {
		profiles.Profile["default"] = &partition_pb.PartitionProfile{
			Compute: "SPX",
			Memory:  "NPS1",
		}
		profile := profiles.Profile["default"]
		currentCompute = profile.Compute
	}

	if currentCompute != previousCompute { 
		fmt.Printf("Profile: %s, Updated Compute: %s\n", selectedProfile, currentCompute)
		previousCompute = currentCompute 
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

	for i:=0; i<len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Println("Failed to get socket count")
		}
		for j:=0; j<device_count; j++ {
			var processor_type C.processor_type_t 
			ret := C.amdsmi_get_processor_type(processor_handles[j], &processor_type)
			if ret != 0 {
				fmt.Printf("Error: %d\n", ret)
				return
			}
			if (processor_type != C.AMDSMI_PROCESSOR_TYPE_AMD_GPU) {
				fmt.Println("Expect AMDSMI_PROCESSOR_TYPE_AMD_GPU device type!\n", ret)
			}	
		}
	}

	ret_n := C.amdsmi_set_gpu_compute_partition(processor_handles[0], computeType)
	
	if ret_n != C.AMDSMI_STATUS_SUCCESS {
		fmt.Printf("Failed to partition %v \n", ret_n)
	}

	for i:=0; i<len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Println("Failed to get socket count")
		}
	}

	// Initialize the AMD SMI library for GPU
    ret = C.amdsmi_shut_down()
    if ret != C.AMDSMI_STATUS_SUCCESS {
        fmt.Println("Failed to shutdown AMD SMI!")
    }

	fmt.Printf("Successfully configured partition\n")

}