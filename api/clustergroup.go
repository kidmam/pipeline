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

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/helm"
	cgroup "github.com/banzaicloud/pipeline/internal/clustergroup"
	"github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/ghodss/yaml"
	"github.com/gin-gonic/gin"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	k8sHelm "k8s.io/helm/pkg/helm"
)

type ClusterGetter interface {
	GetClusterByName(ctx context.Context, organizationID uint, clusterName string) (cluster.CommonCluster, error)
}

// ClusterGroupAPI implements the Cluster Group Management API actions.
type ClusterGroupAPI struct {
	clusterGetter       ClusterGetter
	clusterGroupManager *cgroup.Manager
	db                  *gorm.DB
	logger              logrus.FieldLogger
	errorHandler        emperror.Handler
}

// NewClusterGroupAPI returns a new ClusterGroupAPI instance.
func NewClusterGroupAPI(
	clusterGetter ClusterGetter,
	clusterGroupManager *cgroup.Manager,
	db *gorm.DB,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *ClusterGroupAPI {
	return &ClusterGroupAPI{
		clusterGetter:       clusterGetter,
		clusterGroupManager: clusterGroupManager,
		db:                  db,
		logger:              logger,
		errorHandler:        errorHandler,
	}
}

func (n *ClusterGroupAPI) GetClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)
	orgID := auth.GetCurrentOrganization(c.Request).ID
	cgId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	cg, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		OrganizationID: orgID,
		ID:             cgId,
	})
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	response := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cg, true)
	c.JSON(http.StatusOK, response)
}

func (n *ClusterGroupAPI) DeleteClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	cgModel, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		ID:             clusterGroupId,
	})
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	cgroup := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)
	enabledFeatures, err := n.clusterGroupManager.GetEnabledFeatures(*cgroup)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}
	if len(enabledFeatures) > 0 {
		if err != nil {
			featureNames := reflect.ValueOf(enabledFeatures).MapKeys()
			msg := fmt.Sprintf("cluster group: %s, has following features enabled: %s, you have to disable features, before deleting the cluster group.", cgroup.Name, featureNames)
			c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
				Code:    http.StatusBadRequest,
				Message: msg,
				Error:   msg,
			})
			return
		}
	}

	err = n.clusterGroupManager.DeleteClusterGroup(cgModel)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}

func (n *ClusterGroupAPI) GetAllClusterGroups(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)
	response := make([]clustergroup.ClusterGroup, 0)

	clusterGroups, err := n.clusterGroupManager.FindAll()
	if err != nil {
		errorHandler.Handle(err)

		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
	}
	for _, cgModel := range clusterGroups {
		cg := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)
		response = append(response, *cg)
	}

	c.JSON(http.StatusOK, response)
}

func (n *ClusterGroupAPI) CreateClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	var req clustergroup.ClusterGroupCreateUpdateRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Error parsing request",
			Error:   err.Error(),
		})
		return
	}

	orgId := auth.GetCurrentOrganization(c.Request).ID

	cgModel, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		OrganizationID: orgId,
		Name:           req.Name,
	})
	if err != nil {
		if !gorm.IsRecordNotFoundError(err) {
			errorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
			return
		}
	}
	if cgModel != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Cluster group already exists with this name",
			Error:   "Cluster group already exists with this name",
		})
		return
	}

	memberClusterModels := make([]cgroup.MemberClusterModel, 0)
	for _, clusterName := range req.Members {
		cluster, err := n.clusterGetter.GetClusterByName(ctx, orgId, clusterName)
		if err != nil {
			err = errors.Wrapf(err, "%s not found", clusterName)
			errorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
			return
		}
		clusterIsReady, err := cluster.IsReady()
		if err == nil && clusterIsReady {
			log.Infof(clusterName)
			memberClusterModels = append(memberClusterModels, cgroup.MemberClusterModel{
				ClusterID: cluster.GetID(),
			})
			log.Infof("Join cluster %s to group: %s", clusterName, req.Name)
		}

	}
	if len(memberClusterModels) == 0 {
		err := errors.New("No ready cluster members found.")
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "No ready cluster members found.",
			Error:   err.Error(),
		})
		return
	}

	id, err := n.clusterGroupManager.CreateClusterGroup(req.Name, orgId, memberClusterModels)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	c.JSON(http.StatusAccepted, clustergroup.ClusterGroupCreateResponse{
		Name:       req.Name,
		ResourceID: *id,
	})
}

