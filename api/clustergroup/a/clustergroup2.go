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

package a

import (
	"context"
	"net/http"
	"strings"

	"github.com/banzaicloud/pipeline/api"
	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/config"
	"github.com/banzaicloud/pipeline/helm"
	cgroup "github.com/banzaicloud/pipeline/internal/clustergroup"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/goph/emperror"
	"github.com/sirupsen/logrus"
)

var ErrorHandler emperror.Handler

func init() {
	ErrorHandler = config.ErrorHandler()
}

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

// api.ErrorResponseFrom translates the given error into a components.ErrorResponse
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

	return api.ErrorResponseFrom(err)
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

func (n *ClusterGroupAPI) GetFeatures(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}
	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	features, err := n.clusterGroupManager.GetFeatures(*clusterGroup)
	if err != nil {
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, features)
}

func (n *ClusterGroupAPI) GetFeature(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

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
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	var response clustergroup.ClusterGroupFeatureResponse
	response.Enabled = feature.Enabled
	response.Properties = feature.Properties

	//call feature handler to get statuses
	if feature.Enabled {
		status, err := n.clusterGroupManager.GetFeatureStatus(*feature)
		if err != nil {
			ErrorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
			return
		}
		response.Status = status
	}

	c.JSON(http.StatusOK, response)
}

func (n *ClusterGroupAPI) DisableFeature(c *gin.Context) {
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
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.ReconcileFeatures(*clusterGroup, false)
	if err != nil {
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}

func (n *ClusterGroupAPI) UpdateFeature(c *gin.Context) {
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
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.ReconcileFeatures(*clusterGroup, false)
	if err != nil {
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}

func (n *ClusterGroupAPI) EnableFeature(c *gin.Context) {
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
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.ReconcileFeatures(*clusterGroup, false))
	if err != nil {
		ErrorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, api.ErrorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
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

	organization, err := auth.GetOrganizationById(clusterGroup.OrganizationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Error  getting organization",
			Error:   err.Error(),
		})
		return
	}
	var deployment *clustergroup.ClusterGroupDeployment
	err = c.BindJSON(&deployment)
	if err != nil {
		c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Message: "Error parsing request",
			Error:   err.Error(),
		})
		return
	}

	if len(strings.TrimSpace(deployment.ReleaseName)) == 0 {
		deployment.ReleaseName, _ = helm.GenerateName("")
	}

	targetClusterStatus, err := n.deploymentManager.CreateDeployment(clusterGroup, organization.Name, deployment)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	n.logger.Debug("Release name: ", deployment.ReleaseName)
	response := clustergroup.CreateUpdateDeploymentResponse{
		ReleaseName:    deployment.ReleaseName,
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

	response, err := n.deploymentManager.GetDeployment(clusterGroup, name)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// ListDeployments
func (n *ClusterGroupAPI) ListDeployments(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := n.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	response, err := n.deploymentManager.GetAllDeployments(clusterGroup)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// DeleteDeployment
func (n *ClusterGroupAPI) DeleteDeployment(c *gin.Context) {
	c.JSON(http.StatusAccepted, "")
	return
}

// UpdateDeployment
func (n *ClusterGroupAPI) UpgradeDeployment(c *gin.Context) {
	c.JSON(http.StatusAccepted, "")
	return
}
