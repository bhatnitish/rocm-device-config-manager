package types

type PartitionStatus struct {
	SelectedProfile string
	FinalStatus     string
	Reason          string
	GPUStatus       []GPUPartitionStatus
}

type GPUPartitionStatus struct {
	GpuID         int
	PartitionType string
	Status        string
	Message       string
}

type AINICStatus struct {
	SelectedProfile string
	FinalStatus     string
	Reason          string
	CardStatus      []AINICCardStatus
}

type AINICCardStatus struct {
	CardID        string
	NICProfile    string
	Status        string
	Message       string
	LastOperation string
}
