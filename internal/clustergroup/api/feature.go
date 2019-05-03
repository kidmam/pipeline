package api

// FeatureRequest
type FeatureRequest struct {
	Properties interface{} `json:"properties,omitempty" yaml:"properties"`
}

// FeatureResponse
type FeatureResponse struct {
	FeatureRequest
	Enabled bool              `json:"enabled"`
	Status  map[string]string `json:"status,omitempty" yaml:"status"`
}

// Feature
type Feature struct {
	Name         string       `json:"name"`
	ClusterGroup ClusterGroup `json:"clusterGroup"`
	Enabled      bool         `json:"enabled"`
	Properties   interface{}  `json:"properties"`
}
