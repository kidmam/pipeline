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

package clustergroup

import (
	"time"
)

// ClusterGroupDeployment describes a Helm deployment to a Cluster Group
type ClusterGroupDeployment struct {
	Name           string                            `json:"name" yaml:"name" binding:"required"`
	Version        string                            `json:"version,omitempty" yaml:"version,omitempty"`
	Package        []byte                            `json:"package,omitempty" yaml:"package,omitempty"`
	ReleaseName    string                            `json:"releaseName" yaml:"releaseName"`
	ReUseValues    bool                              `json:"reuseValues" yaml:"reuseValues"`
	Namespace      string                            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	DryRun         bool                              `json:"dryrun,omitempty" yaml:"dryrun,omitempty"`
	Wait           bool                              `json:"wait,omitempty" yaml:"wait,omitempty"`
	Timeout        int64                             `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Values         map[string]interface{}            `json:"values,omitempty" yaml:"values,omitempty"`
	ValueOverrides map[string]map[string]interface{} `json:"valueOverrides,omitempty" yaml:"valueOverrides,omitempty"`
	RollingMode    bool                              `json:"rollingMode,omitempty" yaml:"rollingMode,omitempty"`
	Atomic         bool                              `json:"atomic,omitempty" yaml:"atomic,omitempty"`
}

// CreateUpdateDeploymentResponse describes a create/update deployment response
type CreateUpdateDeploymentResponse struct {
	ReleaseName    string             `json:"releaseName"`
	TargetClusters []DeploymentStatus `json:"targetClusters"`
}

// DeploymentStatus describes a status of a deployment on a target cluster
type DeploymentStatus struct {
	ClusterId   uint   `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	Status      string `json:"status"`
}

// GetDeploymentResponse describes the details of a helm deployment
type GetDeploymentResponse struct {
	ReleaseName    string                            `json:"releaseName"`
	Chart          string                            `json:"chart"`
	ChartName      string                            `json:"chartName"`
	ChartVersion   string                            `json:"chartVersion"`
	Namespace      string                            `json:"namespace"`
	Version        int32                             `json:"version"`
	Description    string                            `json:"description"`
	CreatedAt      time.Time                         `json:"createdAt,omitempty"`
	Updated        time.Time                         `json:"updatedAt,omitempty"`
	Values         map[string]interface{}            `json:"values"`
	ValueOverrides map[string]map[string]interface{} `json:"valueOverrides,omitempty" yaml:"valueOverrides,omitempty"`
	TargetClusters []DeploymentStatus                `json:"targetClusters"`
}

// ListDeploymentResponse describes a deployment list response
type ListDeploymentResponse struct {
	Name           string             `json:"releaseName"`
	Chart          string             `json:"chart"`
	ChartName      string             `json:"chartName"`
	ChartVersion   string             `json:"chartVersion"`
	Version        int32              `json:"version"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	Namespace      string             `json:"namespace"`
	CreatedAt      time.Time          `json:"createdAt,omitempty"`
	Supported      bool               `json:"supported"`
	WhiteListed    bool               `json:"whiteListed"`
	Rejected       bool               `json:"rejected"`
	TargetClusters []DeploymentStatus `json:"targetClusters"`
}

// DeleteResponse describes a deployment delete response
type DeleteResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Name    string `json:"name"`
}
