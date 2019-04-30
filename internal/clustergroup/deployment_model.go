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

const clusterGroupDeploymentTableName = "clustergroup_deployments"
const clusterGroupDeploymentOverridesTableName = "clustergroup_deployment_overrides"

// TableName changes the default table name.
func (ClusterGroupDeploymentModel) TableName() string {
	return clusterGroupDeploymentTableName
}

// TableName changes the default table name.
func (DeploymentValueOverrides) TableName() string {
	return clusterGroupDeploymentOverridesTableName
}

type ClusterGroupDeploymentModel struct {
	ID                    uint `gorm:"primary_key"`
	ClusterGroupID        uint
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time `gorm:"unique_index:idx_unique_id" sql:"index"`
	DeploymentName        string
	DeploymentVersion     string
	DeploymentPackage     []byte
	DeploymentReleaseName string
	ReUseValues           bool
	Namespace             string
	OrganizationName      string
	Wait                  bool
	Timeout               int64
	Values                []byte
	ValueOverrides        []DeploymentValueOverrides `gorm:"foreignkey:ClusterGroupDeploymentID"`
}

// ClusterGroupFeature describes feature param of a cluster group.
type DeploymentValueOverrides struct {
	ID                       uint `gorm:"primary_key"`
	ClusterGroupDeploymentID uint
	ClusterID                uint
	Values                   []byte
}
