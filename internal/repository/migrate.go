package repository

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// AutoMigrate 自动迁移数据库表结构
// 此函数会在应用启动时自动调用，创建或更新数据库表
func AutoMigrate(db *gorm.DB) error {
	// 启用 pgvector 扩展（幂等操作）
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		log.Printf("[Migrate] Warning: Failed to enable vector extension: %v", err)
	}

	// 按顺序迁移所有表
	// 注意：表的顺序很重要，需要先创建被引用的表
	models := []interface{}{
		// 1. 用户认证相关
		&User{},          // 用户表
		&RefreshToken{},  // 刷新令牌表

		// 2. 知识库相关
		&KnowledgeBase{}, // 知识库表
		&Knowledge{},     // 知识文档表

		// 3. 文档分块相关
		&Chunk{},              // 文档分块表
		&ChunkLexicalIndex{},  // 分块词法索引表
		&ChunkTermPosting{},   // 分块词项倒排表

		// 4. 会话与消息相关
		&Session{},            // 会话表
		&Message{},            // 消息表
		&MessageReference{},   // 消息引用表
		&MessageSkillTrace{},  // 消息技能追踪表

		// 5. 记忆相关
		&Memory{}, // 用户记忆表

		// 6. Agent 与技能相关
		&Agent{},      // 智能代理表
		&Skill{},      // 技能表
		&AgentSkill{}, // Agent 技能关联表
	}

	// 执行迁移
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
	}

	log.Println("[Migrate] Database migration completed successfully")
	return nil
}

