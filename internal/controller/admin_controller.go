package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"bujo/internal/middleware"
	"bujo/internal/models"
	"bujo/internal/service"
)

type AdminController struct {
	admin *service.AdminService
}

func NewAdminController(admin *service.AdminService) *AdminController {
	return &AdminController{admin: admin}
}

// ListUsers GET /api/admin/users
func (ctrl *AdminController) ListUsers(c *gin.Context) {
	users, err := ctrl.admin.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

type updateRoleRequest struct {
	Role models.Role `json:"role"`
}

// UpdateRole PATCH /api/admin/users/:id — promote or demote an account.
func (ctrl *AdminController) UpdateRole(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, err := ctrl.admin.UpdateRole(id, req.Role)
	if err != nil {
		respondAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// DeleteUser DELETE /api/admin/users/:id
func (ctrl *AdminController) DeleteUser(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	actor := middleware.CurrentUser(c)
	if err := ctrl.admin.DeleteUser(actor.ID, id); err != nil {
		respondAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func respondAdminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidRole):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrCannotDemoteLast),
		errors.Is(err, service.ErrCannotDeleteLast),
		errors.Is(err, service.ErrCannotDeleteSelf):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
