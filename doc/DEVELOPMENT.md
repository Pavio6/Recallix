# Recallix 开发文档

## 1. 项目目标

实现一个名为 `Recallix` 的通用型 RAG 应用，支持：

1. 文档上传、解析、切分、向量化和入库
2. 基于知识库的多轮问答
3. 检索、重排、上下文合并、TopK 过滤
4. 父子 chunk
5. 跨会话长期记忆
6. 流式回答
7. 用户注册、登录和数据隔离
8. 可部署上线的服务结构

当前版本暂不考虑：

1. MCP
2. Skills
3. 图谱记忆
4. 复杂多 Agent 编排

系统定位：

```text
Recallix：一个支持知识库问答和长期记忆的通用 RAG 平台
```

---

## 2. 总体技术架构

### 2.1 技术选型

1. 后端
   - `Go`
   - 负责 API、业务编排、文档处理、RAG 链路、记忆读写

2. 业务数据库
   - `PostgreSQL`
   - 保存用户、知识库、文档、chunk 原文、会话、消息、长期记忆原文

3. 向量数据库
   - `Milvus`
   - 保存 chunk embedding 和 memory embedding

4. 异步任务
   - `Redis + Asynq`
   - 用于文档解析、向量化、记忆写入等异步任务

5. 模型能力
   - `Embedding Model`
     - 负责 query、chunk、memory 的向量化
   - `Rerank Model`
     - 对候选 chunk 重新排序
   - `Chat Model`
     - 负责问题改写、长期记忆提炼、最终答案生成

### 2.2 模型配置方式

第一版不做模型管理后台，也不在页面上动态切换模型。

三类模型都直接通过 `.env` 配置，由服务启动时读取：

1. `CHAT_MODEL`
   - 对话模型
   - 用于问题改写、长期记忆提炼、最终回答生成

2. `EMBEDDING_MODEL`
   - 向量模型
   - 用于 query、chunk、memory 的 embedding

3. `RERANK_MODEL`
   - 重排模型
   - 用于候选 chunk 的相关性重排

示例：

```env
CHAT_MODEL=your-chat-model
EMBEDDING_MODEL=your-embedding-model
RERANK_MODEL=your-rerank-model
```

这样设计的原因：

1. 第一版实现更简单
2. 模型选择由部署环境控制，便于上线和切换
3. 先把 RAG 主链路做稳定，后续再考虑模型管理能力

### 2.3 系统分层

1. 接口层
   - 文档上传接口
   - 知识库管理接口
   - 会话接口
   - 问答接口

2. 应用服务层
   - 文档入库服务
   - 文档解析服务
   - RAG 问答服务
   - 记忆服务

3. 领域能力层
   - Parser
   - Chunker
   - Retriever
   - Reranker
   - Prompt Builder
   - Memory Extractor

4. 基础设施层
   - PostgreSQL Repository
   - Milvus Repository
   - Redis / Asynq
   - 文件存储
   - 模型客户端

### 2.4 推荐项目目录结构

```text
rag-platform/
├── cmd/
│   ├── api/                    # HTTP 服务启动入口
│   └── worker/                 # 异步任务 worker 启动入口
├── configs/                    # 配置文件模板
├── migrations/                 # 数据库迁移脚本
├── docs/                       # 项目文档
├── internal/
│   ├── app/                    # 应用初始化、依赖装配
│   ├── config/                 # 配置读取
│   ├── middleware/             # JWT、日志、限流、CORS
│   ├── auth/                   # 注册、登录、密码、Token
│   ├── user/                   # 用户资料
│   ├── knowledge/              # 知识库与文档管理
│   ├── document/
│   │   ├── parser/             # md/txt/docx 解析器
│   │   ├── chunker/            # 切分策略
│   │   └── processor/          # 文档异步处理流程
│   ├── retrieval/
│   │   ├── embedding/          # 向量化
│   │   ├── vectorstore/        # Milvus 访问
│   │   ├── keyword/            # 关键词检索
│   │   ├── hybrid/             # 混合检索
│   │   └── rerank/             # 重排
│   ├── chat/
│   │   ├── session/            # 会话与消息
│   │   ├── query/              # 问题理解与改写
│   │   ├── prompt/             # Prompt 组装
│   │   └── service/            # RAG 问答主流程
│   ├── memory/                 # 长期记忆
│   ├── task/                   # Asynq 任务
│   ├── storage/                # 本地/对象存储抽象
│   ├── repository/             # PostgreSQL 仓储实现
│   ├── model/                  # Chat / Embedding / Rerank 客户端
│   └── shared/                 # 通用错误、分页、工具函数
├── web/                        # 前端项目，可后续独立拆仓
├── deploy/
│   ├── docker/
│   ├── compose/
│   └── nginx/
├── scripts/
├── go.mod
└── README.md
```

