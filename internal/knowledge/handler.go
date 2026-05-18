package knowledge

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"recallix/internal/auth"
	"recallix/internal/repository"
	"recallix/internal/shared"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Create(c *gin.Context) {
	userID := auth.GetUserID(c)
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: err.Error()},
		})
		return
	}
	kb := repository.KnowledgeBase{
		ID:          shared.NewID(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := h.db.Create(&kb).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to create knowledge base"},
		})
		return
	}
	c.JSON(http.StatusCreated, shared.APIResponse{Success: true, Data: kb})
}

func (h *Handler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	var kbs []repository.KnowledgeBase
	if err := h.db.Where("user_id = ?", userID).Order("created_at desc").Find(&kbs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to list knowledge bases"},
		})
		return
	}
	if kbs == nil {
		kbs = []repository.KnowledgeBase{}
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: kbs})
}

func (h *Handler) Get(c *gin.Context) {
	userID := auth.GetUserID(c)
	id := c.Param("id")
	var kb repository.KnowledgeBase
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&kb).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Knowledge base not found"},
		})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: kb})
}
