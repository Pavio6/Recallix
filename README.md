# Recallix

基于 RAG 架构的私有知识库智能问答系统。支持多格式文档入库、自动分片、向量检索、混合召回、重排序和流式问答，实现知识文档的智能检索与结果溯源。

## 整体架构

```
用户上传文档（.md / .txt / .docx）
        │
        ▼
   MinIO 对象存储 ──→ Redis / Asynq 异步队列
        │                    │
        │                    ▼
        │          解析 → 分片 → 嵌入 → Milvus
        │
        ▼
  用户提问 ──→ 意图识别 + 问题改写
        │
        ▼
  混合检索（向量 + 关键词）
        │
        ▼
  Rerank + 分数阈值过滤
        │
        ▼
  Prompt 组装 ──→ LLM 流式生成 ──→ SSE 返回
        │
        ▼
  引用快照 / 检索状态持久化（溯源）
```

## RAG 核心流程

### 1. 文档入库与异步处理

**上传入口：** `POST /api/v1/knowledge-bases/:id/files`

| 步骤 | 说明 |
|------|------|
| 格式校验 | 支持 `.md` `.txt` `.docx`，其余格式拒绝 |
| SHA-256 去重 | 同用户下相同内容文件不重复入库 |
| MinIO 存储 | 文件以对象形式存入 MinIO（bucket: `recallix`） |
| 创建记录 | `knowledges` 表写入记录，`parse_status = "pending"` |
| 入队 | 将 `document:process` 任务推入 Redis / Asynq 队列 |

**异步处理（Worker）：**

```
pending → processing → done / failed
```

Worker 并发从 Redis 拉取任务，执行：

1. **解析（Parser）** — `PlainParser` 处理 `.md`/`.txt`，`DocxParser` 提取 `.docx` 的 XML 文本
2. **分片（Chunker）** — 根据文档结构自动选择切分策略
3. **嵌入（Embedding）** — 调用嵌入模型（如 `text-embedding-v3`）将每个 chunk 转为 1024 维向量
4. **入库（Milvus + PostgreSQL）** — chunk 文本写入 PostgreSQL，向量写入 Milvus
5. 完成后 `parse_status = "done"`，失败则 `"failed"`

---

### 2. Chunk 分片策略

四种策略，由 `chunkAuto` 自动选择：

```
chunkAuto 决策顺序：
  有 # 标题 ?  ──→ heading 策略
  有 第X章 / 1. ? ──→ heuristic 策略
  以上都不满足 ──→ recursive 策略（兜底）
```

#### heading 策略
- 按 Markdown 标题层级（`#` ~ `######`）拆分文档为节
- 构建面包屑路径作为上下文头（如 `# 操作系统 > ## 进程与线程`）
- 每节内部递归切分，所有 chunk 携带 `ContextHeader` 标签

#### heuristic 策略
- 在章节标记处切分（`第X章`、`第X节`、`1.`、`1、`）
- 适用于无 Markdown 标题、但有中文序号结构的纯文本文档

#### recursive 策略
- `splitToUnits`：按分隔符优先级递归切分
  - `\n\n` → `\n` → `。` → `！` → `？` → `;` → `；`
- `mergeUnits`：以 512 字符为目标合并 unit，相邻 chunk 保留 80 字符重叠
- 分隔符穷尽后仍无法切分的超长段落，原样保留为单个 chunk

#### 配置默认值

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `ChunkSize` | 512 | 目标 chunk 大小（字符数） |
| `ChunkOverlap` | 80 | 相邻 chunk 之间的重叠字符数 |
| 分隔符序列 | 7 级 | `双换行 → 单换行 → 句号 → 感叹号 → 问号 → 分号英文 → 分号中文` |

---

### 3. 查询理解与检索控制

每次用户提问先经过 `query.Understand()` —— 单次 LLM 调用同时完成改写和意图分类。

**6 种意图：**

| 意图 | 含义 | 是否触发检索 |
|------|------|-------------|
| `kb_search` | 知识检索型问题 | ✅ |
| `clarification` | 模糊但知识型 | ✅ |
| `greeting` | 问候/寒暄 | ❌ |
| `chitchat` | 闲聊 | ❌ |
| `follow_up` | 仅依赖历史可回答 | ❌ |
| `summarize` | 总结对话本身 | ❌ |

**问题改写（Query Rewrite）：**
- 载入最近 6 条对话历史
- 让 LLM 将指代词、省略信息补全为自包含的检索查询
- 例如 `它和线程有什么区别` → `进程和线程有什么区别`

**检索控制流程：**

```
用户提问
  → 载入历史（最近 20 条消息）
  → 意图识别 + 问题改写
  → NeedsRetrieval()?
      ├─ false → 跳过检索，纯 LLM 回答
      └─ true  → 进入混合检索
```

---

### 4. 混合检索与重排序

**混合召回：** 向量检索 + 关键词检索融合

