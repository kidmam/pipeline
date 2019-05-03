package deployment

import (
	"context"
	"net/http"
	"strings"

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/helm"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/gin-gonic/gin"
)

func (n *API) Create(c *gin.Context) {
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