func (n *ClusterGroupAPI) UpdateClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	var req clustergroup.ClusterGroupCreateUpdateRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Error parsing request",
			Error:   err.Error(),
		})
		return
	}

	orgId := auth.GetCurrentOrganization(c.Request).ID

	cgModel, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		ID: clusterGroupId,
	})

	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	existingClusterGroup := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)
	newMembers := make(map[uint]cluster.CommonCluster, 0)

	for _, clusterName := range req.Members {
		cluster, err := n.clusterGetter.GetClusterByName(ctx, orgId, clusterName)
		if err != nil {
			err = errors.Wrapf(err, "%s not found", clusterName)
			errorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
			return
		}
		clusterIsReady, err := cluster.IsReady()
		if err == nil && clusterIsReady {
			log.Infof("Join cluster %s to group: %s", clusterName, existingClusterGroup.Name)
			newMembers[cluster.GetID()] = cluster
		} else {
			log.Infof("Can't join cluster %s to group: %s as it not ready!", clusterName, existingClusterGroup.Name)
		}
	}

	existingClusterGroup, err = n.clusterGroupManager.UpdateClusterGroup(ctx, existingClusterGroup, req.Name, newMembers)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	// call feature handlers on members update
	err = n.clusterGroupManager.ReconcileFeatureHandlers(*existingClusterGroup)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}
}

func (n *ClusterGroupAPI) GetFeature(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}
	cgModel, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		OrganizationID: orgId,
		ID:             clusterGroupId,
	})
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}
	cg := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)

	featureName := c.Param("featureName")
	feature, err := n.clusterGroupManager.GetFeature(*cg, featureName)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	var response clustergroup.ClusterGroupFeatureResponse
	response.Enabled = feature.Enabled
	response.Properties = feature.Properties

	//call feature handler to get statuses
	if feature.Enabled {
		status, err := n.clusterGroupManager.GetFeatureStatus(*feature)
		if err != nil {
			errorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
			return
		}
		response.Status = status
	}

	c.JSON(http.StatusOK, response)
}

func (n *ClusterGroupAPI) SetFeature(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	var req clustergroup.ClusterGroupFeatureRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Error parsing request",
			Error:   err.Error(),
		})
		return
	}

	orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}
	cgModel, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		OrganizationID: orgId,
		ID:             clusterGroupId,
	})
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}
	if cgModel == nil {
		msg := fmt.Sprintf("cluster group with id: %v not found", clusterGroupId)
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: msg,
			Error:   msg,
		})
		return
	}
	cg := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)
	featureName := c.Param("featureName")

	err = n.clusterGroupManager.SetFeatureParams(featureName, cg, req.Enabled, req.Properties)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.ReconcileFeatureHandlers(*cg)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}

type ClusterGroupDeployment struct {
	ClusterGroupId        uint
	DeploymentName        string
	DeploymentVersion     string
	DeploymentPackage     []byte
	DeploymentReleaseName string
	ReuseValues           bool
	Namespace             string
	Values                []byte
	ValueOverrides        map[string][]byte
	OrganizationName      string
	DryRun                bool
	Wait                  bool
	Timeout               int64
}

func parseDeploymentRequest(c *gin.Context, clusterGroup *clustergroup.ClusterGroup) (*ClusterGroupDeployment, error) {
	organization, err := auth.GetOrganizationById(clusterGroup.OrganizationID)
	if err != nil {
		return nil, errors.Wrap(err, "Error during getting organization. ")
	}
	var deployment *clustergroup.CreateUpdateDeploymentRequest
	err = c.BindJSON(&deployment)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing request:")
	}
	log.Debugf("Parsing chart %s with version %s and release name %s", deployment.Name, deployment.Version, deployment.ReleaseName)

	request := ClusterGroupDeployment{
		OrganizationName:      organization.Name,
		DeploymentName:        deployment.Name,
		DeploymentVersion:     deployment.Version,
		DeploymentPackage:     deployment.Package,
		DeploymentReleaseName: deployment.ReleaseName,
		ReuseValues:           deployment.ReUseValues,
		Namespace:             deployment.Namespace,
		DryRun:                deployment.DryRun,
		Wait:                  deployment.Wait,
		Timeout:               deployment.Timeout,
		ValueOverrides:        make(map[string][]byte),
	}

	if deployment.Values != nil {
		request.Values, err = yaml.Marshal(deployment.Values)
		if err != nil {
			return nil, errors.Wrap(err, "Can't parse Values:")
		}
	}
	for clusterName, valueOverrides := range deployment.ValueOverrides {
		yaml, err := yaml.Marshal(valueOverrides)
		if err != nil {
			return nil, errors.Wrapf(err, "Can't parse Values for cluster %s:", clusterName)
		}
		request.ValueOverrides[clusterName] = yaml
	}
	log.Debug("Custom values: ", string(request.Values))
	return &request, nil
}

