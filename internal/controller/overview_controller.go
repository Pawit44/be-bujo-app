package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"bujo/internal/middleware"
	"bujo/internal/service"
)

type OverviewController struct {
	overview *service.OverviewService
}

func NewOverviewController(overview *service.OverviewService) *OverviewController {
	return &OverviewController{overview: overview}
}

// Index GET /api/index — everything the INDEX page needs in one round trip.
func (ctrl *OverviewController) Index(c *gin.Context) {
	uid := middleware.CurrentUser(c).ID
	view, err := ctrl.overview.Index(uid)
	if err != nil {
		internalError(c, "overview.Index", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// Stats GET /api/stats?month=YYYY-MM — completion figures for one month.
func (ctrl *OverviewController) Stats(c *gin.Context) {
	uid := middleware.CurrentUser(c).ID
	month := c.Query("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	view, err := ctrl.overview.Stats(uid, month)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}
