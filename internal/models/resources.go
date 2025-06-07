package models

// AvailableResources represents the hardware resources available for allocation.
type AvailableResources struct {
	Profiles []ResourceProfile `json:"profiles"`
}

// ResourceProfile defines a specific hardware configuration that can be allocated.
type ResourceProfile struct {
	// Name is a unique identifier for the profile, e.g., "small", "medium-cpu".
	Name string `json:"name"`
	// Description is a human-readable summary, e.g., "1 CPU, 2Gi RAM".
	Description string `json:"description"`
	// IsDefault indicates if this is the default profile.
	IsDefault bool `json:"is_default"`
}
