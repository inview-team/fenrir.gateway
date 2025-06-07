package http

import "chatops-bot/internal/models"

// ExecuteActionRequest is the request body for the ExecuteAction endpoint.
type ExecuteActionRequest struct {
	Action     string            `json:"action"`
	IncidentID uint              `json:"incident_id"`
	UserID     uint              `json:"user_id"`
	Parameters map[string]string `json:"parameters"`
}

// ExecuteActionResponse is the response body for the ExecuteAction endpoint.
type ExecuteActionResponse struct {
	Result models.ActionResult `json:"result"`
}

// ResourceDetailsRequest is the request body for the ResourceDetails endpoint.
type ResourceDetailsRequest struct {
	IncidentID   uint              `json:"incident_id"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	Labels       map[string]string `json:"labels"`
}

// ResourceDetailsResponse is the response body for the ResourceDetails endpoint.
type ResourceDetailsResponse struct {
	Details *models.ResourceDetails `json:"details,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

// AvailableResourcesResponse is the response body for the AvailableResources endpoint.
type AvailableResourcesResponse struct {
	Resources *models.AvailableResources `json:"resources,omitempty"`
	Error     string                     `json:"error,omitempty"`
}
