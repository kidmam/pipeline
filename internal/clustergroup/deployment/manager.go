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

package deployment

import (
	"context"
	"encoding/json"
	"fmt"

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
	pkgHelm "github.com/banzaicloud/pipeline/pkg/helm"
	hapi_release5 "k8s.io/helm/pkg/proto/hapi/release"
)

// CGDeploymentManager
type CGDeploymentManager struct {
	clusterGetter api.ClusterGetter
	repository    *CGDeploymentRepository
	logger        logrus.FieldLogger
	errorHandler  emperror.Handler
}

const SucceededStatus = "deployed"
const FailedStatus = "failed"
const DeletedStatus = "deleted"
const NotInstalledStatus = "not installed"
const StaleStatus = "stale"
const releaseNameMaxLen = 53

const FeatureName = "deployment"

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
	m.logger.Infof("reconcile deployments on group: %v", featureState.ClusterGroup.Name)

	clusterGroup := featureState.ClusterGroup
	deploymentModels, err := m.repository.FindAll(clusterGroup.Id)
	if err != nil {
		err = emperror.WrapWith(err, "failed to list deployment for cluster group",
			"clusterGroupID", clusterGroup.Id)
		m.logger.Error(err.Error())
	}
	for _, deployment := range deploymentModels {

		if !featureState.Enabled {
			// if feature is disabled delete all deployments belonging to the cluster group
			m.DeleteDeployment(&featureState.ClusterGroup, deployment.DeploymentName, false)
		} else {
			// delete deployment from clusters not belonging to the group anymore
			m.deleteDeploymentFromTargetClusters(&featureState.ClusterGroup, deployment.DeploymentName, deployment, false, false)
		}

	}

	return nil
}

func (m *CGDeploymentManager) ValidateState(featureState api.Feature) error {
	return nil
}

func (m *CGDeploymentManager) ValidateProperties(clusterGroup api.ClusterGroup, properties interface{}) error {
	return nil
}

func (m *CGDeploymentManager) GetMembersStatus(featureState api.Feature) (map[uint]string, error) {
	statusMap := make(map[uint]string, 0)
	return statusMap, nil
}

func (m CGDeploymentManager) installDeploymentOnCluster(apiCluster api.Cluster, orgName string, env helm_env.EnvSettings, depInfo *DeploymentInfo, requestedChart *chart.Chart, dryRun bool) error {
	m.logger.Infof("install cluster group deployment %s on %s", depInfo.ReleaseName, apiCluster.GetName())
	k8sConfig, err := apiCluster.GetK8sConfig()
	if err != nil {
		return err
	}

	values, err := depInfo.GetValuesForCluster(apiCluster.GetName())
	if err != nil {
		return err
	}

	hClient, err := pkgHelm.NewClient(k8sConfig, m.logger)
	if err != nil {
		return err
	}
	defer hClient.Close()

	options := []k8sHelm.InstallOption{
		k8sHelm.ReleaseName(depInfo.ReleaseName),
		k8sHelm.InstallDryRun(dryRun),
		k8sHelm.InstallWait(false),
		k8sHelm.ValueOverrides(values),
	}
	installOptions := append(helm.DefaultInstallOptions, options...)

	release, err := hClient.InstallReleaseFromChart(
		requestedChart,
		depInfo.Namespace,
		installOptions...,
	)
	if err != nil {
		return fmt.Errorf("error deploying chart: %v", err)
	}

	m.logger.Infof("installing cluster group deployment %s on %s succeeded: %s", release.Release.Name, apiCluster.GetName(), release.String())
	return nil
}

func (m CGDeploymentManager) upgradeDeploymentOnCluster(apiCluster api.Cluster, orgName string, env helm_env.EnvSettings, depInfo *DeploymentInfo, requestedChart *chart.Chart, dryRun bool) error {
	m.logger.Infof("upgrade cluster group deployment %s on %s", depInfo.ReleaseName, apiCluster.GetName())
	k8sConfig, err := apiCluster.GetK8sConfig()
	if err != nil {
		return err
	}

	values, err := depInfo.GetValuesForCluster(apiCluster.GetName())
	if err != nil {
		return err
	}

	hClient, err := pkgHelm.NewClient(k8sConfig, m.logger)
	if err != nil {
		return err
	}
	defer hClient.Close()

	upgradeRes, err := hClient.UpdateReleaseFromChart(
		depInfo.ReleaseName,
		requestedChart,
		k8sHelm.UpdateValueOverrides(values),
		k8sHelm.UpgradeDryRun(dryRun),
		//helm.ResetValues(u.resetValues),
		k8sHelm.ReuseValues(false),
	)
	if err != nil {
		return fmt.Errorf("error deploying chart: %v", err)
	}

	m.logger.Infof("upgrading cluster group deployment %s on %s succeeded: %s", depInfo.ReleaseName, apiCluster.GetName(), upgradeRes.String())
	return nil
}

