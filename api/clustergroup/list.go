package clustergroup

import (
	"context"
	"net/http"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/gin-gonic/gin"
)

func (a *API) List(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	clusterGroups, err := a.clusterGroupManager.GetAllClusterGroups(ctx)
	if err != nil {
		a.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, clusterGroups)
}
