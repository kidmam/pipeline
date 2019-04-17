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

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/cluster"
	cgroup "github.com/banzaicloud/pipeline/internal/clustergroup"
	"github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
	//ctx := ginutils.Context(context.Background(), c)
	orgID := auth.GetCurrentOrganization(c.Request).ID
	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	cg, err := n.clusterGroupManager.FindOne(cgroup.ClusterGroupModel{
		OrganizationID: orgID,
		ID:             clusterGroupId,
	})
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	err = n.clusterGroupManager.DeleteClusterGroup(cg)
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

func getLeavingMembers(existingClusterGroup *clustergroup.ClusterGroup, requestedMembers []string) map[string]cluster.CommonCluster {
    leavingMembers := make(map[string]cluster.CommonCluster, 0)
    for clusterName, cluster := range existingClusterGroup.MemberClusters {
    	leavingMembers[clusterName] = cluster
	}
	for _, clusterName := range requestedMembers {
		delete(leavingMembers, clusterName)
	}
    return leavingMembers
}


func (n *ClusterGroupAPI) SetupClusterGroup(c *gin.Context, update bool) {
	ctx := ginutils.Context(context.Background(), c)

	var req clustergroup.ClusterGroupRequest
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

	var existingClusterGroup *clustergroup.ClusterGroup
	if !update && err == nil {
		existingClusterGroup := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)
		if existingClusterGroup != nil {
			c.JSON(http.StatusBadRequest, pkgCommon.ErrorResponse{
				Code:    http.StatusBadRequest,
				Message: "Cluster group already exists with this name",
				Error:   err.Error(),
			})
			return
		}
	}

	memberClusterModels := make([]cgroup.MemberClusterModel, 0)
	joiningMembers := make(map[string]cluster.CommonCluster, 0)
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
		}

		if update && !existingClusterGroup.IsMember(clusterName) {
			joiningMembers[clusterName] = cluster
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

	clusterGroupModel := cgroup.ClusterGroupModel{
		Name:           req.Name,
		OrganizationID: orgId,
		Members:        memberClusterModels,
	}

	err = n.db.Save(&clusterGroupModel).Error
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	//call feature handlers on members update
	if update {
		enabledFeatures, err := n.clusterGroupManager.GetEnabledFeatureHandlers(*existingClusterGroup)
		if err != nil {
			errorHandler.Handle(err)
			ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
			return
		}

		if len(joiningMembers) > 0 {
			for _, member := range joiningMembers {
				for _, feature := range enabledFeatures {
					err = feature.JoinCluster(member)
				}
			}
		}

		leavingMembers := getLeavingMembers(existingClusterGroup, req.Members)
		if len(leavingMembers) > 0 {
			for _, member := range leavingMembers {
				for _, feature := range enabledFeatures {
					err = feature.LeaveCluster(member)
				}
			}
		}
	}

}

func (n *ClusterGroupAPI) CreateClusterGroup(c *gin.Context) {
	n.SetupClusterGroup(c, false)
}

func (n *ClusterGroupAPI) UpdateClusterGroup(c *gin.Context) {
	n.SetupClusterGroup(c, true)
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
	response.Properties = feature.Params

	//TODO call feature handler to get statuses

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
	cg := n.clusterGroupManager.GetClusterGroupFromModel(ctx, cgModel, false)

	featureName := c.Param("featureName")
	feature, err := n.clusterGroupManager.GetFeature(*cg, featureName)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}
	err = n.clusterGroupManager.SetFeatureParams(*feature, req.Enabled, req.Properties)
	if err != nil {
		errorHandler.Handle(err)
		ginutils.ReplyWithErrorResponse(c, errorResponseFrom(err))
		return
	}

	c.JSON(http.StatusOK, "")
}
