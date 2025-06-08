package models

type ActionType string

const (
	ActionRollbackDeployment ActionType = "rollback_deployment"
	ActionScaleDeployment    ActionType = "scale_deployment"
	ActionDescribeDeployment ActionType = "describe_deployment"

	ActionGetPodLogs  ActionType = "get_pod_logs"
	ActionDescribePod ActionType = "describe_pod"
	ActionDeletePod   ActionType = "delete_pod"

	ActionListPodsForDeployment ActionType = "list_pods_for_deployment"

	ActionAllocateHardware  ActionType = "allocate_hardware"
	ActionGetDeploymentInfo ActionType = "get_deployment_info"
)

type ActionResult struct {
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	ResultData *ResultData `json:"result_data,omitempty"`
}

type ResourceInfo struct {
	Name      string               `json:"name"`
	Status    string               `json:"status,omitempty"`
	Resources []ContainerResources `json:"resources,omitempty"`
}

type ResultData struct {
	Type     string         `json:"type"`
	Items    []ResourceInfo `json:"items"`
	ItemType string         `json:"item_type,omitempty"`
}

type ActionRequest struct {
	Action     string            `json:"action"`
	IncidentID uint              `json:"incident_id"`
	UserID     uint              `json:"user_id"`
	Parameters map[string]string `json:"parameters"`
}

type SuggestedAction struct {
	HumanReadable string            `json:"human_readable"`
	Action        string            `json:"action"`
	Parameters    map[string]string `json:"parameters"`
}

type ActionTarget struct {
	Type string
	Name string
}
