package globals

const (
	// config map json path inside k8
	JsonFilePath            = "/etc/config-manager/config.json"
	DefaultComputePartition = "SPX"
	DefaultMemoryPartition  = "NPS1"
	LabelKey                = "amd.com/gpu-config-profile"
	TriggerLabelKey         = "amd.com/apply-gpu-config-profile"
)
