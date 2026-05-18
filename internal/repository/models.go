package repository

import "time"

type User struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	Email        string    `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Nickname     string    `gorm:"size:100" json:"nickname"`
	Status       int       `gorm:"default:1" json:"status"` // 1=active, 0=disabled
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RefreshToken struct {
	ID        string     `gorm:"primaryKey;size:64" json:"id"`
	UserID    string     `gorm:"index;size:64;not null" json:"user_id"`
	TokenHash string     `gorm:"uniqueIndex;size:255;not null" json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type KnowledgeBase struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	UserID      string    `gorm:"index;size:64;not null" json:"user_id"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	Description string    `gorm:"size:2000" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Knowledge struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`
	FileName        string    `gorm:"size:500;not null" json:"file_name"`
	FilePath        string    `gorm:"size:1000" json:"file_path"`
	FileHash        string    `gorm:"index;size:128" json:"file_hash"`
	FileSize        int64     `json:"file_size"`
	ParseStatus     string    `gorm:"size:20;default:pending" json:"parse_status"` // pending, processing, done, failed
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Chunk struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`
	UserID        string    `gorm:"index;size:64;not null" json:"user_id"`
	KnowledgeID   string    `gorm:"index;size:64;not null" json:"knowledge_id"`
	ParentChunkID *string   `gorm:"size:64" json:"parent_chunk_id,omitempty"`
	Content       string    `gorm:"type:text;not null" json:"content"`
	Seq           int       `json:"seq"`
	StartPos      int       `json:"start_pos"`
	EndPos        int       `json:"end_pos"`
	ContextHeader string    `gorm:"size:1000" json:"context_header"`
	ChunkType     string    `gorm:"size:20;default:text" json:"chunk_type"` // text, image_caption, image_ocr
	CreatedAt     time.Time `json:"created_at"`
}

// ChunkLexicalIndex stores one document-level record per chunk for sparse
// retrieval statistics such as document length.
type ChunkLexicalIndex struct {
	ChunkID         string    `gorm:"primaryKey;size:64" json:"chunk_id"`
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`
	DocLen          int       `json:"doc_len"`
	CreatedAt       time.Time `json:"created_at"`
}

// ChunkTermPosting is the inverted-index posting list entry for one term in one
// chunk. TermFreq stores how often the term appears in that chunk.
type ChunkTermPosting struct {
	Term            string    `gorm:"primaryKey;size:255" json:"term"`
	ChunkID         string    `gorm:"primaryKey;size:64;index" json:"chunk_id"`
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`
	TermFreq        int       `json:"term_freq"`
	CreatedAt       time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	UserID    string    `gorm:"index;size:64;not null" json:"user_id"`
	Title     string    `gorm:"size:500" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID              string             `gorm:"primaryKey;size:64" json:"id"`
	SessionID       string             `gorm:"index;size:64;not null" json:"session_id"`
	Role            string             `gorm:"size:20;not null" json:"role"` // user, assistant
	Content         string             `gorm:"type:text;not null" json:"content"`
	RetrievalStatus string             `gorm:"size:20" json:"retrieval_status,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	References      []MessageReference `gorm:"-" json:"references,omitempty"`
}

// MessageReference stores the exact retrieval evidence used to generate an
// assistant message. The snapshot fields intentionally duplicate chunk content
// so historical answers remain explainable even if documents are re-indexed or
// chunks are later deleted.
type MessageReference struct {
	ID                    string    `gorm:"primaryKey;size:64" json:"id"`
	MessageID             string    `gorm:"index;size:64;not null" json:"message_id"`
	ChunkID               string    `gorm:"index;size:64;not null" json:"chunk_id"`
	KnowledgeID           string    `gorm:"index;size:64;not null" json:"knowledge_id"`
	KnowledgeBaseID       string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`
	Rank                  int       `gorm:"not null" json:"rank"`
	Score                 float64   `json:"score"`
	Seq                   int       `json:"seq"`
	ContextHeaderSnapshot string    `gorm:"size:1000" json:"context_header_snapshot"`
	ContentSnapshot       string    `gorm:"type:text;not null" json:"content_snapshot"`
	CreatedAt             time.Time `json:"created_at"`
}

type Memory struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	UserID     string    `gorm:"index;size:64;not null" json:"user_id"`
	MemoryText string    `gorm:"type:text;not null" json:"memory_text"`
	MemoryType string    `gorm:"size:20;default:fact" json:"memory_type"` // preference, profile, project, fact
	Importance int       `gorm:"default:0" json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
