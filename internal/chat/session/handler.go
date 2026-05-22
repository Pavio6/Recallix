package session

import (
	"encoding/json"
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
		Title   string  `json:"title"`
		Mode    string  `json:"mode"`
		AgentID *string `json:"agent_id"`
	}
	c.ShouldBindJSON(&req)
	if req.Title == "" {
		req.Title = "新对话"
	}

	session := repository.Session{
		ID:        shared.NewID(),
		UserID:    userID,
		Title:     req.Title,
		Mode:      normalizeMode(req.Mode),
		AgentID:   req.AgentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.db.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to create session"},
		})
		return
	}

	c.JSON(http.StatusCreated, shared.APIResponse{Success: true, Data: session})
}

func (h *Handler) Update(c *gin.Context) {
	userID := auth.GetUserID(c)
	sessionID := c.Param("id")

	var session repository.Session
	if err := h.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Session not found"},
		})
		return
	}

	var req struct {
		Mode    string  `json:"mode"`
		AgentID *string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{Success: false})
		return
	}
	session.Mode = normalizeMode(req.Mode)
	if session.Mode == "agent_reasoning" {
		session.AgentID = req.AgentID
	} else {
		session.AgentID = nil
	}
	session.UpdatedAt = time.Now()
	if err := h.db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{Success: false})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: session})
}

func (h *Handler) ListRecent(c *gin.Context) {
	userID := auth.GetUserID(c)
	var sessions []repository.Session

	cutoff := time.Now().AddDate(0, 0, -7)
	err := h.db.Where("user_id = ? AND updated_at >= ?", userID, cutoff).
		Order("updated_at desc").Limit(50).Find(&sessions).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to list sessions"},
		})
		return
	}

	if sessions == nil {
		sessions = []repository.Session{}
	}

	type sessionWithCount struct {
		repository.Session
		MessageCount int64 `json:"message_count"`
	}

	result := make([]sessionWithCount, len(sessions))
	for i, s := range sessions {
		var count int64
		h.db.Model(&repository.Message{}).Where("session_id = ?", s.ID).Count(&count)
		result[i] = sessionWithCount{Session: s, MessageCount: count}
	}

	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: result})
}

func normalizeMode(mode string) string {
	if mode == "agent_reasoning" {
		return mode
	}
	return "quick_answer"
}

func (h *Handler) GetMessages(c *gin.Context) {
	userID := auth.GetUserID(c)
	sessionID := c.Param("id")

	var session repository.Session
	if err := h.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Session not found"},
		})
		return
	}

	var messages []repository.Message
	h.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages)

	if messages == nil {
		messages = []repository.Message{}
	}

	messageIndex := make(map[string]int, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for i, message := range messages {
		messageIndex[message.ID] = i
		if message.Role == "assistant" {
			messageIDs = append(messageIDs, message.ID)
		}
	}
	if len(messageIDs) > 0 {
		var references []repository.MessageReference
		h.db.Where("message_id IN ?", messageIDs).
			Order("message_id asc, rank asc").
			Find(&references)
		for _, ref := range references {
			if idx, ok := messageIndex[ref.MessageID]; ok {
				messages[idx].References = append(messages[idx].References, ref)
			}
		}

		var traces []repository.MessageSkillTrace
		h.db.Where("message_id IN ?", messageIDs).Find(&traces)
		for i := range traces {
			trace := traces[i]
			if idx, ok := messageIndex[trace.MessageID]; ok {
				messages[idx].SkillTrace = &trace
				var selected []repository.MessageSkillSummary
				if err := json.Unmarshal([]byte(trace.SelectedSkillsJSON), &selected); err == nil {
					messages[idx].UsedSkills = selected
				}
			}
		}
	}

	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: messages})
}

func (h *Handler) Get(c *gin.Context) {
	userID := auth.GetUserID(c)
	sessionID := c.Param("id")

	var session repository.Session
	if err := h.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Session not found"},
		})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: session})
}
