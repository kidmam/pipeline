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
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CGDeploymentRepository
type CGDeploymentRepository struct {
	db     *gorm.DB
	logger logrus.FieldLogger
}

// FindByName returns a cluster group deployment by name.
func (g *CGDeploymentRepository) FindByName(deploymentName string) (*ClusterGroupDeploymentModel, error) {
	if len(deploymentName) == 0 {
		return nil, errors.New("deployment name is required")
	}
	var result ClusterGroupDeploymentModel
	err := g.db.Where(ClusterGroupDeploymentModel{
		DeploymentName: deploymentName,
	}).Preload("ValueOverrides").First(&result).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, emperror.With(err,
			"deploymentName", deploymentName,
		)
	}

	return &result, nil
}

// FindAll returns all cluster group deployments
func (g *CGDeploymentRepository) FindAll() ([]*ClusterGroupDeploymentModel, error) {
	var deployments []*ClusterGroupDeploymentModel

	err := g.db.Preload("ValueOverrides").Find(&deployments).Error
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch cluster group deployments")
	}

	return deployments, nil
}
