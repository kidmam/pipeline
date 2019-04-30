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
	"fmt"

	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/helm"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/ghodss/yaml"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
	k8sHelm "k8s.io/helm/pkg/helm"
)

// CGDeploymentManager
type CGDeploymentManager struct {
	repository   *CGDeploymentRepository
	logger       logrus.FieldLogger
	errorHandler emperror.Handler
}

// NewCGDeploymentManager returns a new CGDeploymentManager instance.
func NewCGDeploymentManager(
	db *gorm.DB,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *CGDeploymentManager {
	return &CGDeploymentManager{
		repository: &CGDeploymentRepository{
			db:     db,
			logger: logger,
		},
		logger:       logger,
		errorHandler: errorHandler,
	}
}

func (m CGDeploymentManager) installDeploymentOnCluster(commonCluster cluster.CommonCluster, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) error {
	m.logger.Infof("Installing deployment on %s", commonCluster.GetName())
	k8sConfig, err := commonCluster.GetK8sConfig()
	if err != nil {
		return err
	}

	values := cgDeployment.Values
	clusterSpecificOverrides, exists := cgDeployment.ValueOverrides[commonCluster.GetName()]
	// merge values with overrides for cluster if any
	if exists {
		values = helm.MergeValues(cgDeployment.Values, clusterSpecificOverrides)
	}
	marshalledValues, err := yaml.Marshal(values)
	if err != nil {
		return err
	}

	installOptions := []k8sHelm.InstallOption{
		k8sHelm.InstallWait(cgDeployment.Wait),
		k8sHelm.ValueOverrides(marshalledValues),
	}

	if cgDeployment.Timeout > 0 {
		installOptions = append(installOptions, k8sHelm.InstallTimeout(cgDeployment.Timeout))
	}

	release, err := helm.CreateDeployment(
		cgDeployment.Name,
		cgDeployment.Version,
		cgDeployment.Package,
		cgDeployment.Namespace,
		cgDeployment.ReleaseName,
		cgDeployment.DryRun,
		nil,
		k8sConfig,
		helm.GenerateHelmRepoEnv(orgName),
		installOptions...,
	)
	if err != nil {
		//TODO distinguish error codes
		return err
	}
	m.logger.Infof("Installing deployment on %s succeeded: %s", commonCluster.GetName(), release.String())
	return nil
}

func (m CGDeploymentManager) getClusterDeploymentStatus(commonCluster cluster.CommonCluster, name string) (string, error) {
	m.logger.Infof("Installing deployment on %s", commonCluster.GetName())
	k8sConfig, err := commonCluster.GetK8sConfig()
	if err != nil {
		return "", err
	}

	deployments, err := helm.ListDeployments(&name, "", k8sConfig)
	if err != nil {
		m.logger.Errorf("ListDeployments for '%s' failed due to: %s", name, err.Error())
		return "", err
	}
	for _, release := range deployments.GetReleases() {
		if release.Name == name {
			return release.Info.Status.Code.String(), nil
		}
	}
	return "unknown", nil
}

func (m CGDeploymentManager) CreateDeployment(clusterGroup *clustergroup.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) []clustergroup.DeploymentStatus {
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.MemberClusters {
		deploymentCount++
		go func(commonCluster cluster.CommonCluster, cgDeployment *clustergroup.ClusterGroupDeployment) {
			clerr := m.installDeploymentOnCluster(commonCluster, orgName, cgDeployment)
			status := "SUCCEEDED"
			if clerr != nil {
				status = fmt.Sprintf("FAILED: %s", clerr.Error())
			}
			statusChan <- clustergroup.DeploymentStatus{
				ClusterId:   commonCluster.GetID(),
				ClusterName: commonCluster.GetName(),
				Status:      status,
			}
		}(commonCluster, cgDeployment)

	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	return targetClusterStatus
}

func (m CGDeploymentManager) GetDeployment(clusterGroup *clustergroup.ClusterGroup, deploymentName string) []clustergroup.DeploymentStatus {
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)

	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.MemberClusters {
		deploymentCount++
		go func(commonCluster cluster.CommonCluster, name string) {
			status, clErr := m.getClusterDeploymentStatus(commonCluster, name)
			if clErr != nil {
				status = fmt.Sprintf("Failed to get status: %s", clErr.Error())
			}
			statusChan <- clustergroup.DeploymentStatus{
				ClusterId:   commonCluster.GetID(),
				ClusterName: commonCluster.GetName(),
				Status:      status,
			}
		}(commonCluster, deploymentName)
	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	return targetClusterStatus
}
