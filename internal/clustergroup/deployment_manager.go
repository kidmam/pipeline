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
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"
	"github.com/technosophos/moniker"
	k8sHelm "k8s.io/helm/pkg/helm"
	helm_env "k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/proto/hapi/chart"

	"github.com/banzaicloud/pipeline/helm"
	"github.com/banzaicloud/pipeline/internal/clustergroup/api"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgHelm "github.com/banzaicloud/pipeline/pkg/helm"
)

// CGDeploymentManager
type CGDeploymentManager struct {
	clusterGetter api.ClusterGetter
	repository    *CGDeploymentRepository
	logger        logrus.FieldLogger
	errorHandler  emperror.Handler
}

const SUCCEEDED_STATUS = "deployed"
const FAILED_STATUS = "failed"
const DELETED_STATUS = "deleted"
const releaseNameMaxLen = 53

// NewCGDeploymentManager returns a new CGDeploymentManager instance.
func NewCGDeploymentManager(
	db *gorm.DB,
	clusterGetter api.ClusterGetter,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *CGDeploymentManager {
	return &CGDeploymentManager{
		repository: &CGDeploymentRepository{
			db:     db,
			logger: logger,
		},
		clusterGetter: clusterGetter,
		logger:        logger,
		errorHandler:  errorHandler,
	}
}

func (m *CGDeploymentManager) ReconcileState(featureState api.Feature) error {
	//TODO delete deployment from a cluster leaving the group, delete all deployments of a group if featureState.Enabled = false
	m.logger.Infof("reconcile deployments on group: %v", featureState.ClusterGroup.Name)
	return nil
}

func (m *CGDeploymentManager) ValidateState(featureState api.Feature) error {
	return nil
}

func (m *CGDeploymentManager) ValidateProperties(properties interface{}) error {
	return nil
}

func (m *CGDeploymentManager) GetMembersStatus(featureState api.Feature) (map[string]string, error) {
	statusMap := make(map[string]string, 0)
	for _, memberCluster := range featureState.ClusterGroup.Clusters {
		statusMap[memberCluster.GetName()] = "ready"
	}
	return statusMap, nil
}

func (m CGDeploymentManager) installDeploymentOnCluster(commonCluster api.Cluster, orgName string, env helm_env.EnvSettings, cgDeployment *clustergroup.ClusterGroupDeployment, requestedChart *chart.Chart) error {
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

	overrideOpts := []k8sHelm.InstallOption{
		k8sHelm.InstallWait(cgDeployment.Wait),
		k8sHelm.ValueOverrides(marshalledValues),
	}

	if cgDeployment.Timeout > 0 {
		overrideOpts = append(overrideOpts, k8sHelm.InstallTimeout(cgDeployment.Timeout))
	}

	hClient, err := pkgHelm.NewClient(k8sConfig, m.logger)
	if err != nil {
		return err
	}
	defer hClient.Close()

	basicOptions := []k8sHelm.InstallOption{
		k8sHelm.ReleaseName(cgDeployment.ReleaseName),
		k8sHelm.InstallDryRun(cgDeployment.DryRun),
	}
	installOptions := append(helm.DefaultInstallOptions, basicOptions...)
	installOptions = append(installOptions, overrideOpts...)

	release, err := hClient.InstallReleaseFromChart(
		requestedChart,
		cgDeployment.Namespace,
		installOptions...,
	)
	if err != nil {
		return fmt.Errorf("error deploying chart: %v", err)
	}

	m.logger.Infof("Installing deployment on %s succeeded: %s", commonCluster.GetName(), release.String())
	return nil
}

func (m CGDeploymentManager) upgradeDeploymentOnCluster(commonCluster api.Cluster, orgName string, env helm_env.EnvSettings, cgDeployment *clustergroup.ClusterGroupDeployment, requestedChart *chart.Chart) error {
	m.logger.Infof("Upgrading deployment on %s", commonCluster.GetName())
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

	hClient, err := pkgHelm.NewClient(k8sConfig, m.logger)
	if err != nil {
		return err
	}
	defer hClient.Close()

	upgradeRes, err := hClient.UpdateReleaseFromChart(
		cgDeployment.ReleaseName,
		requestedChart,
		k8sHelm.UpdateValueOverrides(marshalledValues),
		k8sHelm.UpgradeDryRun(false),
		//helm.ResetValues(u.resetValues),
		k8sHelm.ReuseValues(false),
	)
	if err != nil {
		return fmt.Errorf("error deploying chart: %v", err)
	}

	m.logger.Infof("Upgrading deployment on %s succeeded: %s", commonCluster.GetName(), upgradeRes.String())
	return nil
}

func (m CGDeploymentManager) getClusterDeploymentStatus(commonCluster api.Cluster, name string) (string, error) {
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

func (m CGDeploymentManager) createDeploymentModel(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment, requestedChart *chart.Chart) (*ClusterGroupDeploymentModel, error) {
	deploymentModel := &ClusterGroupDeploymentModel{
		ClusterGroupID:        clusterGroup.Id,
		DeploymentName:        cgDeployment.Name,
		DeploymentVersion:     cgDeployment.Version,
		DeploymentPackage:     cgDeployment.Package,
		DeploymentReleaseName: cgDeployment.ReleaseName,
		Description:           requestedChart.Metadata.Description,
		ChartName:             requestedChart.Metadata.Name,
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
	for _, cluster := range clusterGroup.Clusters {
		valueOverrideModel := DeploymentValueOverrides{
			ClusterID:   cluster.GetID(),
			ClusterName: cluster.GetName(),
		}
		if valuesOverride, ok := cgDeployment.ValueOverrides[cluster.GetName()]; ok {
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

func (m CGDeploymentManager) updateDeploymentModel(clusterGroup *api.ClusterGroup, deploymentModel *ClusterGroupDeploymentModel, cgDeployment *clustergroup.ClusterGroupDeployment, requestedChart *chart.Chart) error {
	deploymentModel.DeploymentVersion = cgDeployment.Version
	deploymentModel.Description = requestedChart.Metadata.Description
	deploymentModel.ChartName = requestedChart.Metadata.Name

	if cgDeployment.ReUseValues {
		return nil
	}
	//TODO merge values
	values, err := json.Marshal(cgDeployment.Values)
	if err != nil {
		return err
	}
	deploymentModel.Values = values
	deploymentModel.ValueOverrides = make([]DeploymentValueOverrides, 0)
	for _, cluster := range clusterGroup.Clusters {
		valueOverrideModel := DeploymentValueOverrides{
			ClusterID:   cluster.GetID(),
			ClusterName: cluster.GetName(),
		}
		if valuesOverride, ok := cgDeployment.ValueOverrides[cluster.GetName()]; ok {
			marshalledValues, err := json.Marshal(valuesOverride)
			if err != nil {
				return err
			}
			valueOverrideModel.Values = marshalledValues
		}
		deploymentModel.ValueOverrides = append(deploymentModel.ValueOverrides, valueOverrideModel)
	}

	return nil
}

func (m CGDeploymentManager) CreateDeployment(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) ([]clustergroup.DeploymentStatus, error) {

	env := helm.GenerateHelmRepoEnv(orgName)
	requestedChart, err := helm.GetRequestedChart(cgDeployment.ReleaseName, cgDeployment.Name, cgDeployment.Version, cgDeployment.Package, env)
	if err != nil {
		return nil, fmt.Errorf("error loading chart: %v", err)
	}

	if len(cgDeployment.Version) == 0 {
		cgDeployment.Version = requestedChart.Metadata.Version
	}

	if cgDeployment.Namespace == "" {
		log.Warn("Deployment namespace was not set failing back to default")
		cgDeployment.Namespace = helm.DefaultNamespace
	}

	// save deployment
	deploymentModel, err := m.createDeploymentModel(clusterGroup, orgName, cgDeployment, requestedChart)
	if err != nil {
		return nil, emperror.Wrap(err, "Error creating deployment model")
	}
	if !cgDeployment.DryRun {
		err = m.repository.Save(deploymentModel)
		if err != nil {
			return nil, emperror.Wrap(err, "Error saving deployment model")
		}
	}

	// install charts on cluster group members
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.Clusters {
		deploymentCount++
		go func(commonCluster api.Cluster, cgDeployment *clustergroup.ClusterGroupDeployment) {
			clerr := m.installDeploymentOnCluster(commonCluster, orgName, env, cgDeployment, requestedChart)
			status := SUCCEEDED_STATUS
			if clerr != nil {
				status = fmt.Sprintf("%s: %s", FAILED_STATUS, clerr.Error())
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
		Chart:        deploymentModel.DeploymentName,
		ChartName:    deploymentModel.ChartName,
		Description:  deploymentModel.Description,
		ChartVersion: deploymentModel.DeploymentVersion,
		Namespace:    deploymentModel.Namespace,
		CreatedAt:    deploymentModel.CreatedAt,
	}
	if deploymentModel.UpdatedAt != nil {
		deployment.UpdatedAt = *deploymentModel.UpdatedAt
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
			deployment.ValueOverrides[valueOverrides.ClusterName] = unmarshalledValues
		}
	}
	return deployment, nil
}

func (m CGDeploymentManager) GetDeployment(clusterGroup *api.ClusterGroup, deploymentName string) (*clustergroup.GetDeploymentResponse, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		return nil, err
	}
	deployment, err := m.getDeploymentFromModel(deploymentModel)
	if err != nil {
		return nil, err
	}

	// get deployment status for each cluster group member
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)

	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.Clusters {
		deploymentCount++
		go func(commonCluster api.Cluster, name string) {
			status, clErr := m.getClusterDeploymentStatus(commonCluster, name)
			if clErr != nil {
				status = fmt.Sprintf("Failed to get status: %s", clErr.Error())
			}
			statusChan <- clustergroup.DeploymentStatus{
				ClusterId:    commonCluster.GetID(),
				ClusterName:  commonCluster.GetName(),
				Status:       status,
				Cloud:        commonCluster.GetCloud(),
				Distribution: commonCluster.GetDistribution(),
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

func (m CGDeploymentManager) GenerateReleaseName(clusterGroup *api.ClusterGroup) string {
	moniker := moniker.New()
	name := moniker.NameSep("-")
	if len(name) > releaseNameMaxLen {
		name = name[:releaseNameMaxLen]
	}
	return name
}

func (m CGDeploymentManager) GetAllDeployments(clusterGroup *api.ClusterGroup) ([]*clustergroup.ListDeploymentResponse, error) {

	deploymentModels, err := m.repository.FindAll(clusterGroup.Id)
	if err != nil {
		return nil, err
	}
	resultList := make([]*clustergroup.ListDeploymentResponse, 0)
	for _, deploymentModel := range deploymentModels {
		deployment := &clustergroup.ListDeploymentResponse{
			Name:         deploymentModel.DeploymentReleaseName,
			Chart:        deploymentModel.DeploymentName,
			ChartName:    deploymentModel.ChartName,
			ChartVersion: deploymentModel.DeploymentVersion,
			Namespace:    deploymentModel.Namespace,
			CreatedAt:    deploymentModel.CreatedAt,
		}
		if deploymentModel.UpdatedAt != nil {
			deployment.UpdatedAt = *deploymentModel.UpdatedAt
		}
		resultList = append(resultList, deployment)

	}

	return resultList, nil
}

func (m CGDeploymentManager) deleteDeploymentFromCluster(clusterId uint, commonCluster api.Cluster, name string) error {
	if commonCluster == nil {
		m.logger.Warnf("cluster %v is not member of the cluster group anymore", clusterId)
	}

	ctx := context.Background()
	cluster, err := m.clusterGetter.GetClusterByIDOnly(ctx, clusterId)
	if err != nil {
		return errors.Wrap(err, "cluster not found anymore")
	}
	commonCluster = cluster

	m.logger.Infof("delete deployment from %s", commonCluster.GetName())
	k8sConfig, err := commonCluster.GetK8sConfig()
	if err != nil {
		return err
	}

	err = helm.DeleteDeployment(name, k8sConfig)
	if err != nil {
		m.logger.Errorf("DeleteDeployment for '%s' failed due to: %s", name, err.Error())
		return err
	}
	return nil
}

// DeleteDeployment deletes deployments from targeted clusters
func (m CGDeploymentManager) DeleteDeployment(clusterGroup *api.ClusterGroup, deploymentName string, forceDelete bool) ([]clustergroup.DeploymentStatus, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		return nil, err
	}

	// get deployment status for each cluster group member
	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)

	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	// there should be an override for each cluster deployment has been deployed to
	for _, clusterOverride := range deploymentModel.ValueOverrides {
		deploymentCount++
		go func(clusterID uint, commonCluster api.Cluster, name string) {
			clErr := m.deleteDeploymentFromCluster(clusterID, commonCluster, name)
			status := DELETED_STATUS
			if clErr != nil {
				errMsg := fmt.Sprintf("failed to delete cluster: %s", clErr.Error())
				m.logger.Error(errMsg)
				if !forceDelete {
					status = errMsg
				}
			}
			statusChan <- clustergroup.DeploymentStatus{
				ClusterId:   commonCluster.GetID(),
				ClusterName: commonCluster.GetName(),
				Status:      status,
			}
		}(clusterOverride.ClusterID, clusterGroup.Clusters[clusterOverride.ClusterID], deploymentName)
	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	//TODO delete succeeded or all if force
	err = m.repository.Delete(deploymentModel)
	if err != nil {
		return nil, err
	}

	return targetClusterStatus, nil
}

// UpdateDeployment upgrades deployment using provided values or using already provided values if ReUseValues = true.
// The deployment is installed on a member cluster in case it's was not installed previously.
func (m CGDeploymentManager) UpdateDeployment(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *clustergroup.ClusterGroupDeployment) ([]clustergroup.DeploymentStatus, error) {

	env := helm.GenerateHelmRepoEnv(orgName)
	requestedChart, err := helm.GetRequestedChart(cgDeployment.ReleaseName, cgDeployment.Name, cgDeployment.Version, cgDeployment.Package, env)
	if err != nil {
		return nil, fmt.Errorf("error loading chart: %v", err)
	}

	if len(cgDeployment.Version) == 0 {
		cgDeployment.Version = requestedChart.Metadata.Version
	}

	if cgDeployment.Namespace == "" {
		log.Warn("Deployment namespace was not set failing back to default")
		cgDeployment.Namespace = helm.DefaultNamespace
	}

	// get deployment
	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, cgDeployment.Name)
	if err != nil {
		return nil, err
	}

	// if reUseValues = false update values / valueOverrides from request
	err = m.updateDeploymentModel(clusterGroup, deploymentModel, cgDeployment, requestedChart)
	if err != nil {
		return nil, emperror.Wrap(err, "Error updating deployment model")
	}
	err = m.repository.Save(deploymentModel)
	if err != nil {
		return nil, emperror.Wrap(err, "Error saving deployment model")
	}

	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	// upgrade & install deployments
	for _, commonCluster := range clusterGroup.Clusters {
		deploymentCount++
		go func(commonCluster api.Cluster, cgDeployment *clustergroup.ClusterGroupDeployment) {
			//TODO install or upgrade
			clerr := m.upgradeDeploymentOnCluster(commonCluster, orgName, env, cgDeployment, requestedChart)
			status := SUCCEEDED_STATUS
			if clerr != nil {
				status = fmt.Sprintf("%s: %s", FAILED_STATUS, clerr.Error())
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
