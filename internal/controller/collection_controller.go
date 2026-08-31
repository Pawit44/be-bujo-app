package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"bujo/internal/middleware"
	"bujo/internal/service"
)

type CollectionController struct {
	collections *service.CollectionService
}

func NewCollectionController(collections *service.CollectionService) *CollectionController {
	return &CollectionController{collections: collections}
}

type collectionRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	Icon        string  `json:"icon"`
	Pinned      *bool   `json:"pinned"`
	Position    *int    `json:"position"`
}

func (r collectionRequest) toInput() service.CollectionInput {
	return service.CollectionInput{
		Title:       r.Title,
		Description: r.Description,
		Color:       r.Color,
		Icon:        r.Icon,
		Pinned:      r.Pinned,
		Position:    r.Position,
	}
}

// List GET /api/collections
func (ctrl *CollectionController) List(c *gin.Context) {
	uid := middleware.CurrentUser(c).ID
	views, err := ctrl.collections.List(uid)
	if err != nil {
		internalError(c, "collections.List", err)
		return
	}
	c.JSON(http.StatusOK, views)
}

// Get GET /api/collections/:id
func (ctrl *CollectionController) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	col, err := ctrl.collections.Get(id, uid)
	if err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, col)
}

// Create POST /api/collections
func (ctrl *CollectionController) Create(c *gin.Context) {
	var req collectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	col, err := ctrl.collections.Create(uid, req.toInput())
	if err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, col)
}

// Update PATCH /api/collections/:id
func (ctrl *CollectionController) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req collectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	col, err := ctrl.collections.Update(id, uid, req.toInput())
	if err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, col)
}

// Delete DELETE /api/collections/:id — removes the collection and every
// entry that lived on it.
func (ctrl *CollectionController) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	if err := ctrl.collections.Delete(id, uid); err != nil {
		respondCollectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func respondCollectionError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrCollectionNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
