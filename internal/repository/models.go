package repository

import "time"

// =============================================================================
// 用户认证相关表
// =============================================================================

// User 用户表：存储系统用户信息
type User struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`                       // 用户唯一标识（UUID）
	Email        string    `gorm:"uniqueIndex;size:255;not null" json:"email"`          // 用户邮箱，唯一索引，用于登录
	PasswordHash string    `gorm:"size:255;not null" json:"-"`                          // 密码哈希值（bcrypt），JSON 序列化时隐藏
	Nickname     string    `gorm:"size:100" json:"nickname"`                            // 用户昵称
	Status       int       `gorm:"default:1" json:"status"`                            // 用户状态：1=启用, 0=禁用
	CreatedAt    time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt    time.Time `json:"updated_at"`                                          // 更新时间
}

// RefreshToken 刷新令牌表：存储用户的 JWT 刷新令牌
type RefreshToken struct {
	ID        string     `gorm:"primaryKey;size:64" json:"id"`                      // 令牌唯一标识
	UserID    string     `gorm:"index;size:64;not null" json:"user_id"`             // 关联用户 ID，索引
	TokenHash string     `gorm:"uniqueIndex;size:255;not null" json:"-"`            // 令牌哈希值，唯一索引
	ExpiresAt time.Time  `json:"expires_at"`                                        // 过期时间
	RevokedAt *time.Time `json:"revoked_at,omitempty"`                              // 撤销时间（未撤销为 null）
	CreatedAt time.Time  `json:"created_at"`                                        // 创建时间
}

// =============================================================================
// 知识库相关表
// =============================================================================

// KnowledgeBase 知识库表：存储用户创建的知识库
type KnowledgeBase struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`                       // 知识库唯一标识
	UserID      string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	Name        string    `gorm:"size:255;not null" json:"name"`                      // 知识库名称
	Description string    `gorm:"size:2000" json:"description"`                       // 知识库描述
	CreatedAt   time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt   time.Time `json:"updated_at"`                                          // 更新时间
}

// Knowledge 知识文档表：存储上传的文档元数据
type Knowledge struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`                       // 文档唯一标识
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`    // 所属知识库 ID，索引
	FileName        string    `gorm:"size:500;not null" json:"file_name"`                 // 原始文件名
	FilePath        string    `gorm:"size:1000" json:"file_path"`                         // MinIO 存储路径（minio://bucket/key 格式）
	FileHash        string    `gorm:"index;size:128" json:"file_hash"`                    // 文件 SHA-256 哈希值，用于去重
	FileSize        int64     `json:"file_size"`                                          // 文件大小（字节）
	ParseStatus     string    `gorm:"size:20;default:pending" json:"parse_status"`        // 解析状态：pending=待处理, processing=处理中, done=完成, failed=失败
	CreatedAt       time.Time `json:"created_at"`                                         // 创建时间
	UpdatedAt       time.Time `json:"updated_at"`                                         // 更新时间
}

// =============================================================================
// 文档分块相关表
// =============================================================================

// Chunk 文档分块表：存储文档切分后的内容片段
type Chunk struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`                       // 分块唯一标识
	UserID        string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	KnowledgeID   string    `gorm:"index;size:64;not null" json:"knowledge_id"`         // 所属文档 ID，索引
	ParentChunkID *string   `gorm:"size:64" json:"parent_chunk_id,omitempty"`           // 父分块 ID（用于层级分块）
	Content       string    `gorm:"type:text;not null" json:"content"`                  // 分块文本内容
	Seq           int       `json:"seq"`                                                // 在文档中的序号（0 开始）
	StartPos      int       `json:"start_pos"`                                          // 在原文中的起始位置（rune 偏移）
	EndPos        int       `json:"end_pos"`                                            // 在原文中的结束位置（rune 偏移）
	ContextHeader string    `gorm:"size:1000" json:"context_header"`                    // 面包屑上下文（如 "# 标题 > ## 子标题"）
	ChunkType     string    `gorm:"size:20;default:text" json:"chunk_type"`             // 分块类型：text=文本, image_caption=图片描述, image_ocr=图片 OCR
	CreatedAt     time.Time `json:"created_at"`                                         // 创建时间
}

// ChunkLexicalIndex 分块词法索引表：存储每个分块的文档级统计信息，用于 BM25 检索
type ChunkLexicalIndex struct {
	ChunkID         string    `gorm:"primaryKey;size:64" json:"chunk_id"`                // 分块 ID（主键）
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`             // 所属用户 ID，索引
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`   // 所属知识库 ID，索引
	DocLen          int       `json:"doc_len"`                                           // 分块的 token 数量（用于 BM25 长度归一化）
	CreatedAt       time.Time `json:"created_at"`                                        // 创建时间
}

// ChunkTermPosting 分块词项倒排表：存储词项到分块的倒排索引，用于 BM25 关键词检索
type ChunkTermPosting struct {
	Term            string    `gorm:"primaryKey;size:255" json:"term"`                   // 词项（主键一部分）
	ChunkID         string    `gorm:"primaryKey;size:64;index" json:"chunk_id"`          // 分块 ID（主键一部分）
	UserID          string    `gorm:"index;size:64;not null" json:"user_id"`             // 所属用户 ID，索引
	KnowledgeBaseID string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`   // 所属知识库 ID，索引
	TermFreq        int       `json:"term_freq"`                                         // 词项在该分块中的出现次数
	CreatedAt       time.Time `json:"created_at"`                                        // 创建时间
}

