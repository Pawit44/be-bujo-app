package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"bujo/internal/middleware"
	"bujo/internal/service"
)

type FolderController struct {
	folders *service.FolderService
}

func NewFolderController(folders *service.FolderService) *FolderController {
	return &FolderController{folders: folders}
}

type folderRequest struct {
	Title    string `json:"title"`
	Position *int   `json:"position"`
}

func (r folderRequest) toInput() service.FolderInput {
	return service.FolderInput{Title: r.Title, Position: r.Position}
}

// List GET /api/collections/:id/folders
func (ctrl *FolderController) List(c *gin.Context) {
	collectionID, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	folders, err := ctrl.folders.List(collectionID, uid)
	if err != nil {
		internalError(c, "folders.List", err)
		return
	}
	c.JSON(http.StatusOK, folders)
}

// Create POST /api/collections/:id/folders
func (ctrl *FolderController) Create(c *gin.Context) {
	collectionID, ok := parseID(c)
	if !ok {
		return
	}
	var req folderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	folder, err := ctrl.folders.Create(collectionID, uid, req.toInput())
	if err != nil {
		respondFolderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// Update PATCH /api/folders/:id — rename or reorder.
func (ctrl *FolderController) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req folderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	folder, err := ctrl.folders.Update(id, uid, req.toInput())
	if err != nil {
		respondFolderError(c, err)
		return
	}
	c.JSON(http.StatusOK, folder)
}

// Delete DELETE /api/folders/:id — its entries move back to the
// collection's unsorted area, they are not deleted with it.
func (ctrl *FolderController) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	if err := ctrl.folders.Delete(id, uid); err != nil {
		respondFolderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func respondFolderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrFolderNotFound), errors.Is(err, service.ErrCollectionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
