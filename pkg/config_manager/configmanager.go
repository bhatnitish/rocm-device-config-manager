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

	"github.com/fsnotify/fsnotify"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
)

type PartitionProfiles struct { 
	PartitionProfiles map[string]struct { 
		Compute string `json:"compute"`
		Memory string `json:"memory"`
	} `json:"partition-profiles"` 
}

// Global variables
var jsonFilePath = "partition.json"
var previousCompute string 
var selectedProfile string

var (
    paritionName = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "parition_name_change",
            Help: "Tracks the changes in parition name",
        },
        []string{"name"},
    )
)

func init() {
    prometheus.MustRegister(paritionName)
}

func main() {

	// Read profile name from command line argument
	flag.StringVar(&selectedProfile, "profile", "default", "Profile name to monitor")
	flag.Parse()
    // Start Prometheus metrics server
    go func() {
        http.Handle("/metrics", promhttp.Handler())
        log.Fatal(http.ListenAndServe(":8080", nil))
    }()

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Close()

    // Initial read
    paritionGPU()

    // Add the JSON file to the watcher
    err = watcher.Add(jsonFilePath)
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
                if event.Op&fsnotify.Write == fsnotify.Write {
                    log.Println("Detected changes in parition.json, re-reading the file.")
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


func readPartitionInfoFromJSON(filename string) (*PartitionProfiles, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %v: %v",filename, err)
	}

	var partitionInfo PartitionProfiles
	err = json.Unmarshal(file, &partitionInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return &partitionInfo, nil
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

	partitionInfo, err := readPartitionInfoFromJSON(jsonFilePath)
	if err != nil {
		log.Fatalf("Error reading partition info: %v\n", err)
	}

	profile, exists := partitionInfo.PartitionProfiles[selectedProfile]
	if !exists { 
		log.Fatalf("Profile %s not found", selectedProfile) 
	}

	currentCompute := profile.Compute
	if currentCompute != previousCompute { 
		fmt.Printf("Profile: %s, Updated Compute: %s\n", selectedProfile, currentCompute)
		previousCompute = currentCompute 
	}

	computeType := getComputePartitionType(currentCompute)
	memoryType := getMemoryPartitionType(profile.Memory)

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
		fmt.Println("Device count before", device_count)
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
	fmt.Printf("Successfully configured partition %\n having memory partition %d", 0, memoryType)
	for i:=0; i<len(sockets); i++ {
		processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Println("Failed to get socket count")
		}
		fmt.Println("Device count after", device_count)
	}

	// Initialize the AMD SMI library for GPU
    ret = C.amdsmi_shut_down()
    if ret != C.AMDSMI_STATUS_SUCCESS {
        fmt.Println("Failed to shutdown AMD SMI!")
    }
	paritionName.WithLabelValues(currentCompute).Set(1)
}
