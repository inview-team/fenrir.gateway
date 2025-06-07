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
	ReplicasInfo string `json:"replicas_info,omitempty"` // e.g., "2/3"
	Restarts     string `json:"restarts,omitempty"`      // e.g., "5"
	Age          string `json:"age"`
	RawOutput    string `json:"raw_output,omitempty"` // For any extra details
}