func (m CGDeploymentManager) upgradeOrInstallDeploymentOnCluster(apiCluster api.Cluster, orgName string, env helm_env.EnvSettings, depInfo *DeploymentInfo, requestedChart *chart.Chart, dryRun bool) error {
	status := m.getClusterDeploymentStatus(apiCluster, depInfo.ReleaseName, depInfo)
	if status.Status == NotInstalledStatus {
		err := m.installDeploymentOnCluster(apiCluster, orgName, env, depInfo, requestedChart, dryRun)
		if err != nil {
			return err
		}
	}

	if status.Stale {
		err := m.upgradeDeploymentOnCluster(apiCluster, orgName, env, depInfo, requestedChart, dryRun)
		if err != nil {
			return err
		}
	} else {
		m.logger.Infof("nothing to do deployment %s on %s is up to date", depInfo.ReleaseName, apiCluster.GetName())
	}

	return nil
}

func (m CGDeploymentManager) findRelease(apiCluster api.Cluster, name string) (*hapi_release5.Release, error) {
	k8sConfig, err := apiCluster.GetK8sConfig()
	if err != nil {
		return nil, err
	}

	//TODO we may consider using getReleaseContent instead
	deployments, err := helm.ListDeployments(&name, "", k8sConfig)
	if err != nil {
		m.logger.Errorf("ListDeployments for '%s' failed due to: %s", name, err.Error())
		return nil, err
	}
	for _, release := range deployments.GetReleases() {
		if release.Name == name {
			return release, nil
		}
	}
	return nil, nil
}

func (m CGDeploymentManager) getClusterDeploymentStatus(apiCluster api.Cluster, name string, depInfo *DeploymentInfo) TargetClusterStatus {
	m.logger.Debugf("get deployment status on %s", apiCluster.GetName())
	deploymentStatus := TargetClusterStatus{
		ClusterId:    apiCluster.GetID(),
		ClusterName:  apiCluster.GetName(),
		Cloud:        apiCluster.GetCloud(),
		Distribution: apiCluster.GetDistribution(),
	}
	release, err := m.findRelease(apiCluster, name)
	if err != nil {
		deploymentStatus.Status = fmt.Sprintf("failed to get status: %s", err.Error())
		return deploymentStatus
	}
	if release != nil {
		deploymentStatus.Version = release.Chart.Metadata.Version
		deploymentStatus.Stale = m.isStaleDeployment(release, depInfo, apiCluster)
		deploymentStatus.Status = release.Info.Status.Code.String()
		return deploymentStatus
	}

	deploymentStatus.Status = NotInstalledStatus
	deploymentStatus.Stale = true
	return deploymentStatus
}

func (m CGDeploymentManager) isStaleDeployment(release *hapi_release5.Release, depInfo *DeploymentInfo, apiCluster api.Cluster) bool {
	if release.Chart.Metadata.Name != depInfo.ChartName {
		return true
	}
	if release.Chart.Metadata.Version != depInfo.ChartVersion {
		return true
	}
	values, err := depInfo.GetValuesForCluster(apiCluster.GetName())
	if err != nil {
		return false
	}
	m.logger.Debugf("release values: \n%s \nuser values:\n%s ", release.Config.Raw, string(values))

	if len(release.Config.Raw) != len(string(values)) || release.Config.Raw != string(values) {
		return true
	}
	return false
}

