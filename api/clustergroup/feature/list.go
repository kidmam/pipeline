package feature

import (
	"context"
	"net/http"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/gin-gonic/gin"
)

func (a *API) List(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroupId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	clusterGroup, err := a.clusterGroupManager.GetClusterGroupById(ctx, clusterGroupId)
	if err != nil {
		a.errorHandler.Handle(c, err)
		return
	}

	features, err := a.clusterGroupManager.GetFeatures(*clusterGroup)
	if err != nil {
		a.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, features)
}
