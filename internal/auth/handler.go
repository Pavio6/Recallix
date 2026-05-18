package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"recallix/internal/config"
	"recallix/internal/repository"
	"recallix/internal/shared"
)

type Handler struct {
	db  *gorm.DB
	cfg *config.Config
	svc *Service
}

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{
		db:  db,
		cfg: cfg,
		svc: NewService(cfg),
	}
}

func (h *Handler) Service() *Service { return h.svc }

type registerReq struct {
	Email    string `json:"email" binding:"required,email"`
	Nickname string `json:"nickname" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: err.Error()},
		})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Nickname = strings.TrimSpace(req.Nickname)
	if req.Nickname == "" {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: "Nickname is required"},
		})
		return
	}

	var existing repository.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "CONFLICT", Message: "Email already registered"},
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to process password"},
		})
		return
	}

	user := repository.User{
		ID:           shared.NewID(),
		Email:        req.Email,
		PasswordHash: string(hash),
		Nickname:     req.Nickname,
		Status:       1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to create user"},
		})
		return
	}

	c.JSON(http.StatusCreated, shared.APIResponse{
		Success: true,
		Data:    gin.H{"id": user.ID, "email": user.Email, "nickname": user.Nickname},
	})
}

type loginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: err.Error()},
		})
		return
	}

	var user repository.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "UNAUTHORIZED", Message: "Invalid email or password"},
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "UNAUTHORIZED", Message: "Invalid email or password"},
		})
		return
	}

	tokens, err := h.svc.GenerateTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to generate tokens"},
		})
		return
	}

	hashedRefresh, _ := bcrypt.GenerateFromPassword([]byte(tokens.RefreshToken), bcrypt.DefaultCost)
	refreshRecord := repository.RefreshToken{
		ID:        shared.NewID(),
		UserID:    user.ID,
		TokenHash: string(hashedRefresh),
		ExpiresAt: time.Now().Add(h.cfg.JWTRefreshExpire),
		CreatedAt: time.Now(),
	}
	h.db.Create(&refreshRecord)

	c.JSON(http.StatusOK, shared.APIResponse{
		Success: true,
		Data: gin.H{
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"user": gin.H{
				"id":       user.ID,
				"email":    user.Email,
				"nickname": user.Nickname,
			},
		},
	})
}

func (h *Handler) Me(c *gin.Context) {
	userID := GetUserID(c)
	var user repository.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "User not found"},
		})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{
		Success: true,
		Data:    gin.H{"id": user.ID, "email": user.Email, "nickname": user.Nickname},
	})
}