func installDeploymentOnCluster(commonCluster cluster.CommonCluster, cgDeployment *ClusterGroupDeployment) error {
	log.Infof("Installing deployment on %s", commonCluster.GetName())
	k8sConfig, err := commonCluster.GetK8sConfig()
	if err != nil {
		return err
	}

	convertedValues := cgDeployment.Values
	clusterSpecificOverrides, exists := cgDeployment.ValueOverrides[commonCluster.GetName()]
	// merge values with overrides for cluster if any
	if exists {
		values := make(map[string]interface{})
		err := yaml.Unmarshal(cgDeployment.Values, &values)
		if err != nil {
			return err
		}
		overrideValues := make(map[string]interface{})
		err = yaml.Unmarshal(clusterSpecificOverrides, &overrideValues)
		if err != nil {
			return err
		}
		values = helm.MergeValues(values, overrideValues)
		convertedValues, err = yaml.Marshal(values)
		if err != nil {
			return err
		}
	}

	installOptions := []k8sHelm.InstallOption{
		k8sHelm.InstallWait(cgDeployment.Wait),
		k8sHelm.ValueOverrides(convertedValues),
	}

	if cgDeployment.Timeout > 0 {
		installOptions = append(installOptions, k8sHelm.InstallTimeout(cgDeployment.Timeout))
	}

	release, err := helm.CreateDeployment(
		cgDeployment.DeploymentName,
		cgDeployment.DeploymentVersion,
		cgDeployment.DeploymentPackage,
		cgDeployment.Namespace,
		cgDeployment.DeploymentReleaseName,
		cgDeployment.DryRun,
		nil,
		k8sConfig,
		helm.GenerateHelmRepoEnv(cgDeployment.OrganizationName),
		installOptions...,
	)
	if err != nil {
		//TODO distinguish error codes
		return err
	}
	log.Infof("Installing deployment on %s succeeded: %s", commonCluster.GetName(), release.String())
	return nil
}

// CreateDeployment creates a Helm deployment
func (n *ClusterGroupAPI) CreateDeployment(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, orgId, clusterGroupId)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	cgDeployment, err := parseDeploymentRequest(c, clusterGroup)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	if len(strings.TrimSpace(cgDeployment.DeploymentReleaseName)) == 0 {
		cgDeployment.DeploymentReleaseName, _ = helm.GenerateName("")
	}

	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.MemberClusters {
		deploymentCount++
		go func(commonCluster cluster.CommonCluster, cgDeployment *ClusterGroupDeployment) {
			clerr := installDeploymentOnCluster(commonCluster, cgDeployment)
			status := "SUCCEEDED"
			if clerr == nil {
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

	log.Debug("Release name: ", cgDeployment.DeploymentReleaseName)
	response := clustergroup.CreateUpdateDeploymentResponse{
		ReleaseName:    cgDeployment.DeploymentReleaseName,
		TargetClusters: targetClusterStatus,
	}
	c.JSON(http.StatusCreated, response)
	return
}

func getClusterDeploymentStatus(commonCluster cluster.CommonCluster, name string) (string, error) {
	log.Infof("Installing deployment on %s", commonCluster.GetName())
	k8sConfig, err := commonCluster.GetK8sConfig()
	if err != nil {
		return "", err
	}

	deployments, err := helm.ListDeployments(&name, "", k8sConfig)
	if err != nil {
		log.Errorf("ListDeployments for '%s' failed due to: %s", name, err.Error())
		return "", err
	}
	for _, release := range deployments.GetReleases() {
		if release.Name == name {
			return release.Info.Status.Code.String(), nil
		}
	}
	return "unknown", nil
}

// GetDeployment returns the details of a helm deployment
func (n *ClusterGroupAPI) GetDeployment(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	name := c.Param("name")
	log.Infof("getting details for cluster group deployment: [%s]", name)

	orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, orgId, clusterGroupId)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	targetClusterStatus := make([]clustergroup.DeploymentStatus, 0)
	deploymentCount := 0
	statusChan := make(chan clustergroup.DeploymentStatus)
	defer close(statusChan)

	for _, commonCluster := range clusterGroup.MemberClusters {
		deploymentCount++
		go func(commonCluster cluster.CommonCluster, name string) {
			status, clErr := getClusterDeploymentStatus(commonCluster, name)
			if clErr != nil {
				status = fmt.Sprintf("Failed to get status: %s", clErr.Error())
			}
			statusChan <- clustergroup.DeploymentStatus{
				ClusterId:   commonCluster.GetID(),
				ClusterName: commonCluster.GetName(),
				Status:      status,
			}
		}(commonCluster, name)

	}

	// wait for goroutines to finish
	for i := 0; i < deploymentCount; i++ {
		status := <-statusChan
		targetClusterStatus = append(targetClusterStatus, status)
	}

	response := clustergroup.GetDeploymentResponse{
		ReleaseName:    name,
		TargetClusters: targetClusterStatus,
	}

	c.JSON(http.StatusOK, response)

}

// ListDeployments
func (n *ClusterGroupAPI) ListDeployments(c *gin.Context) {
	c.JSON(http.StatusCreated, "")
	return
}

// DeleteDeployment
func (n *ClusterGroupAPI) DeleteDeployment(c *gin.Context) {
	c.JSON(http.StatusCreated, "")
	return
}

// UpdateDeployment
func (n *ClusterGroupAPI) UpgradeDeployment(c *gin.Context) {
	c.JSON(http.StatusCreated, "")
	return
}
