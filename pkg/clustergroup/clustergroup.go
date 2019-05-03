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
	"github.com/banzaicloud/pipeline/internal/clustergroup/adapter"
	"github.com/pkg/errors"
)

// ClusterGroupCreateUpdateRequest describes fields of a Create / Update Cluster Group request
type ClusterGroupCreateUpdateRequest struct {
	Name    string   `json:"name" yaml:"name"`
	Members []string `json:"members,omitempty" yaml:"members"`
}

// ClusterGroupCreateResponse describes fields of a Create Cluster Group response
type ClusterGroupCreateResponse struct {
	Name       string `json:"name"`
	ResourceID uint   `json:"id"`
}

// MemberClusterStatus
type MemberClusterStatus struct {
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status" yaml:"status"`
}

// ClusterGroup
type ClusterGroup struct {
	Id             uint                             `json:"id" yaml:"id"`
	UID            string                           `json:"uid" yaml:"uid"`
	Name           string                           `json:"name" yaml:"name"`
	OrganizationID uint                             `json:"organizationId" yaml:"organizationId"`
	Members        []string                         `json:"members,omitempty" yaml:"members"`
	MembersStatus  []MemberClusterStatus            `json:"membersStatus,omitempty" yaml:"membersStatus"`
	MemberClusters map[string]adapter.Cluster `json:"-" yaml:"-"`
}

func (g *ClusterGroup) IsMember(clusterName string) bool {
	for _, m := range g.Members {
		if clusterName == m {
			return true
		}
	}
	return false
}

// ClusterGroupFeatureRequest
type ClusterGroupFeatureRequest struct {
	Enabled    bool        `json:"enabled" yaml:"enabled"`
	Properties interface{} `json:"properties,omitempty" yaml:"properties"`
}

// ClusterGroupFeatureResponse
type ClusterGroupFeatureResponse struct {
	ClusterGroupFeatureRequest
	Status map[string]string `json:"status,omitempty" yaml:"status"`
}

// Validate validates ClusterGroupCreateUpdateRequest request
func (g *ClusterGroupCreateUpdateRequest) Validate() error {

	if len(g.Name) == 0 {
		return errors.New("cluster group name is empty")
	}

	if len(g.Members) == 0 {
		return errors.New("there should be at least one cluster member")
	}
	return nil
}
