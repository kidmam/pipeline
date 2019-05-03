package deployment

import (
	"context"
	"net/http"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/gin-gonic/gin"
)

func (n *API) Get(c *gin.Context) {
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