// =============================================================================
// 会话与消息相关表
// =============================================================================

// Session 会话表：存储用户的对话会话
type Session struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`                       // 会话唯一标识
	UserID    string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	Title     string    `gorm:"size:500" json:"title"`                              // 会话标题（默认为用户首条消息）
	Mode      string    `gorm:"size:30;default:quick_answer" json:"mode"`           // 会话模式：quick_answer=快速回答, agent_reasoning=Agent 推理
	AgentID   *string   `gorm:"size:64" json:"agent_id,omitempty"`                  // 关联的 Agent ID（可选）
	CreatedAt time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt time.Time `json:"updated_at"`                                          // 更新时间
}

// Message 消息表：存储对话中的每条消息
type Message struct {
	ID              string                `gorm:"primaryKey;size:64" json:"id"`               // 消息唯一标识
	SessionID       string                `gorm:"index;size:64;not null" json:"session_id"`   // 所属会话 ID，索引
	Role            string                `gorm:"size:20;not null" json:"role"`               // 角色：user=用户, assistant=助手
	Content         string                `gorm:"type:text;not null" json:"content"`          // 消息内容
	RetrievalStatus string                `gorm:"size:20" json:"retrieval_status,omitempty"`  // 检索状态：hit=命中, miss=未命中, skipped=跳过
	CreatedAt       time.Time             `json:"created_at"`                                 // 创建时间
	References      []MessageReference    `gorm:"-" json:"references,omitempty"`              // 检索引用（运行时填充）
	SkillTrace      *MessageSkillTrace    `gorm:"-" json:"skill_trace,omitempty"`             // 技能调用追踪（运行时填充）
	UsedSkills      []MessageSkillSummary `gorm:"-" json:"used_skills,omitempty"`             // 使用的技能摘要（运行时填充）
}

// MessageReference 消息引用表：存储助手消息使用的检索证据快照
type MessageReference struct {
	ID                    string    `gorm:"primaryKey;size:64" json:"id"`                       // 引用唯一标识
	MessageID             string    `gorm:"index;size:64;not null" json:"message_id"`           // 所属消息 ID，索引
	ChunkID               string    `gorm:"index;size:64;not null" json:"chunk_id"`             // 引用的分块 ID，索引
	KnowledgeID           string    `gorm:"index;size:64;not null" json:"knowledge_id"`         // 引用的文档 ID，索引
	KnowledgeBaseID       string    `gorm:"index;size:64;not null" json:"knowledge_base_id"`    // 引用的知识库 ID，索引
	Rank                  int       `gorm:"not null" json:"rank"`                               // 引用排名（1 开始）
	Score                 float64   `json:"score"`                                              // 检索得分
	Seq                   int       `json:"seq"`                                                // 分块在文档中的序号
	ContextHeaderSnapshot string    `gorm:"size:1000" json:"context_header_snapshot"`           // 面包屑上下文快照
	ContentSnapshot       string    `gorm:"type:text;not null" json:"content_snapshot"`         // 分块内容快照（即使原文被删除也可追溯）
	CreatedAt             time.Time `json:"created_at"`                                         // 创建时间
}

// MessageSkillTrace 消息技能追踪表：存储技能调度和选择的决策过程
type MessageSkillTrace struct {
	ID                  string    `gorm:"primaryKey;size:64" json:"id"`                           // 追踪唯一标识
	MessageID           string    `gorm:"uniqueIndex;size:64;not null" json:"message_id"`         // 所属消息 ID，唯一索引
	CandidateSkillsJSON string    `gorm:"type:text;not null;default:'[]'" json:"candidate_skills_json"`  // 候选技能列表（JSON）
	SelectedSkillsJSON  string    `gorm:"type:text;not null;default:'[]'" json:"selected_skills_json"`   // 选中技能列表（JSON）
	SelectorRawOutput   string    `gorm:"type:text" json:"selector_raw_output"`                   // 选择器原始输出
	CreatedAt           time.Time `json:"created_at"`                                             // 创建时间
}

// MessageSkillSummary 消息技能摘要：运行时结构，不直接存储到数据库
type MessageSkillSummary struct {
	ID          string  `json:"id"`                          // 技能 ID
	Name        string  `json:"name"`                        // 技能名称
	Description string  `json:"description,omitempty"`       // 技能描述
	Score       float64 `json:"score,omitempty"`             // 技能匹配得分
}

// =============================================================================
// 记忆相关表
// =============================================================================

// Memory 用户记忆表：存储从对话中提取的用户偏好和事实
type Memory struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`                       // 记忆唯一标识
	UserID     string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	MemoryText string    `gorm:"type:text;not null" json:"memory_text"`              // 记忆内容文本
	MemoryType string    `gorm:"size:20;default:fact" json:"memory_type"`            // 记忆类型：preference=偏好, profile=画像, project=项目, fact=事实
	Importance int       `gorm:"default:0" json:"importance"`                        // 重要性评分（0-10）
	CreatedAt  time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt  time.Time `json:"updated_at"`                                          // 更新时间
}

