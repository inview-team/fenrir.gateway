package models

type AvailableResources struct {
	Profiles []ResourceProfile `json:"profiles"`
}

type ResourceProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}
