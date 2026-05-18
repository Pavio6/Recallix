package document

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"recallix/internal/auth"
	"recallix/internal/config"
	"recallix/internal/document/parser"
	"recallix/internal/repository"
	"recallix/internal/shared"
	"recallix/internal/storage"
	"recallix/internal/task"
)

type Handler struct {
	db         *gorm.DB
	cfg        *config.Config
	storage    *storage.FileStorage
	taskClient *asynq.Client
}

func NewHandler(db *gorm.DB, cfg *config.Config, store *storage.FileStorage, taskClient *asynq.Client) *Handler {
	return &Handler{db: db, cfg: cfg, storage: store, taskClient: taskClient}
}

func (h *Handler) Upload(c *gin.Context) {
	userID := auth.GetUserID(c)
	kbID := c.Param("id")

	var kb repository.KnowledgeBase
	if err := h.db.Where("id = ? AND user_id = ?", kbID, userID).First(&kb).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Knowledge base not found"},
		})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: "File is required"},
		})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if !isAllowedExt(ext) {
		c.JSON(http.StatusBadRequest, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "VALIDATION", Message: fmt.Sprintf("Unsupported file type: %s. Allowed: .md, .txt, .docx", ext)},
		})
		return
	}

	hash := sha256.New()
	fileContent, err := io.ReadAll(io.TeeReader(file, hash))
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to read file"},
		})
		return
	}

	fileHash := fmt.Sprintf("%x", hash.Sum(nil))

	var duplicateCount int64
	if err := h.db.Model(&repository.Knowledge{}).
		Where("file_hash = ? AND user_id = ?", fileHash, userID).
		Count(&duplicateCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to check duplicate file"},
		})
		return
	}
	if duplicateCount > 0 {
		c.JSON(http.StatusConflict, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "CONFLICT", Message: "Duplicate file, already uploaded"},
		})
		return
	}

	knowledgeID := shared.NewID()
	objectKey := filepath.ToSlash(filepath.Join(userID, kbID, knowledgeID+ext))
	filePath, err := h.storage.Save(c.Request.Context(), objectKey, header.Header.Get("Content-Type"),
		int64(len(fileContent)), bytes.NewReader(fileContent))
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to save file: " + err.Error()},
		})
		return
	}

	knowledge := repository.Knowledge{
		ID:              knowledgeID,
		UserID:          userID,
		KnowledgeBaseID: kbID,
		FileName:        header.Filename,
		FilePath:        filePath,
		FileHash:        fileHash,
		FileSize:        header.Size,
		ParseStatus:     "pending",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.db.Create(&knowledge).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to create knowledge record"},
		})
		return
	}

	// Enqueue async document processing
	docTask, err := task.NewDocumentProcessTask(knowledge.ID)
	if err != nil {
		log.Printf("[Upload] Failed to create task: %v", err)
	} else if _, err := h.taskClient.Enqueue(docTask); err != nil {
		log.Printf("[Upload] Failed to enqueue task: %v", err)
	}

	c.JSON(http.StatusCreated, shared.APIResponse{Success: true, Data: knowledge})
}

func (h *Handler) Content(c *gin.Context) {
	userID := auth.GetUserID(c)
	id := c.Param("id")

	var k repository.Knowledge
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&k).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Knowledge not found"},
		})
		return
	}

	reader, err := h.storage.Open(c.Request.Context(), k.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to read file"},
		})
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to read file"},
		})
		return
	}

	ext := filepath.Ext(k.FileName)
	contentType := "text/plain"
	switch ext {
	case ".md":
		contentType = "text/markdown; charset=utf-8"
	case ".txt":
		contentType = "text/plain; charset=utf-8"
	case ".docx":
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}

	c.Data(http.StatusOK, contentType, data)
}

func (h *Handler) GetContent(c *gin.Context) {
	userID := auth.GetUserID(c)
	id := c.Param("id")

	var k repository.Knowledge
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&k).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Knowledge not found"},
		})
		return
	}

	reader, err := h.storage.Open(c.Request.Context(), k.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to read file"},
		})
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to read file"},
		})
		return
	}

	text := string(data)
	if strings.ToLower(filepath.Ext(k.FileName)) == ".docx" {
		if md, err := parser.ParseBytes(".docx", data); err == nil {
			text = md.Text
		}
	}

	c.JSON(http.StatusOK, shared.APIResponse{
		Success: true,
		Data: gin.H{
			"id":        k.ID,
			"file_name": k.FileName,
			"content":   text,
		},
	})
}

func (h *Handler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	kbID := c.Query("knowledge_base_id")
	query := h.db.Where("user_id = ?", userID)
	if kbID != "" {
		query = query.Where("knowledge_base_id = ?", kbID)
	}
	var knowledges []repository.Knowledge
	if err := query.Order("created_at desc").Find(&knowledges).Error; err != nil {
		c.JSON(http.StatusInternalServerError, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "INTERNAL_ERROR", Message: "Failed to list knowledges"},
		})
		return
	}
	if knowledges == nil {
		knowledges = []repository.Knowledge{}
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: knowledges})
}

func (h *Handler) Get(c *gin.Context) {
	userID := auth.GetUserID(c)
	id := c.Param("id")
	var k repository.Knowledge
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&k).Error; err != nil {
		c.JSON(http.StatusNotFound, shared.APIResponse{
			Success: false,
			Error:   &shared.APIError{Code: "NOT_FOUND", Message: "Knowledge not found"},
		})
		return
	}
	c.JSON(http.StatusOK, shared.APIResponse{Success: true, Data: k})
}

func isAllowedExt(ext string) bool {
	switch ext {
	case ".md", ".txt", ".docx":
		return true
	}
	return false
}
