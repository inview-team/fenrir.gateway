package http

type Pod struct {
	Name      string                `json:"name"`
	Status    string                `json:"status"`
	Restarts  int                   `json:"restarts"`
	Age       string                `json:"age"`
	Resources []*ContainerResources `json:"resources"`
}

type ContainerResources struct {
	Name         string `json:"name"`
	CpuUsage     int64  `json:"cpuUsage"`
	MemoryUsage  int64  `json:"memoryUsage"`
	CpuLimits    int64  `json:"cpuLimits"`
	MemoryLimits int64  `json:"memoryLimits"`
}

type Pods struct {
	Pods []DeploymentPod `json:"pods"`
}

type DeploymentPod struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Deployment struct {
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
}