### 2.5 目录设计原则

1. `cmd` 只放启动入口，不承载业务逻辑
2. `internal` 按业务能力拆分，方便以后继续扩展
3. `document` 负责入库，`retrieval` 负责检索，职责分开
4. API 服务和 Worker 分离，耗时任务不阻塞请求线程
5. PostgreSQL、Milvus、Redis、文件存储和模型调用都通过接口封装，便于后续替换实现

---

## 3. 核心数据设计

### 3.1 PostgreSQL

#### `users`

保存用户信息：

1. `id`
2. `email`
3. `password_hash`
4. `nickname`
5. `status`
6. `created_at`
7. `updated_at`

#### `refresh_tokens`

保存刷新令牌：

1. `id`
2. `user_id`
3. `token_hash`
4. `expires_at`
5. `revoked_at`
6. `created_at`

#### `knowledge_bases`

保存知识库信息：

1. `id`
2. `user_id`
3. `name`
4. `description`
5. `created_at`
6. `updated_at`

#### `knowledges`

保存上传文档信息：

1. `id`
2. `user_id`
3. `knowledge_base_id`
4. `file_name`
5. `file_path`
6. `file_hash`
7. `file_size`
8. `parse_status`
9. `created_at`
10. `updated_at`

#### `chunks`

保存 chunk 业务数据：

1. `id`
2. `user_id`
3. `knowledge_id`
4. `parent_chunk_id`
5. `content`
6. `seq`
7. `start_pos`
8. `end_pos`
9. `context_header`
10. `chunk_type`
11. `created_at`

#### `sessions`

保存会话：

1. `id`
2. `user_id`
3. `title`
4. `created_at`
5. `updated_at`

#### `messages`

保存原始聊天消息：

1. `id`
2. `session_id`
3. `role`
4. `content`
5. `created_at`

#### `memories`

保存长期记忆原文：

1. `id`
2. `user_id`
3. `memory_text`
4. `memory_type`
5. `importance`
6. `created_at`
7. `updated_at`

### 3.2 Milvus

#### `chunk_vectors`

保存 chunk 向量：

1. `chunk_id`
2. `user_id`
3. `knowledge_base_id`
4. `embedding`

#### `memory_vectors`

保存记忆向量：

1. `memory_id`
2. `user_id`
3. `embedding`

---

## 4. 用户与认证

### 4.1 设计目标

1. 支持注册和登录
2. 保存每个用户自己的知识库、会话、消息和长期记忆
3. 所有核心查询都按 `user_id` 隔离
4. 为部署上线提供基础安全能力

### 4.2 第一版认证方案

1. 注册
   - 用户使用 `email + password`
   - 密码用 `bcrypt` 哈希后保存

2. 登录
   - 登录成功后返回：
     - `access_token`
     - `refresh_token`

3. Token
   - `access_token`
     - 使用 JWT
     - 有效期较短
   - `refresh_token`
     - 有效期较长
     - 数据库只保存 hash，便于注销和失效

