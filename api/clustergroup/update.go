package clustergroup

import (
	"context"
	"net/http"

	"github.com/banzaicloud/pipeline/auth"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
	"github.com/gin-gonic/gin"
)

func (n *API) Update(c *gin.Context) {
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

	c.Status(http.StatusAccepted)
}
