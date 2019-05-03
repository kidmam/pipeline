package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goph/emperror"

	"github.com/banzaicloud/pipeline/api"
	cgroup "github.com/banzaicloud/pipeline/internal/clustergroup"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	pkgCommon "github.com/banzaicloud/pipeline/pkg/common"
)

type ErrorHandler struct {
	Handler emperror.Handler
}

func (e ErrorHandler) Handle(c *gin.Context, err error) {
	ginutils.ReplyWithErrorResponse(c, e.errorResponseFrom(err))

	e.Handler.Handle(err)
}

// errorResponseFrom translates the given error into a components.ErrorResponse
func (e ErrorHandler) errorResponseFrom(err error) *pkgCommon.ErrorResponse {
	if cgroup.IsClusterGroupNotFoundError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusNotFound,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsClusterGroupAlreadyExistsError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsMemberClusterNotFoundError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsNoReadyMembersError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	if cgroup.IsClusterGroupHasEnabledFeaturesError(err) {
		return &pkgCommon.ErrorResponse{
			Code:    http.StatusBadRequest,
			Error:   err.Error(),
			Message: err.Error(),
		}
	}

	return api.ErrorResponseFrom(err)
}
