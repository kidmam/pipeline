package clustergroup

import (
	"context"
	"net/http"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/gin-gonic/gin"
)

func (n *API) Delete(c *gin.Context) {
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

	c.Status(http.StatusOK)
}