4. 鉴权
   - 除注册、登录外，其他接口默认都需要登录
   - 中间件解析 JWT，得到 `user_id`
   - 后续所有知识库、文档、会话、记忆查询都必须附带 `user_id`

### 4.3 认证主流程

#### 注册

1. 用户提交邮箱和密码
2. 校验邮箱是否已存在
3. 使用 `bcrypt` 生成 `password_hash`
4. 创建用户记录
5. 返回注册成功

#### 登录

1. 用户提交邮箱和密码
2. 根据邮箱查询用户
3. 校验密码
4. 生成 `access_token`
5. 生成 `refresh_token`
6. 保存 `refresh_token` hash
7. 返回两个 token

#### 刷新 token

1. 客户端提交 `refresh_token`
2. 校验 token 是否存在、未过期、未撤销
3. 生成新的 `access_token`
4. 可选地轮换新的 `refresh_token`

### 4.4 用户隔离要求

1. `knowledge_bases` 必须归属用户
2. `knowledges` 必须归属用户
3. `chunks` 必须归属用户
4. `sessions`、`messages`、`memories` 必须归属用户
5. Milvus 检索必须增加 `user_id` 过滤

最终要保证：

```text
用户 A 不能查到用户 B 的知识库、文档、会话和记忆。
```

---

## 5. 业务流程一：文档入库

### 4.1 入库目标

把用户上传的原始文件，变成后续可检索的 chunk。

### 4.2 支持的文档格式

第一版只支持：

1. `md`
2. `txt`
3. `docx`

### 4.3 主流程

1. 用户上传文件
2. 校验知识库、文件类型、重复文件
3. 生成文件 hash
4. 在 PostgreSQL 中创建 `knowledge` 记录
5. 保存原始文件到本地存储
6. 回写 `file_path`
7. 使用 Asynq 投递异步文档解析任务
8. Worker 读取原始文件
9. 将不同格式统一解析成 `MarkdownContent`
10. 对 `MarkdownContent` 做文档切分
11. 保存 chunk 原文到 PostgreSQL 的 `chunks`
12. 调用 Embedding Model 生成向量
13. 将向量和 `chunk_id` 写入 Milvus
14. 更新文档状态为可用

### 4.4 为什么先统一成 `MarkdownContent`

1. 后续 chunker 只处理一种统一文本
2. `md`、`txt`、`docx` 不需要各写一套切分逻辑
3. 标题、段落、表格等结构更容易保留下来

### 4.5 文档切分策略

#### 核心原则

1. 不是固定长度硬切
2. 优先保留结构和自然边界
3. 再受长度约束
4. 最后通过 overlap 保留边界语义

#### 默认配置

1. `chunk_size = 512`
2. `chunk_overlap = 80`
3. 单位为 Unicode 字符数

#### 支持策略

1. `auto`
   - 自动分析文档结构并选择策略

2. `heading`
   - 优先按 Markdown 标题切

3. `heuristic`
   - 面向报告类文本的启发式结构切分

4. `legacy / recursive`
   - 递归分隔符切分，作为稳定兜底方案

#### 普通递归切分流程

1. 先识别不能随便拆开的内容
   - 表格
   - 代码块
   - 链接
   - 图片引用

2. 按分隔符递归切成临时 `unit`
   - 空行 `\n\n`
   - 换行 `\n`
   - 中文句号 `。`
   -  `！`、`？`、`;`、`；`

3. 再把多个 `unit` 合并成最终 chunk

4. 合并时控制 `chunk_size`

5. 相邻 chunk 之间尽量保留 `chunk_overlap`

#### 父子 chunk

1. `child chunk`
   - 小块
   - 负责向量检索

2. `parent chunk`
   - 大块
   - 负责最终返回给模型阅读

一句话：

```text
小块负责召回，大块负责阅读。
```

---

## 6. 业务流程二：RAG 提问链路

### 6.1 提问目标

根据用户问题，从知识库和长期记忆中找到相关上下文，再生成答案。

### 6.2 主流程

