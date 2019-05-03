package feature

import (
	"context"
	"net/http"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/gin-gonic/gin"
)

func (n *API) Get(c *gin.Context) {
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
		n.errorHandler.Handle(c, err)
		return
	}

	var response clustergroup.ClusterGroupFeatureResponse
	response.Enabled = feature.Enabled
	response.Properties = feature.Properties

	//call feature handler to get statuses
	if feature.Enabled {
		status, err := n.clusterGroupManager.GetFeatureStatus(*feature)
		if err != nil {
			n.errorHandler.Handle(c, err)
			return
		}
		response.Status = status
	}

	c.JSON(http.StatusOK, response)
}
