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

package clustergroup

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/banzaicloud/pipeline/auth"
	cgroupIAPI "github.com/banzaicloud/pipeline/internal/clustergroup/api"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
)

func (n *API) Create(c *gin.Context) {
	ctx := ginutils.Context(context.Background(), c)

	var req cgroupIAPI.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		n.errorHandler.Handle(c, c.Error(err).SetType(gin.ErrorTypeBind))
		return
	}

	orgId := auth.GetCurrentOrganization(c.Request).ID
	id, err := n.clusterGroupManager.CreateClusterGroup(ctx, req.Name, orgId, req.Members)
	if err != nil {
		n.errorHandler.Handle(c, err)
		return
	}

	c.JSON(http.StatusCreated, cgroupIAPI.CreateResponse{
		Name:       req.Name,
		ResourceID: *id,
	})
}