1. 接收用户问题
2. 加载当前会话最近几轮历史消息
3. 调用 Chat Model 做问题理解
4. 输出：
   - `rewrite_query`
   - `intent`
5. 如果是非检索意图，可跳过知识库检索
6. 用 `rewrite_query` 检索长期记忆
7. 用 `rewrite_query` 检索知识库 chunk
8. 进行混合召回
   - 向量检索
   - 关键词检索
9. 对候选 chunk 执行 rerank
10. 去重、合并相邻或重叠 chunk
11. 如果命中 child chunk，补充对应 parent chunk
12. 按得分过滤 TopK
13. 组装 Prompt
14. 调用 Chat Model 流式生成答案
15. 保存问答消息
16. 异步触发长期记忆提炼

### 6.3 Prompt 组成顺序

1. System Prompt
2. 长期记忆
3. 最近历史消息
4. TopK 知识库上下文
5. 当前用户问题

### 6.4 检索链路中的关键点

1. `rewrite_query`
   - 让模糊问题变得更适合检索

2. 混合检索
   - 向量检索负责语义相似
   - 关键词检索负责精确词命中

3. `rerank`
   - 对初召回结果重新排序
   - 提高最终上下文质量

4. 合并
   - 去重
   - 合并相邻 chunk
   - 减少上下文碎片

5. `TopK`
   - 控制最终进入 Prompt 的内容数量
   - 避免 prompt 被无关文本污染

---

## 7. 业务流程三：长期记忆

### 7.1 设计目标

长期记忆解决：

```text
跨会话仍然记得用户的重要信息
```

会话历史只解决当前会话内的连续性，不能替代长期记忆。

### 7.2 记忆结构

当前版本采用：

```text
memory_text + embedding + metadata
```

不采用 Neo4j 图谱记忆。

### 7.3 写入流程

1. 一轮回答完成
2. 读取本轮用户问题和助手回答
3. 调用 Chat Model 判断是否值得长期保存
4. 如果值得，提炼成简洁 `memory_text`
5. 将记忆原文写入 PostgreSQL
6. 调用 Embedding Model 生成 memory embedding
7. 将 `memory_id + embedding + user_id` 写入 Milvus

### 7.4 召回流程

1. 用户发来新问题
2. 先得到 `rewrite_query`
3. 对 `rewrite_query` 做 embedding
4. 在 Milvus 中按 `user_id` 过滤检索 memory
5. 取相关度最高的少量 memory
6. 加入最终 Prompt

### 7.5 为什么这样设计

1. 比图数据库方案更简单
2. 比纯关键词召回更稳
3. 可以跨会话召回用户偏好、项目背景、长期事实
4. PostgreSQL 保存业务原文，Milvus 保存向量索引，职责清晰

---

## 8. 核心模块拆分

### 8.1 文档模块

1. `KnowledgeService`
   - 创建文档记录
   - 文件去重
   - 状态管理

2. `FileService`
   - 保存和读取原始文件

3. `DocumentProcessService`
   - 执行异步解析任务

4. `ParserRegistry`
   - 根据文件类型选择解析器

5. `ChunkService`
   - 负责 chunk 生成和保存

### 8.2 检索模块

1. `EmbeddingService`
2. `VectorStore`
3. `KeywordRetriever`
4. `HybridRetriever`
5. `RerankService`
6. `ContextMergeService`

### 8.3 问答模块

1. `SessionService`
2. `MessageService`
3. `QueryUnderstandService`
4. `PromptBuilder`
5. `ChatService`

### 8.4 记忆模块

1. `MemoryService`
2. `MemoryExtractor`
3. `MemoryRepository`
4. `MemoryVectorStore`

---

## 9. 推荐接口设计

### 9.1 认证

1. `POST /auth/register`
2. `POST /auth/login`
3. `POST /auth/refresh`
4. `POST /auth/logout`

### 9.2 文档与知识库

1. `POST /knowledge-bases`
2. `GET /knowledge-bases`
3. `POST /knowledge-bases/:id/files`
4. `GET /knowledges/:id`

