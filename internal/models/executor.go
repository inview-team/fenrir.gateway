package models

// ResourceDetailsRequest defines the request to get details for a specific resource.
type ResourceDetailsRequest struct {
	IncidentID   uint              `json:"incident_id"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	Labels       map[string]string `json:"labels"` // For context, e.g., namespace
}

// ResourceDetails contains the detailed information about a resource.
type ResourceDetails struct {
	Status       string `json:"status"`
	ReplicasInfo string `json:"replicas_info,omitempty"`
	Restarts     int    `json:"restarts,omitempty"`
	Age          string `json:"age"`
	RawOutput    string `json:"raw_output"`
}

type PodInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"age"`
	Resources []struct {
		Name         string `json:"name"`
		CpuUsage     int    `json:"cpuUsage"`
		CpuLimits    int    `json:"cpuLimits"`
		MemoryUsage  int    `json:"memoryUsage"`
		MemoryLimits int    `json:"memoryLimits"`
	} `json:"resources"`
}