// =============================================================================
// Agent 与技能相关表
// =============================================================================

// Agent 智能代理表：存储用户创建的 AI Agent 配置
type Agent struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`                       // Agent 唯一标识
	UserID    string    `gorm:"index;size:64;not null" json:"user_id"`              // 所属用户 ID，索引
	Name      string    `gorm:"size:255;not null" json:"name"`                      // Agent 名称
	Nickname  string    `gorm:"size:255" json:"nickname"`                            // Agent 昵称
	Model     string    `gorm:"size:255;not null" json:"model"`                     // 使用的 LLM 模型
	Prompt    string    `gorm:"type:text;not null" json:"prompt"`                   // 系统提示词
	CreatedAt time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt time.Time `json:"updated_at"`                                          // 更新时间
	Skills    []Skill   `gorm:"-" json:"skills,omitempty"`                          // 关联的技能列表（运行时填充）
}

// Skill 技能表：存储可复用的技能定义（如 SKILL.md 文件）
type Skill struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`                       // 技能唯一标识
	UserID        string    `gorm:"index;size:64" json:"user_id"`                      // 所属用户 ID，索引（系统技能为 null）
	Name          string    `gorm:"size:255;not null" json:"name"`                      // 技能名称
	Description   string    `gorm:"size:1000" json:"description"`                       // 技能描述
	SourceURL     string    `gorm:"size:2000" json:"source_url"`                        // 来源 URL
	SourceRepo    string    `gorm:"size:500" json:"source_repo"`                        // 来源仓库
	SourceRef     string    `gorm:"size:255" json:"source_ref"`                         // 来源引用（分支/标签）
	SourcePath    string    `gorm:"size:1000" json:"source_path"`                       // 来源路径
	StoragePrefix string    `gorm:"size:1000" json:"storage_prefix"`                    // MinIO 存储前缀
	EntryFileURI  string    `gorm:"size:2000" json:"entry_file_uri"`                    // 入口文件 URI
	FileCount     int       `gorm:"default:0" json:"file_count"`                        // 文件数量
	Enabled       bool      `gorm:"default:true" json:"enabled"`                        // 是否启用
	CreatedAt     time.Time `json:"created_at"`                                          // 创建时间
	UpdatedAt     time.Time `json:"updated_at"`                                          // 更新时间
	AgentCount    int64     `gorm:"-" json:"agent_count"`                               // 使用此技能的 Agent 数量（运行时填充）

	// Legacy fields are kept only so an existing development database created by
	// the previous prompt-fragment implementation can still accept new rows.
	// New runtime behavior no longer reads either field.
	LegacyInstructions string `gorm:"column:instructions;type:text;not null;default:''" json:"-"`       // 废弃：旧版指令字段
	LegacySourceType   string `gorm:"column:source_type;size:20;not null;default:github" json:"-"`      // 废弃：旧版来源类型
}

// AgentSkill Agent 技能关联表：存储 Agent 和 Skill 的多对多关系
type AgentSkill struct {
	AgentID string `gorm:"primaryKey;size:64" json:"agent_id"`   // Agent ID（主键一部分）
	SkillID string `gorm:"primaryKey;size:64" json:"skill_id"`   // Skill ID（主键一部分）
}
