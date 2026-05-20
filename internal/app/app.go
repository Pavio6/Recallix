package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	agentcfg "recallix/internal/agent"
	"recallix/internal/auth"
	"recallix/internal/chat/service"
	"recallix/internal/chat/session"
	"recallix/internal/config"
	"recallix/internal/document"
	"recallix/internal/document/processor"
	"recallix/internal/knowledge"
	"recallix/internal/memory"
	"recallix/internal/model/llm"
	"recallix/internal/repository"
	"recallix/internal/retrieval/hybrid"
	"recallix/internal/retrieval/vectorstore"
	"recallix/internal/storage"
	"recallix/internal/task"
)

type App struct {
	Config     *config.Config
	DB         *gorm.DB
	Router     *gin.Engine
	TaskClient *asynq.Client
	TaskServer *asynq.Server
	VectorStore vectorstore.VectorStore
}

func New() (*App, error) {
	cfg := config.Load()

	gin.SetMode(cfg.GinMode)

	db, err := repository.NewDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	if err := repository.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("migration: %w", err)
	}

	vs, err := vectorstore.NewPGStore(db, cfg.EmbeddingDimension)
	if err != nil {
		log.Printf("WARNING: Failed to initialize pgvector store: %v (vector search will be unavailable)", err)
		vs = nil
	}

	taskClient := task.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	taskServer := task.NewServer(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.WorkerConcurrency)

	store, err := storage.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	chatClient := llm.NewChatClient(cfg)
	embedClient := llm.NewEmbeddingClient(cfg)
	rerankClient := llm.NewRerankClient(cfg)

	authHandler := auth.NewHandler(db, cfg)
	kbHandler := knowledge.NewHandler(db)
	docHandler := document.NewHandler(db, cfg, store, taskClient)
	sessionHandler := session.NewHandler(db)

	memService := memory.NewService(db, chatClient, embedClient, vs)
	agentService := agentcfg.NewService(db, cfg, store)
	agentHandler := agentcfg.NewHandler(db, agentService)

	var hybridService *hybrid.Service
	if vs != nil {
		hybridService = hybrid.NewService(db, embedClient, vs)
	}

	chatService := service.New(db, cfg, chatClient, embedClient, rerankClient, hybridService, memService, taskClient, agentService)

	docProc := processor.NewDocProcessor(db, embedClient, vs, store)

	mux := asynq.NewServeMux()
	mux.HandleFunc(task.TypeDocumentProcess, func(ctx context.Context, t *asynq.Task) error {
		var payload task.DocumentProcessPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		log.Printf("[Worker] Processing document: %s", payload.KnowledgeID)
		return docProc.HandleDocumentProcess(t)
	})
	mux.HandleFunc(task.TypeMemoryExtract, func(ctx context.Context, t *asynq.Task) error {
		var payload task.MemoryExtractPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		log.Printf("[Worker] Extracting memory for session: %s", payload.SessionID)
		return memService.ProcessExtraction(payload.UserID, payload.Question, payload.Answer)
	})

	go func() {
		log.Printf("[Worker] Starting async task worker (concurrency: %d)", cfg.WorkerConcurrency)
		if err := taskServer.Run(mux); err != nil {
			log.Printf("[Worker] Worker stopped: %v", err)
		}
	}()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "http://localhost"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		AllowCredentials: true,
	}))

	api := r.Group("/api/v1")
	{
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)

		protected := api.Group("")
		protected.Use(authHandler.Service().Middleware())
		{
			protected.GET("/auth/me", authHandler.Me)

			protected.GET("/knowledge-bases", kbHandler.List)
			protected.POST("/knowledge-bases", kbHandler.Create)
			protected.GET("/knowledge-bases/:id", kbHandler.Get)

			protected.GET("/knowledges", docHandler.List)
			protected.GET("/knowledges/:id", docHandler.Get)
			protected.GET("/knowledges/:id/content", docHandler.GetContent)
			protected.GET("/knowledges/:id/file", docHandler.Content)
			protected.POST("/knowledge-bases/:id/files", docHandler.Upload)

			protected.POST("/sessions", sessionHandler.Create)
			protected.GET("/sessions/recent", sessionHandler.ListRecent)
			protected.GET("/sessions/:id", sessionHandler.Get)
			protected.GET("/sessions/:id/messages", sessionHandler.GetMessages)
			protected.PUT("/sessions/:id", sessionHandler.Update)
			protected.GET("/agents", agentHandler.ListAgents)
			protected.POST("/agents", agentHandler.CreateAgent)
			protected.GET("/agents/:id", agentHandler.GetAgent)
			protected.PUT("/agents/:id", agentHandler.UpdateAgent)
			protected.GET("/skills", agentHandler.ListSkills)
			protected.POST("/skills/import", agentHandler.ImportSkill)
			protected.DELETE("/skills/:id", agentHandler.DeleteSkill)

			if chatService != nil {
				protected.POST("/sessions/:id/chat", chatService.Chat)
			}

			protected.GET("/memories", func(c *gin.Context) {
				uid := auth.GetUserID(c)
				memories, err := memService.List(uid)
				if err != nil {
					c.JSON(500, gin.H{"success": false})
					return
				}
				c.JSON(200, gin.H{"success": true, "data": memories})
			})
			protected.DELETE("/memories/:id", func(c *gin.Context) {
				uid := auth.GetUserID(c)
				id := c.Param("id")
				if err := memService.Delete(uid, id); err != nil {
					c.JSON(500, gin.H{"success": false})
					return
				}
				c.JSON(200, gin.H{"success": true})
			})
		}
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "recallix"})
	})

	return &App{
		Config:     cfg,
		DB:         db,
		Router:     r,
		TaskClient: taskClient,
		TaskServer: taskServer,
		VectorStore: vs,
	}, nil
}

func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.Config.ServerPort)
	log.Printf("Recallix server starting on %s", addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := a.Router.Run(addr); err != nil {
			log.Printf("HTTP server stopped: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")

	a.TaskServer.Shutdown()
	return nil
}
