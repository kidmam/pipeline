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
	"net/http"
	"strings"

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/helm"
	cgroup "github.com/banzaicloud/pipeline/internal/clustergroup"
	"github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/ghodss/yaml"
	"github.com/gin-gonic/gin"
	"github.com/goph/emperror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ClusterGroupAPI implements the Cluster Group Management API actions.
type ClusterGroupAPI struct {
	clusterGroupManager *cgroup.Manager
	deploymentManager   *cgroup.CGDeploymentManager
	logger              logrus.FieldLogger
	errorHandler        clusterGroupAPIErrorHandler
}

// NewClusterGroupAPI returns a new ClusterGroupAPI instance.
func NewClusterGroupAPI(
	clusterGroupManager *cgroup.Manager,
	deploymentManager *cgroup.CGDeploymentManager,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *ClusterGroupAPI {
	return &ClusterGroupAPI{
		clusterGroupManager: clusterGroupManager,
		deploymentManager:   deploymentManager,
		logger:              logger,
		errorHandler: clusterGroupAPIErrorHandler{
			handler: errorHandler,
		},
	}
}

type clusterGroupAPIErrorHandler struct {
	handler emperror.Handler
}

func (e clusterGroupAPIErrorHandler) Handle(c *gin.Context, err error) {
	ginutils.ReplyWithErrorResponse(c, e.errorResponseFrom(err))

	e.handler.Handle(err)
}

// errorResponseFrom translates the given error into a components.ErrorResponse
func (e clusterGroupAPIErrorHandler) errorResponseFrom(err error) *pkgCommon.ErrorResponse {
	if cgroup.IsClusterGroupNotFoundError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusNotFound,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsClusterGroupAlreadyExistsError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsMemberClusterNotFoundError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsNoReadyMembersError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsClusterGroupHasEnabledFeaturesError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	return errorResponseFrom(err)
}

func (n *ClusterGroupAPI) GetClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)
	cgId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	response, err := n.clusterGroupManager.GetClusterGroupByIdWithStatus(ctx, cgId, true)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (n *ClusterGroupAPI) GetAllClusterGroups(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroups, err := n.clusterGroupManager.GetAllClusterGroups(ctx)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, clusterGroups)
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

	id, err := n.clusterGroupManager.CreateClusterGroup(ctx, req.Name, orgId, req.Members)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusCreated, clustergroup.ClusterGroupCreateResponse{
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

	orgID := auth.GetCurrentOrganization(c.Request).ID

	err := n.clusterGroupManager.UpdateClusterGroup(ctx, orgID, clusterGroupId, req.Name, req.Members)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusAccepted, "")
}

func (n *ClusterGroupAPI) DeleteClusterGroup(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	err := n.clusterGroupManager.DeleteClusterGroup(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, "")
}

func (n *ClusterGroupAPI) GetFeature(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	//orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}
	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	featureName := c.Param("featureName")
	feature, err := n.clusterGroupManager.GetFeature(*clusterGroup, featureName)
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

	//orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	featureName := c.Param("featureName")

	err = n.clusterGroupManager.SetFeatureParams(featureName, clusterGroup, req.Enabled, req.Properties)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.ReconcileFeatureHandlers(*clusterGroup)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}

func (n *ClusterGroupAPI) parseDeploymentRequest(c *gin.Context, clusterGroup *clustergroup.ClusterGroup) (*clustergroup.ClusterGroupDeployment, error) {
	organization, err := auth.GetOrganizationById(clusterGroup.OrganizationID)
	if err != nil {
		return nil, errors.Wrap(err, "Error during getting organization. ")
	}
	var deployment *clustergroup.CreateUpdateDeploymentRequest
	err = c.BindJSON(&deployment)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing request:")
	}
	n.logger.Debugf("Parsing chart %s with version %s and release name %s", deployment.Name, deployment.Version, deployment.ReleaseName)

	request := clustergroup.ClusterGroupDeployment{
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
	n.logger.Debug("Custom values: ", string(request.Values))
	return &request, nil
}

// CreateDeployment creates a Helm deployment
func (n *ClusterGroupAPI) CreateDeployment(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	//orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	cgDeployment, err := n.parseDeploymentRequest(c, clusterGroup)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	if len(strings.TrimSpace(cgDeployment.DeploymentReleaseName)) == 0 {
		cgDeployment.DeploymentReleaseName, _ = helm.GenerateName("")
	}

	targetClusterStatus := n.deploymentManager.CreateDeployment(clusterGroup, cgDeployment)

	n.logger.Debug("Release name: ", cgDeployment.DeploymentReleaseName)
	response := clustergroup.CreateUpdateDeploymentResponse{
		ReleaseName:    cgDeployment.DeploymentReleaseName,
		TargetClusters: targetClusterStatus,
	}
	c.JSON(http.StatusCreated, response)
	return
}

// GetDeployment returns the details of a helm deployment
func (n *ClusterGroupAPI) GetDeployment(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	name := c.Param("name")
	n.logger.Infof("getting details for cluster group deployment: [%s]", name)

	//orgId := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	targetClusterStatus := n.deploymentManager.GetDeployment(clusterGroup, name)
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
