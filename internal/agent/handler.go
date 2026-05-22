package agent

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"recallix/internal/auth"
	"recallix/internal/shared"
)

type Handler struct {
	db  *gorm.DB
	svc *Service
}

func NewHandler(db *gorm.DB, svc *Service) *Handler {
	return &Handler{db: db, svc: svc}
}

func (h *Handler) ListAgents(c *gin.Context) {
	items, err := h.svc.List(auth.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: items})
}

func (h *Handler) GetAgent(c *gin.Context) {
	item, err := h.svc.Get(auth.GetUserID(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: item})
}

func (h *Handler) CreateAgent(c *gin.Context) {
	var req UpsertAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{Success: false})
		return
	}
	item, err := h.svc.Create(auth.GetUserID(c), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusCreated, shared.APIResponse{Success: true, Data: item})
}

func (h *Handler) UpdateAgent(c *gin.Context) {
	var req UpsertAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{Success: false})
		return
	}
	item, err := h.svc.Update(auth.GetUserID(c), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: item})
}

func (h *Handler) ListSkills(c *gin.Context) {
	items, err := h.svc.ListSkills(auth.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: items})
}

func (h *Handler) ImportSkill(c *gin.Context) {
	var req struct {
		GitHubURL string `json:"github_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.GitHubURL) == "" {
		c.JSON(http.StatusBadRequest, shared.APIResponse{Success: false})
		return
	}
	item, err := h.svc.ImportSkill(c.Request.Context(), auth.GetUserID(c), strings.TrimSpace(req.GitHubURL))
	if err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error: &shared.APIError{
				Code:    "SKILL_IMPORT_FAILED",
				Message: err.Error(),
			},
		})
		return
	}
	c.JSON(http.StatusCreated, shared.APIResponse{Success: true, Data: item})
}

func (h *Handler) DeleteSkill(c *gin.Context) {
	if err := h.svc.DeleteSkill(c.Request.Context(), auth.GetUserID(c), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error: &shared.APIError{
				Code:    "SKILL_DELETE_FAILED",
				Message: err.Error(),
			},
		})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true})
}
