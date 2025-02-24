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

package globals

const (
	// config map json path inside k8
	JsonFilePath            = "/etc/config-manager/config.json"
	DefaultComputePartition = "SPX"
	DefaultMemoryPartition  = "NPS1"
	DefaultProfileName      = "default"
	LabelKey                = "dcm.amd.com/gpu-config-profile"
	TriggerLabelKey         = "dcm.amd.com/apply-gpu-config-profile"

	EventSourceComponentName = "amd-device-config-manager"
	K8EventPrefixName        = "InvalidProfileInfo-"
	K8EventNoPartition       = "NodeNotTaintedBeforeParition-"
	K8EventInvalidProfile    = "InvalidProfileInfo-"
	K8EventSuccessfullyPartitioned = "SuccessfullyPartitioned-"
)

var ValidComputePartitions = []string{"SPX", "CPX", "DPX", "QPX"}
var ValidMemoryPartitions = []string{"NPS1", "NPS4"}
