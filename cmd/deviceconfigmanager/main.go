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

import (
	"context"
	"flag"
	"log"
	"os"

	ainicmanager "github.com/ROCm/device-config-manager/pkg/ainic_manager"
	"github.com/ROCm/device-config-manager/pkg/amdgpu/k8sclient"
	configmanager "github.com/ROCm/device-config-manager/pkg/config_manager"
	"github.com/ROCm/device-config-manager/pkg/globals"
	"github.com/ROCm/device-config-manager/pkg/utils"
)

var (
	Version   string
	BuildDate string
	GitCommit string
)

// DeviceCapabilities tracks what device types are available/configured
type DeviceCapabilities struct {
	HasGPUConfig   bool
	HasAINICConfig bool
}

func detectDeviceCapabilities(isKubernetes bool) DeviceCapabilities {
	capabilities := DeviceCapabilities{}

	// In Debian mode (non-Kubernetes), GPU partitioning is not supported yet
	if !isKubernetes {
		// Only check for AINIC config in Debian mode
		if _, err := os.Stat(globals.JsonFilePathAinic); err == nil {
			capabilities.HasAINICConfig = true
		}
	} else {
		// In Kubernetes mode, both GPU and AINIC are supported
		// Check for GPU config file
		if _, err := os.Stat(globals.JsonFilePath); err == nil {
			capabilities.HasGPUConfig = true
		}

		// Check for AINIC config file
		if _, err := os.Stat(globals.JsonFilePathAinic); err == nil {
			capabilities.HasAINICConfig = true
		}
	}

	return capabilities
}

func parseCommandLineFlags() (bool, bool) {
	var enableGPU = flag.Bool("manage-gpu", true, "Enable GPU configuration management")
	var enableAINIC = flag.Bool("manage-ainic", true, "Enable AINIC configuration management")
	flag.Parse()

	return *enableGPU, *enableAINIC
}

func initializeGPUManager(isKubernetes bool) {
	log.Println("Initializing GPU Configuration Manager")

	// Read GPU profile from node labeller
	selectedProfile, err := configmanager.GetPartitionProfile()
	if err != nil {
		log.Printf("Error getting partition profile: %+v", err)
		return
	}

	// Start the GPU worker routine
	go configmanager.Worker()

	// Trigger initial GPU partitioning if profile is available
	if selectedProfile != "" {
		log.Printf("Triggering initial partitioning with profile: %s", selectedProfile)
		configmanager.TriggerRetryLoop(selectedProfile, "initial partitioning")
	}

	// Start GPU file watcher
	go configmanager.StartFileWatcher(selectedProfile)

	// Start GPU node label watcher
	if isKubernetes {
		go configmanager.NodeLabelWatcher()
	}
}

func initializeAINICManager(isKubernetes bool) {
	log.Println("Initializing AINIC Configuration Manager")

	// Perform initial AINIC configuration
	go ainicmanager.ConfigureAINICs()

	// Start AINIC file watcher
	go ainicmanager.StartFileWatcher(isKubernetes)

	// Start AINIC node label watcher if in Kubernetes
	if isKubernetes {
		go ainicmanager.NodeLabelWatcher()
	}
}

func main() {
	log.Printf("####### UNIFIED DEVICE CONFIG MANAGER #######")
	log.Printf("Version : %v", Version)
	log.Printf("BuildDate: %v", BuildDate)
	log.Printf("GitCommit: %v", GitCommit)
	log.Printf("#####################################")

	// Parse command line flags
	enableGPU, enableAINIC := parseCommandLineFlags()

	// Detect deployment environment
	isKubernetes := utils.IsKubernetes()

	// Detect what device configurations are available
	capabilities := detectDeviceCapabilities(isKubernetes)

	// Environment information
	if isKubernetes {
		log.Println("Running inside a Kubernetes pod")
		var kc *k8sclient.K8sClient = k8sclient.NewClient(context.Background())
		var nodeName string = k8sclient.GetNodeName()
		// delete existing dcm labels
		err := kc.DeleteNodeLabel(nodeName, globals.StateLabelKey)
		err = kc.DeleteNodeLabel(nodeName, globals.AinicStateLabelKey)
		if err != nil {
			log.Printf("Error adding status node label: %s\n", err.Error())
		}
	} else {
		log.Println("Running in standalone mode")
	}

	// Determine what to initialize based on capabilities and flags
	var activeManagers []string

	// Initialize GPU Manager if conditions are met (Kubernetes mode only)
	if enableGPU && !isKubernetes {
		log.Println("GPU partitioning not supported in Debian mode - skipping GPU manager")
	} else if enableGPU && capabilities.HasGPUConfig && isKubernetes {
		initializeGPUManager(isKubernetes)
		activeManagers = append(activeManagers, "GPU")
	}

	// Initialize AINIC Manager if conditions are met
	if enableAINIC && capabilities.HasAINICConfig {
		initializeAINICManager(isKubernetes)
		activeManagers = append(activeManagers, "AINIC")
	}

	// Status summary
	if len(activeManagers) == 0 {
		log.Println("No device managers initialized - no configurations found or all disabled")
		log.Println("Ensure configuration files exist:")
		if isKubernetes {
			log.Printf("   - GPU config: %s (Kubernetes mode)", globals.JsonFilePath)
			log.Printf("   - AINIC config: %s", globals.JsonFilePathAinic)
		} else {
			log.Printf("   - AINIC config: %s (Debian mode - GPU not supported)", globals.JsonFilePathAinic)
		}
		os.Exit(1)
	}

	if isKubernetes {
		log.Printf("Active Device Managers (Kubernetes mode): %v", activeManagers)
	} else {
		log.Printf("Active Device Managers (Debian mode): %v", activeManagers)
	}
	log.Printf("Unified DCM startup complete - managing %d device type(s)", len(activeManagers))

	// Keep the program running to maintain all active managers
	<-make(chan struct{})
}
