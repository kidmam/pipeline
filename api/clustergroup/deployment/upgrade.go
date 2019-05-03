package deployment

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (n *API) Upgrade(c *gin.Context) {
	c.Status(http.StatusAccepted)
	return
}