### 9.3 会话与问答

1. `POST /sessions`
2. `GET /sessions/:id/messages`
3. `POST /sessions/:id/chat`

### 9.4 记忆

1. `GET /users/:id/memories`
2. `DELETE /memories/:id`

---

## 10. 部署架构

### 10.1 线上部署组件

1. `api`
   - 对外提供 HTTP 接口

2. `worker`
   - 消费 Asynq 异步任务

3. `postgres`
   - 保存业务数据

4. `milvus`
   - 保存向量索引

5. `redis`
   - 保存异步任务和缓存

6. `object storage`
   - 第一版可先用本地存储
   - 上线后建议使用 S3 兼容对象存储保存原始文件

7. `nginx`
   - 反向代理
   - HTTPS
   - 静态资源分发

### 10.2 推荐部署形态

```text
Browser
-> Nginx
-> API Service
   -> PostgreSQL
   -> Milvus
   -> Redis
   -> Object Storage

Worker Service
-> Redis
-> PostgreSQL
-> Milvus
-> Object Storage
```

### 10.3 上线前必须具备的能力

1. 环境变量配置
2. 数据库迁移
3. 日志
4. 健康检查接口
5. CORS
6. 请求鉴权
7. 文件大小限制
8. 基础限流
9. Dockerfile
10. `docker-compose` 或云服务器部署脚本

---

## 11. 推荐开发顺序

### 第一阶段：项目骨架与认证

1. 搭建目录结构
2. 配置读取
3. PostgreSQL 连接与 migration
4. 注册、登录、JWT 中间件
5. 用户数据隔离基础能力

### 第二阶段：文档入库

1. 文件上传
2. `knowledge` 保存
3. 文件落盘
4. 异步解析任务
5. `md / txt / docx` 转 `MarkdownContent`
6. `legacy / recursive` 切分
7. chunk 入库
8. embedding 写入 Milvus

### 第三阶段：基础 RAG 问答

1. 用户提问
2. query embedding
3. Milvus 召回
4. PostgreSQL 回表拿 chunk 原文
5. Prompt 组装
6. Chat Model 回答
7. SSE 流式返回

### 第四阶段：增强检索质量

1. 问题改写
2. 关键词检索
3. 混合召回
4. rerank
5. chunk 合并
6. TopK
7. 父子 chunk

### 第五阶段：长期记忆

1. 会话历史保存
2. 回答后提炼 memory
3. memory 原文入库
4. memory 向量化
5. 下次提问时召回 memory
6. 加入 Prompt

### 第六阶段：部署上线

1. Dockerfile
2. `docker-compose`
3. Nginx
4. 环境变量配置
5. 健康检查
6. HTTPS
7. 数据备份

---

## 12. 关键实现约束

1. chunk 原文必须保存在 PostgreSQL
   - 便于回表、展示、引用和调试

2. Milvus 中只保存向量和关联 ID
   - 不承担业务主存储职责

3. 原始聊天消息必须保存
   - 长期记忆是补充，不是消息替代品

4. 文档解析必须异步
   - 避免上传接口阻塞

5. 问答链路必须支持流式返回
   - 提升用户体验

6. 第一版先保证标准 RAG 稳定
   - 不要过早引入复杂 Agent、MCP、Skills 或图谱记忆

7. 所有核心业务数据都要带 `user_id`
   - 方便做权限隔离，也方便以后扩展团队空间和计费

---

## 13. 最终业务闭环

```text
用户注册 / 登录
-> 文档上传
-> 文档解析
-> 文档切分
-> chunk 入库
-> 向量索引
-> 用户提问
-> 历史加载
-> 问题改写
-> 记忆召回
-> 知识检索
-> 重排与合并
-> TopK
-> Prompt 组装
-> 流式回答
-> 消息落库
-> 长期记忆写入
```

这就是 `Recallix` 第一版需要完整打通的主流程。
