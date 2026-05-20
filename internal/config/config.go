package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort string
	GinMode    string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTSecret        string
	JWTAccessExpire  time.Duration
	JWTRefreshExpire time.Duration

	ChatModel             string
	AgentModel            string
	ChatModelBaseURL      string
	ChatModelAPIKey       string
	EmbeddingModel        string
	EmbeddingModelBaseURL string
	EmbeddingModelAPIKey  string
	EmbeddingDimension    int
	RerankModel           string
	RerankModelBaseURL    string
	RerankModelAPIKey     string

	RAGScoreThreshold     float64
	RAGFallbackMinScore   float64
	RAGThresholdFloor     float64
	RAGThresholdDegradeBy float64
	RAGTopK               int

	MinIOEndpoint        string
	MinIOAccessKeyID     string
	MinIOSecretAccessKey string
	MinIOBucketName      string
	MinIOUseSSL          bool

	WorkerConcurrency int

	SkillSandboxMode    string
	SkillSandboxTimeout time.Duration
	SkillSandboxImage   string
}

func Load() *Config {
	_ = godotenv.Load()
	cfg := &Config{
		ServerPort: getEnv("SERVER_PORT", "8081"),
		GinMode:    getEnv("GIN_MODE", "debug"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "recallix"),
		DBPassword: getEnv("DB_PASSWORD", "recallix"),
		DBName:     getEnv("DB_NAME", "recallix"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       0,

		JWTSecret:        getEnv("JWT_SECRET", "change-me"),
		JWTAccessExpire:  6 * time.Hour,
		JWTRefreshExpire: 30 * 24 * time.Hour,

		ChatModel:             getEnv("CHAT_MODEL", "deepseek-v4-pro"),
		AgentModel:            getEnv("AGENT_MODEL", "deepseek-v4-pro"),
		ChatModelBaseURL:      getEnv("CHAT_MODEL_BASE_URL", "https://api.deepseek.com"),
		ChatModelAPIKey:       getEnv("CHAT_MODEL_API_KEY", ""),
		EmbeddingModel:        getEnv("EMBEDDING_MODEL", "text-embedding-v3"),
		EmbeddingModelBaseURL: getEnv("EMBEDDING_MODEL_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		EmbeddingModelAPIKey:  getEnv("EMBEDDING_MODEL_API_KEY", ""),
		EmbeddingDimension:    1024,
		RerankModel:           getEnv("RERANK_MODEL", "qwen3-rerank"),
		RerankModelBaseURL:    getEnv("RERANK_MODEL_BASE_URL", "https://dashscope.aliyuncs.com/compatible-api/v1"),
		RerankModelAPIKey:     getEnv("RERANK_MODEL_API_KEY", ""),
		RAGScoreThreshold:     getEnvFloat("RAG_SCORE_THRESHOLD", 0.45),
		RAGFallbackMinScore:   getEnvFloat("RAG_FALLBACK_MIN_SCORE", 0),
		RAGThresholdFloor:     getEnvFloat("RAG_THRESHOLD_FLOOR", 0),
		RAGThresholdDegradeBy: getEnvFloat("RAG_THRESHOLD_DEGRADE_BY", 0),
		RAGTopK:               getEnvInt("RAG_TOP_K", 5),

		MinIOEndpoint:        getEnv("MINIO_ENDPOINT", ""),
		MinIOAccessKeyID:     getEnv("MINIO_ACCESS_KEY_ID", ""),
		MinIOSecretAccessKey: getEnv("MINIO_SECRET_ACCESS_KEY", ""),
		MinIOBucketName:      getEnv("MINIO_BUCKET_NAME", ""),
		MinIOUseSSL:          getEnvBool("MINIO_USE_SSL", false),

		WorkerConcurrency: 5,

		SkillSandboxMode:    getEnv("SKILL_SANDBOX_MODE", "disabled"),
		SkillSandboxTimeout: 10 * time.Second,
		SkillSandboxImage:   getEnv("SKILL_SANDBOX_IMAGE", "wechatopenai/weknora-sandbox:latest"),
	}

	if dim := os.Getenv("EMBEDDING_MODEL_DIMENSION"); dim != "" {
		if d, err := parseInt(dim); err == nil {
			cfg.EmbeddingDimension = d
		}
	}
	if conc := os.Getenv("WORKER_CONCURRENCY"); conc != "" {
		if c, err := parseInt(conc); err == nil {
			cfg.WorkerConcurrency = c
		}
	}
	if access := os.Getenv("JWT_ACCESS_EXPIRE"); access != "" {
		if d, err := time.ParseDuration(access); err == nil {
			cfg.JWTAccessExpire = d
		}
	}
	if refresh := os.Getenv("JWT_REFRESH_EXPIRE"); refresh != "" {
		if d, err := time.ParseDuration(refresh); err == nil {
			cfg.JWTRefreshExpire = d
		}
	}
	if timeout := os.Getenv("SKILL_SANDBOX_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil && d > 0 {
			cfg.SkillSandboxTimeout = d
		}
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := parseInt(v); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func parseInt(s string) (int, error) {
	var n int
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}