func (m CGDeploymentManager) createDeploymentModel(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *ClusterGroupDeployment, requestedChart *chart.Chart) (*ClusterGroupDeploymentModel, error) {
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
	}
	values, err := json.Marshal(cgDeployment.Values)
	if err != nil {
		return nil, err
	}
	deploymentModel.Values = values
	deploymentModel.TargetClusters = make([]TargetCluster, 0)
	for _, cluster := range clusterGroup.Clusters {
		targetCluster := TargetCluster{
			ClusterID:   cluster.GetID(),
			ClusterName: cluster.GetName(),
		}
		if valuesOverride, ok := cgDeployment.ValueOverrides[cluster.GetName()]; ok {
			marshalledValues, err := json.Marshal(valuesOverride)
			if err != nil {
				return nil, err
			}
			targetCluster.Values = marshalledValues
		}
		deploymentModel.TargetClusters = append(deploymentModel.TargetClusters, targetCluster)
	}

	return deploymentModel, nil
}

func (m CGDeploymentManager) updateDeploymentModel(clusterGroup *api.ClusterGroup, deploymentModel *ClusterGroupDeploymentModel, cgDeployment *ClusterGroupDeployment, requestedChart *chart.Chart) error {
	deploymentModel.DeploymentVersion = cgDeployment.Version
	deploymentModel.Description = requestedChart.Metadata.Description
	deploymentModel.ChartName = requestedChart.Metadata.Name

	//TODO merge request values with persited ones in case reuse = true
	if cgDeployment.ReUseValues {
		return nil
	}

	values, err := json.Marshal(cgDeployment.Values)
	if err != nil {
		return err
	}
	deploymentModel.Values = values
	existingTargetsMap := make(map[uint]TargetCluster, 0)
	for _, target := range deploymentModel.TargetClusters {
		existingTargetsMap[target.ClusterID] = target
	}

	for _, cluster := range clusterGroup.Clusters {
		target, exists := existingTargetsMap[cluster.GetID()]
		if !exists {
			target = TargetCluster{
				ClusterID:   cluster.GetID(),
				ClusterName: cluster.GetName(),
			}
			deploymentModel.TargetClusters = append(deploymentModel.TargetClusters, target)
		}

		if valuesOverride, ok := cgDeployment.ValueOverrides[cluster.GetName()]; ok {
			marshalledValues, err := json.Marshal(valuesOverride)
			if err != nil {
				return err
			}
			target.Values = marshalledValues
		} else {
			target.Values = nil
		}

	}

	return nil
}

