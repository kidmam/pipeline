// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
