package models

type ResourceDetailsRequest struct {
	IncidentID   uint              `json:"incident_id"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	Labels       map[string]string `json:"labels"`
}

type ResourceDetails struct {
	Status       string               `json:"status"`
	ReplicasInfo string               `json:"replicas_info,omitempty"`
	Restarts     int                  `json:"restarts,omitempty"`
	Age          string               `json:"age"`
	RawOutput    string               `json:"raw_output"`
	Resources    []ContainerResources `json:"resources,omitempty"`
}

type ContainerResources struct {
	Name         string `json:"name"`
	CpuUsage     int64  `json:"cpuUsage"`
	MemoryUsage  int64  `json:"memoryUsage"`
	CpuLimits    int64  `json:"cpuLimits"`
	MemoryLimits int64  `json:"memoryLimits"`
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