| 召回通道 | 实现 | 权重 |
|---------|------|------|
| 向量检索 | Milvus L2 距离搜索，限定 `user_id` + `knowledge_base_id` | 70% |
| 关键词检索 | PostgreSQL 词频匹配（BM25-like） | 30% |

两路分数归一化后加权求和，按融合分数降序排列。

**重排序（Rerank）：**

召回结果送入 Rerank 模型（如 `qwen3-rerank`）二次打分，更新每条结果的分数。

**分数阈值过滤（`filterRAGResults`）：**

```
① TopK 截断（默认 5）
     ↓
② ScoreThreshold 过滤（默认 0.45）
     │  └─ 有结果 → 返回
     └─ 无结果 → ③ 阈值降级重试
                    │  └─ 有结果 → 返回
                    └─ 无结果 → ④ FallbackMinScore 单条兜底
                                    │  └─ 满足 → 返回 Top 1
                                    └─ 不满足 → ⑤ 完全拒绝（不走 RAG）
```

**相关环境变量：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `RAG_TOP_K` | `5` | TopK 截断条数 |
| `RAG_SCORE_THRESHOLD` | `0.45` | 分数保留阈值 |
| `RAG_FALLBACK_MIN_SCORE` | `0` | 降级失败后的兜底最低分 |
| `RAG_THRESHOLD_DEGRADE_BY` | `0` | 阈值降级乘数（0 表示不降级） |
| `RAG_THRESHOLD_FLOOR` | `0` | 降级底线 |

---

### 5. 流式问答与结果溯源

**Prompt 组装（按顺序拼接）：**

```
系统指令
  +
知识库上下文（[Source N] chunk 正文，来自混合检索结果）
  +
用户长期记忆（来自 memory 表）
  +
对话历史（最近 20 条消息）
  +
当前问题
```

**SSE 流式响应（`POST /api/v1/sessions/:id/chat`）：**

| 事件类型 | 时机 | 内容 |
|---------|------|------|
| `references` | 回答前 | `retrieval_status` + 引用切片列表（含分数） |
| `answer` | 逐 token | LLM 生成的回答正文 |
| `stop` | 结束 | 流式完成标记 |
| `error` | 异常 | 错误信息 |

**结果溯源：**

| 持久化内容 | 存储位置 | 溯源能力 |
|-----------|---------|---------|
| 检索状态 | `messages.retrieval_status` | 每条回答标记 `skipped` / `hit` / `miss` |
| 引用快照 | `message_references` 表 | 保存引用切片的 `ContentSnapshot` + `ContextHeaderSnapshot`，原文件删除后仍可追溯 |

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端框架 | Go、Gin、GORM |
| 数据库 | PostgreSQL（pgvector） |
| 向量数据库 | Milvus |
| 对象存储 | MinIO |
| 任务队列 | Redis + Asynq |
| 大模型 | DeepSeek V4（对话）、DashScope（嵌入 + Rerank） |
| 前端 | React 19、TypeScript、Vite、Tailwind CSS 4 |
| 流式通信 | SSE（Server-Sent Events） |

---

## 快速开始

### 前提条件

- Go 1.23+
- Node.js 20+
- Docker

### 1. 配置环境变量

```bash
cp .env.example .env    # 如有 .env.example
# 编辑 .env，至少填入 LLM / Embedding / Rerank 的 API Key
```

本地开发时创建 `.env.local` 覆盖 Docker 服务地址：

```
DB_HOST=localhost
REDIS_ADDR=localhost:6379
MILVUS_HOST=localhost
MINIO_ENDPOINT=localhost:9000
```

### 2. 启动基础设施

```bash
make dev-start
```

启动 PostgreSQL、Redis、Milvus、MinIO（Docker 容器）。

### 3. 启动 API

```bash
make dev-app
# → http://localhost:8081
```

### 4. 启动前端

```bash
make dev-frontend
# → http://localhost:5173
```

### 5. 全 Docker 部署

```bash
docker compose up -d
# 前端 → http://localhost
# API  → http://localhost:8081
```

---

## API 端点（RAG 相关）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/knowledge-bases` | 创建知识库 |
| `GET` | `/api/v1/knowledge-bases` | 列出知识库 |
| `POST` | `/api/v1/knowledge-bases/:id/files` | 上传文档到知识库 |
| `GET` | `/api/v1/knowledges` | 列出文档记录 |
| `GET` | `/api/v1/knowledges/:id/content` | 查看文档解析内容 |
| `POST` | `/api/v1/sessions` | 创建对话会话 |
| `GET` | `/api/v1/sessions/recent` | 最近会话列表 |
| `GET` | `/api/v1/sessions/:id/messages` | 获取会话消息历史 |
| `POST` | `/api/v1/sessions/:id/chat` | 流式问答（SSE） |
| `GET` | `/api/v1/memories` | 查看长期记忆 |
| `DELETE` | `/api/v1/memories/:id` | 删除长期记忆 |
