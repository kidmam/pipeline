package clustergroup

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
)

func (a *API) Get(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	cgId, ok := ginutils.UintParam(c, "id")
	if !ok {
		return
	}

	response, err := a.clusterGroupManager.GetClusterGroupByIdWithStatus(ctx, cgId, true)
	if err != nil {
		a.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}