func (m CGDeploymentManager) getDeploymentFromModel(deploymentModel *ClusterGroupDeploymentModel) (*DeploymentInfo, error) {
	deployment := &DeploymentInfo{
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

	deployment.ValueOverrides = make(map[string]map[string]interface{}, 0)
	for _, valueOverrides := range deploymentModel.TargetClusters {
		if len(valueOverrides.Values) > 0 {
			var unmarshalledValues map[string]interface{}
			err = json.Unmarshal(valueOverrides.Values, &unmarshalledValues)
			if err != nil {
				return nil, err
			}
			deployment.ValueOverrides[valueOverrides.ClusterName] = unmarshalledValues
		}
	}
	return deployment, nil
}

func (m CGDeploymentManager) GetDeployment(clusterGroup *api.ClusterGroup, deploymentName string) (*DeploymentInfo, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		return nil, err
	}
	depInfo, err := m.getDeploymentFromModel(deploymentModel)
	if err != nil {
		return nil, err
	}

	// get deployment status for each cluster group member
	targetClusterStatus := make([]TargetClusterStatus, 0)

	deploymentCount := 0
	statusChan := make(chan TargetClusterStatus)
	defer close(statusChan)

	for _, apiCluster := range clusterGroup.Clusters {
		deploymentCount++
		go func(apiCluster api.Cluster, name string) {
			statusChan <- m.getClusterDeploymentStatus(apiCluster, name, depInfo)
		}(apiCluster, deploymentName)
	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	targetClusterStatus = append(targetClusterStatus, m.addStaleClusterStatuses(clusterGroup.Clusters, deploymentModel.TargetClusters)...)

	depInfo.TargetClusters = targetClusterStatus
	return depInfo, nil
}

// returns stale clusters, cluster not members of the cluster group anymore. they may have been already deleted
func (m CGDeploymentManager) addStaleClusterStatuses(clusters map[uint]api.Cluster, overrides []TargetCluster) []TargetClusterStatus {
	staleClusterStatuses := make([]TargetClusterStatus, 0)
	for _, o := range overrides {
		if _, exists := clusters[o.ClusterID]; !exists {

			ctx := context.Background()
			cluster, err := m.clusterGetter.GetClusterByIDOnly(ctx, o.ClusterID)
			status := StaleStatus
			if err != nil {
				status += ", cluster not found"
			}
			deploymentStatus := TargetClusterStatus{
				ClusterId:   o.ClusterID,
				ClusterName: o.ClusterName,
				Status:      status,
			}
			staleClusterStatuses = append(staleClusterStatuses, deploymentStatus)
			if cluster != nil {
				deploymentStatus.Cloud = cluster.GetCloud()
				deploymentStatus.Distribution = cluster.GetDistribution()
			}
		}
	}
	return staleClusterStatuses
}

func (m CGDeploymentManager) GenerateReleaseName(clusterGroup *api.ClusterGroup) string {
	moniker := moniker.New()
	name := moniker.NameSep("-")
	if len(name) > releaseNameMaxLen {
		name = name[:releaseNameMaxLen]
	}
	return name
}

func (m CGDeploymentManager) GetAllDeployments(clusterGroup *api.ClusterGroup) ([]*ListDeploymentResponse, error) {

	deploymentModels, err := m.repository.FindAll(clusterGroup.Id)
	if err != nil {
		return nil, err
	}
	resultList := make([]*ListDeploymentResponse, 0)
	for _, deploymentModel := range deploymentModels {
		deployment := &ListDeploymentResponse{
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

func (m CGDeploymentManager) deleteDeploymentFromCluster(clusterId uint, apiCluster api.Cluster, name string) error {
	if apiCluster == nil {
		m.logger.Warnf("cluster %v is not member of the cluster group anymore", clusterId)
	}

	ctx := context.Background()
	cluster, err := m.clusterGetter.GetClusterByIDOnly(ctx, clusterId)
	if err != nil {
		return errors.WithStack(&memberClusterNotFoundError{
			clusterID: clusterId,
		})
	}
	apiCluster = cluster

	m.logger.Infof("delete deployment from %s", apiCluster.GetName())
	k8sConfig, err := apiCluster.GetK8sConfig()
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

// DeleteDeployment deletes deployments from target clusters
func (m CGDeploymentManager) DeleteDeployment(clusterGroup *api.ClusterGroup, deploymentName string, forceDelete bool) ([]TargetClusterStatus, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		return nil, err
	}

	targetClustersStatus, err := m.deleteDeploymentFromTargetClusters(clusterGroup, deploymentName, deploymentModel, true, forceDelete)
	if err != nil {
		return nil, err
	}

	return targetClustersStatus, nil
}

// SyncDeployment deletes deployments from target clusters not belonging to the group anymore, installs or upgrades to member clusters
func (m CGDeploymentManager) SyncDeployment(clusterGroup *api.ClusterGroup, orgName string, deploymentName string) ([]TargetClusterStatus, error) {

	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, deploymentName)
	if err != nil {
		return nil, err
	}

	depInfo, err := m.getDeploymentFromModel(deploymentModel)
	if err != nil {
		return nil, err
	}

	// get deployment status for each cluster group member
	response := make([]TargetClusterStatus, 0)

	env := helm.GenerateHelmRepoEnv(orgName)
	requestedChart, err := helm.GetRequestedChart(depInfo.ReleaseName, depInfo.ChartName, depInfo.ChartVersion, deploymentModel.DeploymentPackage, env)
	if err != nil {
		return nil, fmt.Errorf("error loading chart: %v", err)
	}
	targetClustersStatus := m.upgradeOrInstallDeploymentToTargetClusters(clusterGroup, orgName, env, depInfo, requestedChart, false)
	response = append(response, targetClustersStatus...)

	targetClustersStatus, err = m.deleteDeploymentFromTargetClusters(clusterGroup, deploymentName, deploymentModel, false, false)
	if err != nil {
		return nil, err
	}
	response = append(response, targetClustersStatus...)

	return response, nil
}

// deleteDeploymentFromTargetClusters deletes deployments from targeted clusters
// if deleteAll = true deployments from all targeted clusters are deleted,
// otherwise only stale deployments from targets not belonging to the cluster group anymore
func (m CGDeploymentManager) deleteDeploymentFromTargetClusters(clusterGroup *api.ClusterGroup, deploymentName string, deploymentModel *ClusterGroupDeploymentModel, deleteAll bool, forceDelete bool) ([]TargetClusterStatus, error) {

	// get deployment status for each cluster group member
	targetClustersStatus := make([]TargetClusterStatus, 0)

	deploymentCount := 0
	statusChan := make(chan TargetClusterStatus)
	defer close(statusChan)

	for _, clusterOverride := range deploymentModel.TargetClusters {
		deploymentCount++
		apiCluster, exists := clusterGroup.Clusters[clusterOverride.ClusterID]
		// delete if deleteAll or in case target doesn't belongs to the cluster group anymore
		if deleteAll || !exists {
			go func(clusterID uint, apiCluster api.Cluster, name string) {
				clErr := m.deleteDeploymentFromCluster(clusterID, apiCluster, name)
				status := DeletedStatus
				// if cluster is not found anymore then is fine
				if _, ok := IsMemberClusterNotFoundError(clErr); clErr != nil && !ok {
					errMsg := fmt.Sprintf("failed to delete cluster group deployment from cluster: %s", clErr.Error())
					m.logger.Warn(errMsg)
					if !forceDelete {
						status = errMsg
					}
				}
				depStatus := TargetClusterStatus{
					ClusterId: clusterID,
					Status:    status,
				}
				if apiCluster != nil {
					depStatus.ClusterName = apiCluster.GetName()
				}
				statusChan <- depStatus
			}(clusterOverride.ClusterID, apiCluster, deploymentName)
		}

	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClustersStatus = append(targetClustersStatus, status)
	}

	err := m.repository.Delete(deploymentModel, targetClustersStatus)
	if err != nil {
		return nil, err
	}

	return targetClustersStatus, nil
}

func (m CGDeploymentManager) upgradeOrInstallDeploymentToTargetClusters(clusterGroup *api.ClusterGroup, orgName string, env helm_env.EnvSettings, depInfo *DeploymentInfo, requestedChart *chart.Chart, dryRun bool) []TargetClusterStatus {
	targetClusterStatus := make([]TargetClusterStatus, 0)
	deploymentCount := 0
	statusChan := make(chan TargetClusterStatus)
	defer close(statusChan)

	// upgrade & install deployments
	for _, apiCluster := range clusterGroup.Clusters {
		deploymentCount++
		go func(apiCluster api.Cluster) {
			clerr := m.upgradeOrInstallDeploymentOnCluster(apiCluster, orgName, env, depInfo, requestedChart, dryRun)
			status := SucceededStatus
			if clerr != nil {
				status = fmt.Sprintf("%s: %s", FailedStatus, clerr.Error())
			}
			statusChan <- TargetClusterStatus{
				ClusterId:   apiCluster.GetID(),
				ClusterName: apiCluster.GetName(),
				Status:      status,
			}
		}(apiCluster)

	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	return targetClusterStatus
}

func (m CGDeploymentManager) CreateDeployment(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *ClusterGroupDeployment) ([]TargetClusterStatus, error) {

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

	depInfo, err := m.getDeploymentFromModel(deploymentModel)
	if err != nil {
		return nil, err
	}

	targetClusterStatus := m.upgradeOrInstallDeploymentToTargetClusters(clusterGroup, orgName, env, depInfo, requestedChart, cgDeployment.DryRun)
	return targetClusterStatus, nil
}

// UpdateDeployment upgrades deployment using provided values or using already provided values if ReUseValues = true.
// The deployment is installed on a member cluster in case it's was not installed previously.
func (m CGDeploymentManager) UpdateDeployment(clusterGroup *api.ClusterGroup, orgName string, cgDeployment *ClusterGroupDeployment) ([]TargetClusterStatus, error) {

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
	deploymentModel, err := m.repository.FindByName(clusterGroup.Id, cgDeployment.ReleaseName)
	if err != nil {
		return nil, err
	}

	// if reUseValues = false update values / valueOverrides from request
	err = m.updateDeploymentModel(clusterGroup, deploymentModel, cgDeployment, requestedChart)
	if err != nil {
		return nil, emperror.Wrap(err, "Error updating deployment model")
	}
	if !cgDeployment.DryRun {
		err = m.repository.Save(deploymentModel)
		if err != nil {
			return nil, emperror.Wrap(err, "Error saving deployment model")
		}
	}

	depInfo, err := m.getDeploymentFromModel(deploymentModel)
	if err != nil {
		return nil, err
	}

	targetClusterStatus := m.upgradeOrInstallDeploymentToTargetClusters(clusterGroup, orgName, env, depInfo, requestedChart, cgDeployment.DryRun)
	return targetClusterStatus, nil
}
