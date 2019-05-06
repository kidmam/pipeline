// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

// FeatureRequest
type FeatureRequest interface{}

// FeatureResponse
type FeatureResponse struct {
	Properties FeatureRequest    `json:"properties,omitempty" yaml:"properties"`
	Enabled    bool              `json:"enabled"`
	Status     map[string]string `json:"status,omitempty" yaml:"status"`
}

// Feature
type Feature struct {
	Name         string       `json:"name"`
	ClusterGroup ClusterGroup `json:"clusterGroup"`
	Enabled      bool         `json:"enabled"`
	Properties   interface{}  `json:"properties"`
}

type FeatureHandler interface {
	ReconcileState(featureState Feature) error
	ValidateState(featureState Feature) error
	ValidateProperties(properties interface{}) error
	GetMembersStatus(featureState Feature) (map[string]string, error)
}