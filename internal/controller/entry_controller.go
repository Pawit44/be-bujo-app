package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"bujo/internal/middleware"
	"bujo/internal/models"
	"bujo/internal/repository"
	"bujo/internal/service"
)

type EntryController struct {
	entries *service.EntryService
}

func NewEntryController(entries *service.EntryService) *EntryController {
	return &EntryController{entries: entries}
}

type entryRequest struct {
	Content      string             `json:"content"`
	Type         models.EntryType   `json:"type"`
	Status       models.EntryStatus `json:"status"`
	LogKind      models.LogKind     `json:"logKind"`
	Month        string             `json:"month"`
	Date         string             `json:"date"`
	CollectionID *uint              `json:"collectionId"`
	Priority     *bool              `json:"priority"`
	Inspiration  *bool              `json:"inspiration"`
	Position     *int               `json:"position"`
	Notes        *string            `json:"notes"`
}

func (r entryRequest) toInput() service.EntryInput {
	return service.EntryInput{
		Content:      r.Content,
		Type:         r.Type,
		Status:       r.Status,
		LogKind:      r.LogKind,
		Month:        r.Month,
		Date:         r.Date,
		CollectionID: r.CollectionID,
		Priority:     r.Priority,
		Inspiration:  r.Inspiration,
		Position:     r.Position,
		Notes:        r.Notes,
	}
}

// List GET /api/entries?logKind=&month=&date=&from=&to=&collectionId=&status=&type=&q=
func (ctrl *EntryController) List(c *gin.Context) {
	uid := middleware.CurrentUser(c).ID
	filters := repository.EntryFilters{
		LogKind:      models.LogKind(c.Query("logKind")),
		Month:        c.Query("month"),
		Date:         c.Query("date"),
		DateFrom:     c.Query("from"),
		DateTo:       c.Query("to"),
		CollectionID: c.Query("collectionId"),
		Status:       c.Query("status"),
		Type:         c.Query("type"),
		Search:       c.Query("q"),
	}
	entries, err := ctrl.entries.List(uid, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

// ListDue GET /api/entries/review — every open entry from a spread that has
// already passed: the raw material for the BuJo "migration" ritual (decide,
// for each: done, move it forward, or drop it).
func (ctrl *EntryController) ListDue(c *gin.Context) {
	uid := middleware.CurrentUser(c).ID
	entries, err := ctrl.entries.ListDue(uid, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

// Get GET /api/entries/:id
func (ctrl *EntryController) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	entry, err := ctrl.entries.Get(id, uid)
	if err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

// Create POST /api/entries
func (ctrl *EntryController) Create(c *gin.Context) {
	var req entryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	entry, err := ctrl.entries.Create(uid, req.toInput())
	if err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusCreated, entry)
}

// Update PATCH /api/entries/:id
func (ctrl *EntryController) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req entryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	entry, err := ctrl.entries.Update(id, uid, req.toInput())
	if err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

// Toggle POST /api/entries/:id/toggle — flips a task between open and done.
func (ctrl *EntryController) Toggle(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	entry, err := ctrl.entries.Toggle(id, uid)
	if err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

type migrateRequest struct {
	LogKind      models.LogKind `json:"logKind"`
	Month        string         `json:"month"`
	Date         string         `json:"date"`
	CollectionID *uint          `json:"collectionId"`
}

// Migrate POST /api/entries/:id/migrate
func (ctrl *EntryController) Migrate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req migrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	source, migrated, err := ctrl.entries.Migrate(id, uid, service.MigrateInput{
		LogKind:      req.LogKind,
		Month:        req.Month,
		Date:         req.Date,
		CollectionID: req.CollectionID,
	})
	if err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": source, "migrated": migrated})
}

type reorderRequest struct {
	IDs []uint `json:"ids"`
}

// Reorder POST /api/entries/reorder — persists a drag-and-drop ordering.
func (ctrl *EntryController) Reorder(c *gin.Context) {
	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c).ID
	if err := ctrl.entries.Reorder(uid, req.IDs); err != nil {
		if errors.Is(err, service.ErrEntriesNotOwned) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete DELETE /api/entries/:id
func (ctrl *EntryController) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c).ID
	if err := ctrl.entries.Delete(id, uid); err != nil {
		respondEntryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

// respondEntryError maps the small set of sentinel errors EntryService can
// return to their HTTP status; anything else (validation messages, quota
// text) is a plain 400 shown to the user as-is.
func respondEntryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEntryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
	case errors.Is(err, service.ErrCollectionNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrAlreadyMigrated), errors.Is(err, service.ErrSameLocation):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrContentRequired), errors.Is(err, service.ErrLogKindRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
