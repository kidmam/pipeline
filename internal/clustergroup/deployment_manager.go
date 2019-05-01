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
	"encoding/json"
	"fmt"

	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/helm"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/ghodss/yaml"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
	k8sHelm "k8s.io/helm/pkg/helm"
	helm_env "k8s.io/helm/pkg/helm/environment"
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

func (m CGDeploymentManager) installDeploymentOnCluster(commonCluster cluster.CommonCluster, orgName string, env helm_env.EnvSettings, cgDeployment *clustergroup.ClusterGroupDeployment) error {
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
		env,
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

func (m CGDeploymentManager) createDeploymentModel(clusterGroup *clustergroup.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) (*ClusterGroupDeploymentModel, error) {
	deploymentModel := &ClusterGroupDeploymentModel{
		ClusterGroupID:        clusterGroup.Id,
		DeploymentName:        cgDeployment.Name,
		DeploymentVersion:     cgDeployment.Version,
		DeploymentPackage:     cgDeployment.Package,
		DeploymentReleaseName: cgDeployment.ReleaseName,
		ReUseValues:           cgDeployment.ReUseValues,
		Namespace:             cgDeployment.Namespace,
		OrganizationName:      orgName,
		Wait:                  cgDeployment.Wait,
		Timeout:               cgDeployment.Timeout,
	}
	values, err := json.Marshal(cgDeployment.Values)
	if err != nil {
		return nil, err
	}
	deploymentModel.Values = values
	deploymentModel.ValueOverrides = make([]DeploymentValueOverrides, 0)
	for clusterName, cluster := range clusterGroup.MemberClusters {
		valueOverrideModel := DeploymentValueOverrides{
			ClusterID: cluster.GetID(),
		}
		if valuesOverride, ok := cgDeployment.ValueOverrides[clusterName]; ok {
			marshalledValues, err := json.Marshal(valuesOverride)
			if err != nil {
				return nil, err
			}
			valueOverrideModel.Values = marshalledValues
		}
		deploymentModel.ValueOverrides = append(deploymentModel.ValueOverrides, valueOverrideModel)
	}

	return deploymentModel, nil
}

func (m CGDeploymentManager) CreateDeployment(clusterGroup *clustergroup.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) ([]clustergroup.DeploymentStatus, error) {

	env := helm.GenerateHelmRepoEnv(orgName)
	_, err := helm.GetRequestedChart(cgDeployment.ReleaseName, cgDeployment.Name, cgDeployment.Version, cgDeployment.Package, env)
	if err != nil {
		return nil, fmt.Errorf("error loading chart: %v", err)
	}
	//TODO use already downloaded chart at install

	// save deployment
	deploymentModel, err := m.createDeploymentModel(clusterGroup, orgName, cgDeployment)
	if err != nil {
		return nil, emperror.Wrap(err, "Error creating deployment model")
	}
	err = m.repository.Save(deploymentModel)
	if err != nil {
		return nil, emperror.Wrap(err, "Error saving deployment model")
	}

	// install charts on cluster group members
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.MemberClusters {
		deploymentCount++
		go func(commonCluster cluster.CommonCluster, cgDeployment *clustergroup.ClusterGroupDeployment) {
			clerr := m.installDeploymentOnCluster(commonCluster, orgName, env, cgDeployment)
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

	return targetClusterStatus, nil
}

func (m CGDeploymentManager) getDeploymentFromModel(deploymentModel *ClusterGroupDeploymentModel) (*clustergroup.GetDeploymentResponse, error) {
	deployment := &clustergroup.GetDeploymentResponse{
		ReleaseName:  deploymentModel.DeploymentReleaseName,
		Chart:        "",
		ChartName:    deploymentModel.DeploymentName,
		ChartVersion: deploymentModel.DeploymentVersion,
		Namespace:    deploymentModel.Namespace,
		Version:      0, //deploymentModel.DeploymentVersion ,
		Description:  "",
		CreatedAt:    deploymentModel.CreatedAt,
		Updated:      deploymentModel.UpdatedAt,
	}
	var values map[string]interface{}
	err := json.Unmarshal(deploymentModel.Values, &values)
	if err != nil {
		return nil, err
	}
	deployment.Values = values

	deployment.ValueOverrides = make(map[string]interface{}, 0)
	for _, valueOverrides := range deploymentModel.ValueOverrides {
		if len(valueOverrides.Values) > 0 {
			var unmarshalledValues interface{}
			err = json.Unmarshal(valueOverrides.Values, &unmarshalledValues)
			if err != nil {
				return nil, err
			}
			deployment.ValueOverrides[fmt.Sprintf("%v", valueOverrides.ClusterID)] = unmarshalledValues
		}
	}
	return deployment, nil
}

func (m CGDeploymentManager) GetDeployment(clusterGroup *clustergroup.ClusterGroup, deploymentName string) (*clustergroup.GetDeploymentResponse, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		// TODO create deploymentNotFound error
		//if gorm.IsRecordNotFoundError(err) {
		//	return nil, nil
		//}
		return nil, err
	}
	deployment, err := m.getDeploymentFromModel(deploymentModel)


	// get deployment status for each cluster group member
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
	deployment.TargetClusters = targetClusterStatus

	return deployment, nil
}

func (m CGDeploymentManager) GetAllDeployments(clusterGroup *clustergroup.ClusterGroup) ([]*clustergroup.ListDeploymentResponse, error) {

	deploymentModels, err := m.repository.FindAll(clusterGroup.Id)
	if err != nil {
		// TODO create deploymentNotFound error
		//if gorm.IsRecordNotFoundError(err) {
		//	return nil, nil
		//}
		return nil, err
	}
	resultList := make([]*clustergroup.ListDeploymentResponse, 0)
	for _, deploymentModel := range deploymentModels {
		deployment := &clustergroup.ListDeploymentResponse{
			Name:         deploymentModel.DeploymentReleaseName,
			Chart:        "",
			ChartName:    deploymentModel.DeploymentName,
			ChartVersion: deploymentModel.DeploymentVersion,
			Namespace:    deploymentModel.Namespace,
			Version:      0, //deploymentModel.DeploymentVersion ,
			CreatedAt:    deploymentModel.CreatedAt,
		}
		resultList = append(resultList, deployment)

	}

	return resultList, nil
}
