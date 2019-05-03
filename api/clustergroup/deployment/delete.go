package deployment

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (n *API) Delete(c *gin.Context) {
	c.Status(http.StatusAccepted)
	return
}
