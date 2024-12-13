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
)

type PartitionInfo struct {
	DeviceID       int `json:"device_id"`
	ComputePartitions []struct {
		PartitionID    int    `json:"partition_id"`
		PartitionType  string `json:"partition_type"`
		MemoryPartition string `json:"memory_partition"`
	} `json:"compute_partitions"`
}

func getComputePartitionType(partitionType string) C.amdsmi_compute_partition_type_t {
	switch partitionType {
	case "CPX":
		fmt.Println("AMDSMI_COMPUTE_PARTITION_CPX", C.AMDSMI_COMPUTE_PARTITION_CPX)
		return C.AMDSMI_COMPUTE_PARTITION_CPX
	case "SPX":
		fmt.Println("AMDSMI_COMPUTE_PARTITION_SPX", C.AMDSMI_COMPUTE_PARTITION_SPX)
		return C.AMDSMI_COMPUTE_PARTITION_SPX
	default:
		log.Fatalf("Unknown compute partition type: %s", partitionType)
		return C.AMDSMI_COMPUTE_PARTITION_CPX // default value
	}
}

func getMemoryPartitionType(memoryPartition string) C.amdsmi_memory_partition_type_t {
	switch memoryPartition {
	case "NPS1":
		fmt.Println("AMDSMI_MEMORY_PARTITION_NPS1", C.AMDSMI_MEMORY_PARTITION_NPS1)
		return C.AMDSMI_MEMORY_PARTITION_NPS1
	case "NPS4":
		fmt.Println("AMDSMI_MEMORY_PARTITION_NPS4", C.AMDSMI_MEMORY_PARTITION_NPS4)
		return C.AMDSMI_MEMORY_PARTITION_NPS4
	default:
		log.Fatalf("Unknown memory partition type: %s", memoryPartition)
		return C.AMDSMI_MEMORY_PARTITION_NPS1 // default value
	}
}


func readPartitionInfoFromJSON(filename string) (*PartitionInfo, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var partitionInfo PartitionInfo
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

func main() {
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

	filename := "partition.json"

	partitionInfo, err := readPartitionInfoFromJSON(filename)
	fmt.Println("Partition info %+v",partitionInfo)
	if err != nil {
		log.Fatalf("Error reading partition info: %v\n", err)
	}

	for _, partition := range partitionInfo.ComputePartitions {
		computeType := getComputePartitionType(partition.PartitionType)
		memoryType := getMemoryPartitionType(partition.MemoryPartition)

		ret := C.amdsmi_set_gpu_compute_partition(processor_handles[partition.PartitionID], computeType)
		
		if ret != C.AMDSMI_STATUS_SUCCESS {
			fmt.Printf("Failed to partition %v \n", ret)
		}
		fmt.Printf("Successfully configured partition %\n having memory partition %d", partition.PartitionID, memoryType)
		for i:=0; i<len(sockets); i++ {
			processor_handles, device_count = amdsmiGetProcessorHandles(sockets[i])
			if ret != C.AMDSMI_STATUS_SUCCESS {
				fmt.Println("Failed to get socket count")
			}
			fmt.Println("Device count after", device_count)
		}
	}
}