// AddTableComments 添加表和字段的数据库注释
// PostgreSQL 支持 COMMENT ON 语法来添加注释
func AddTableComments(db *gorm.DB) error {
	comments := []string{
		// 用户认证相关
		`COMMENT ON TABLE users IS '用户表：存储系统用户信息'`,
		`COMMENT ON COLUMN users.id IS '用户唯一标识（UUID）'`,
		`COMMENT ON COLUMN users.email IS '用户邮箱，唯一索引，用于登录'`,
		`COMMENT ON COLUMN users.password_hash IS '密码哈希值（bcrypt）'`,
		`COMMENT ON COLUMN users.nickname IS '用户昵称'`,
		`COMMENT ON COLUMN users.status IS '用户状态：1=启用, 0=禁用'`,

		`COMMENT ON TABLE refresh_tokens IS '刷新令牌表：存储用户的 JWT 刷新令牌'`,
		`COMMENT ON COLUMN refresh_tokens.id IS '令牌唯一标识'`,
		`COMMENT ON COLUMN refresh_tokens.user_id IS '关联用户 ID'`,
		`COMMENT ON COLUMN refresh_tokens.token_hash IS '令牌哈希值'`,
		`COMMENT ON COLUMN refresh_tokens.expires_at IS '过期时间'`,
		`COMMENT ON COLUMN refresh_tokens.revoked_at IS '撤销时间（未撤销为 null）'`,

		// 知识库相关
		`COMMENT ON TABLE knowledge_bases IS '知识库表：存储用户创建的知识库'`,
		`COMMENT ON COLUMN knowledge_bases.id IS '知识库唯一标识'`,
		`COMMENT ON COLUMN knowledge_bases.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN knowledge_bases.name IS '知识库名称'`,
		`COMMENT ON COLUMN knowledge_bases.description IS '知识库描述'`,

		`COMMENT ON TABLE knowledges IS '知识文档表：存储上传的文档元数据'`,
		`COMMENT ON COLUMN knowledges.id IS '文档唯一标识'`,
		`COMMENT ON COLUMN knowledges.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN knowledges.knowledge_base_id IS '所属知识库 ID'`,
		`COMMENT ON COLUMN knowledges.file_name IS '原始文件名'`,
		`COMMENT ON COLUMN knowledges.file_path IS 'MinIO 存储路径（minio://bucket/key 格式）'`,
		`COMMENT ON COLUMN knowledges.file_hash IS '文件 SHA-256 哈希值，用于去重'`,
		`COMMENT ON COLUMN knowledges.file_size IS '文件大小（字节）'`,
		`COMMENT ON COLUMN knowledges.parse_status IS '解析状态：pending=待处理, processing=处理中, done=完成, failed=失败'`,

		// 文档分块相关
		`COMMENT ON TABLE chunks IS '文档分块表：存储文档切分后的内容片段'`,
		`COMMENT ON COLUMN chunks.id IS '分块唯一标识'`,
		`COMMENT ON COLUMN chunks.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN chunks.knowledge_id IS '所属文档 ID'`,
		`COMMENT ON COLUMN chunks.parent_chunk_id IS '父分块 ID（用于层级分块）'`,
		`COMMENT ON COLUMN chunks.content IS '分块文本内容'`,
		`COMMENT ON COLUMN chunks.seq IS '在文档中的序号（0 开始）'`,
		`COMMENT ON COLUMN chunks.start_pos IS '在原文中的起始位置（rune 偏移）'`,
		`COMMENT ON COLUMN chunks.end_pos IS '在原文中的结束位置（rune 偏移）'`,
		`COMMENT ON COLUMN chunks.context_header IS '面包屑上下文（如 "# 标题 > ## 子标题"）'`,
		`COMMENT ON COLUMN chunks.chunk_type IS '分块类型：text=文本, image_caption=图片描述, image_ocr=图片 OCR'`,

		`COMMENT ON TABLE chunk_lexical_indices IS '分块词法索引表：存储每个分块的文档级统计信息，用于 BM25 检索'`,
		`COMMENT ON COLUMN chunk_lexical_indices.chunk_id IS '分块 ID'`,
		`COMMENT ON COLUMN chunk_lexical_indices.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN chunk_lexical_indices.knowledge_base_id IS '所属知识库 ID'`,
		`COMMENT ON COLUMN chunk_lexical_indices.doc_len IS '分块的 token 数量（用于 BM25 长度归一化）'`,

		`COMMENT ON TABLE chunk_term_postings IS '分块词项倒排表：存储词项到分块的倒排索引，用于 BM25 关键词检索'`,
		`COMMENT ON COLUMN chunk_term_postings.term IS '词项'`,
		`COMMENT ON COLUMN chunk_term_postings.chunk_id IS '分块 ID'`,
		`COMMENT ON COLUMN chunk_term_postings.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN chunk_term_postings.knowledge_base_id IS '所属知识库 ID'`,
		`COMMENT ON COLUMN chunk_term_postings.term_freq IS '词项在该分块中的出现次数'`,

		// 会话与消息相关
		`COMMENT ON TABLE sessions IS '会话表：存储用户的对话会话'`,
		`COMMENT ON COLUMN sessions.id IS '会话唯一标识'`,
		`COMMENT ON COLUMN sessions.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN sessions.title IS '会话标题（默认为用户首条消息）'`,
		`COMMENT ON COLUMN sessions.mode IS '会话模式：quick_answer=快速回答, agent_reasoning=Agent 推理'`,
		`COMMENT ON COLUMN sessions.agent_id IS '关联的 Agent ID（可选）'`,

		`COMMENT ON TABLE messages IS '消息表：存储对话中的每条消息'`,
		`COMMENT ON COLUMN messages.id IS '消息唯一标识'`,
		`COMMENT ON COLUMN messages.session_id IS '所属会话 ID'`,
		`COMMENT ON COLUMN messages.role IS '角色：user=用户, assistant=助手'`,
		`COMMENT ON COLUMN messages.content IS '消息内容'`,
		`COMMENT ON COLUMN messages.retrieval_status IS '检索状态：hit=命中, miss=未命中, skipped=跳过'`,

		`COMMENT ON TABLE message_references IS '消息引用表：存储助手消息使用的检索证据快照'`,
		`COMMENT ON COLUMN message_references.id IS '引用唯一标识'`,
		`COMMENT ON COLUMN message_references.message_id IS '所属消息 ID'`,
		`COMMENT ON COLUMN message_references.chunk_id IS '引用的分块 ID'`,
		`COMMENT ON COLUMN message_references.knowledge_id IS '引用的文档 ID'`,
		`COMMENT ON COLUMN message_references.knowledge_base_id IS '引用的知识库 ID'`,
		`COMMENT ON COLUMN message_references.rank IS '引用排名（1 开始）'`,
		`COMMENT ON COLUMN message_references.score IS '检索得分'`,
		`COMMENT ON COLUMN message_references.seq IS '分块在文档中的序号'`,
		`COMMENT ON COLUMN message_references.context_header_snapshot IS '面包屑上下文快照'`,
		`COMMENT ON COLUMN message_references.content_snapshot IS '分块内容快照（即使原文被删除也可追溯）'`,

		`COMMENT ON TABLE message_skill_traces IS '消息技能追踪表：存储技能调度和选择的决策过程'`,
		`COMMENT ON COLUMN message_skill_traces.id IS '追踪唯一标识'`,
		`COMMENT ON COLUMN message_skill_traces.message_id IS '所属消息 ID'`,
		`COMMENT ON COLUMN message_skill_traces.candidate_skills_json IS '候选技能列表（JSON）'`,
		`COMMENT ON COLUMN message_skill_traces.selected_skills_json IS '选中技能列表（JSON）'`,
		`COMMENT ON COLUMN message_skill_traces.selector_raw_output IS '选择器原始输出'`,

		// 记忆相关
		`COMMENT ON TABLE memories IS '用户记忆表：存储从对话中提取的用户偏好和事实'`,
		`COMMENT ON COLUMN memories.id IS '记忆唯一标识'`,
		`COMMENT ON COLUMN memories.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN memories.memory_text IS '记忆内容文本'`,
		`COMMENT ON COLUMN memories.memory_type IS '记忆类型：preference=偏好, profile=画像, project=项目, fact=事实'`,
		`COMMENT ON COLUMN memories.importance IS '重要性评分（0-10）'`,

		// Agent 与技能相关
		`COMMENT ON TABLE agents IS '智能代理表：存储用户创建的 AI Agent 配置'`,
		`COMMENT ON COLUMN agents.id IS 'Agent 唯一标识'`,
		`COMMENT ON COLUMN agents.user_id IS '所属用户 ID'`,
		`COMMENT ON COLUMN agents.name IS 'Agent 名称'`,
		`COMMENT ON COLUMN agents.nickname IS 'Agent 昵称'`,
		`COMMENT ON COLUMN agents.model IS '使用的 LLM 模型'`,
		`COMMENT ON COLUMN agents.prompt IS '系统提示词'`,

		`COMMENT ON TABLE skills IS '技能表：存储可复用的技能定义（如 SKILL.md 文件）'`,
		`COMMENT ON COLUMN skills.id IS '技能唯一标识'`,
		`COMMENT ON COLUMN skills.user_id IS '所属用户 ID（系统技能为 null）'`,
		`COMMENT ON COLUMN skills.name IS '技能名称'`,
		`COMMENT ON COLUMN skills.description IS '技能描述'`,
		`COMMENT ON COLUMN skills.source_url IS '来源 URL'`,
		`COMMENT ON COLUMN skills.source_repo IS '来源仓库'`,
		`COMMENT ON COLUMN skills.source_ref IS '来源引用（分支/标签）'`,
		`COMMENT ON COLUMN skills.source_path IS '来源路径'`,
		`COMMENT ON COLUMN skills.storage_prefix IS 'MinIO 存储前缀'`,
		`COMMENT ON COLUMN skills.entry_file_uri IS '入口文件 URI'`,
		`COMMENT ON COLUMN skills.file_count IS '文件数量'`,
		`COMMENT ON COLUMN skills.enabled IS '是否启用'`,

		`COMMENT ON TABLE agent_skills IS 'Agent 技能关联表：存储 Agent 和 Skill 的多对多关系'`,
		`COMMENT ON COLUMN agent_skills.agent_id IS 'Agent ID'`,
		`COMMENT ON COLUMN agent_skills.skill_id IS 'Skill ID'`,
	}

	for _, comment := range comments {
		if err := db.Exec(comment).Error; err != nil {
			log.Printf("[Migrate] Warning: Failed to add comment: %v", err)
		}
	}

	log.Println("[Migrate] Table comments added successfully")
	return nil
}

// RunMigration 执行完整的数据库迁移流程
func RunMigration(db *gorm.DB) error {
	// 1. 自动迁移表结构
	if err := AutoMigrate(db); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	// 2. 添加表注释
	if err := AddTableComments(db); err != nil {
		return fmt.Errorf("add comments failed: %w", err)
	}

	return nil
}
