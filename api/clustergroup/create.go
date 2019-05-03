package clustergroup

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/banzaicloud/pipeline/auth"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
)

func (n *API) Create(c *gin.Context) {
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
