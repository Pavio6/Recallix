# 向量数据库迁移方案：Milvus → pgvector

## 背景

当前系统使用 Milvus 作为向量数据库，PostgreSQL 已使用 `pgvector/pgvector:pg17` 镜像，具备向量扩展能力。将向量存储迁移至 pgvector，可减少一个外部服务依赖，简化部署，同时利用 PG 事务实现向量与元数据的原子写入。

## 涉及文件清单

| 文件 | 当前状态 | 迁移后 |
|------|---------|--------|
| `internal/retrieval/vectorstore/milvus.go` | Milvus 实现 | 保留接口定义，改为 pgvector 实现 |
| `internal/retrieval/vectorstore/store.go` | 不存在 | **新建** — VectorStore 接口定义 |
| `internal/retrieval/vectorstore/pgvector.go` | 不存在 | **新建** — pgvector 实现 |
| `internal/app/app.go` | 初始化 MilvusStore | 改为 pgvector store |
| `internal/config/config.go` | 含 MilvusHost/MilvusPort | 去掉这两个字段 |
| `internal/document/processor/processor.go` | 依赖 `*vectorstore.MilvusStore` | 改为接口 `vectorstore.VectorStore` |
| `internal/memory/memory.go` | 同上 | 同上 |
| `internal/retrieval/hybrid/hybrid.go` | 同上 | 同上 |
| `internal/repository/db.go` | AutoMigrate | 新增 pgvector 扩展启用 |
| `docker-compose.yml` | 含 milvus 服务 | 去掉 milvus 服务及卷 |
| `Makefile` | 含 MILVUS_HOST | 去掉 |
| `.env` | 含 MILVUS_HOST / MILVUS_PORT | 去掉 |
| `.env.local` | 含 MILVUS_HOST | 去掉 |

## 实施步骤

---

### 第 1 步：定义 VectorStore 接口

**文件：** `internal/retrieval/vectorstore/store.go`（新建）

```go
package vectorstore

import "context"

type VectorStore interface {
    InsertChunk(ctx context.Context, rec VectorRecord) error
    InsertMemory(ctx context.Context, rec VectorRecord) error
    SearchChunks(ctx context.Context, userID, kbID string, embedding []float32, topK int) ([]SearchResult, error)
    SearchMemories(ctx context.Context, userID string, embedding []float32, topK int) ([]SearchResult, error)
    Close() error
}

// VectorRecord, SearchResult 结构体保持不变，从 milvus.go 移到此文件
```

**要点：**
- `VectorRecord` 和 `SearchResult` 从 `milvus.go` 搬到 `store.go`
- 接口方法增加 `context.Context` 参数，规范超时控制
- 不定义 Delete 方法（后续按需添加）

---

### 第 2 步：实现 pgvector 存储

**文件：** `internal/retrieval/vectorstore/pgvector.go`（新建）

**2.1 结构体定义**

```go
type PGStore struct {
    db  *gorm.DB
    dim int
}
```

**2.2 初始化**

```go
func NewPGStore(db *gorm.DB, dim int) (*PGStore, error) {
    // 1. CREATE EXTENSION IF NOT EXISTS vector
    db.Exec("CREATE EXTENSION IF NOT EXISTS vector")

    // 2. 创建 chunk_embeddings 表
    db.Exec(`
        CREATE TABLE IF NOT EXISTS chunk_embeddings (
            chunk_id         VARCHAR(64) PRIMARY KEY,
            user_id          VARCHAR(64) NOT NULL,
            knowledge_base_id VARCHAR(64) NOT NULL DEFAULT '',
            embedding        vector(1024) NOT NULL
        )
    `)

    // 3. 创建 memory_embeddings 表
    db.Exec(`
        CREATE TABLE IF NOT EXISTS memory_embeddings (
            memory_id VARCHAR(64) PRIMARY KEY,
            user_id   VARCHAR(64) NOT NULL,
            embedding vector(1024) NOT NULL
        )
    `)

    // 4. 创建 HNSW 索引（向量维度由 dim 决定，此处示例为 1024）
    //    首次创建较慢，后续插入自动维护
    db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_chunk_embeddings_embedding
        ON chunk_embeddings
        USING hnsw (embedding vector_cosine_ops)
    `)
    db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_memory_embeddings_embedding
        ON memory_embeddings
        USING hnsw (embedding vector_cosine_ops)
    `)

    return &PGStore{db: db, dim: dim}, nil
}
```

**2.3 InsertChunk**

```go
func (s *PGStore) InsertChunk(ctx context.Context, rec VectorRecord) error {
    vec := float32ToPGVector(rec.Embedding)
    return s.db.WithContext(ctx).Exec(`
        INSERT INTO chunk_embeddings (chunk_id, user_id, knowledge_base_id, embedding)
        VALUES (?, ?, ?, ?::vector)
        ON CONFLICT (chunk_id) DO UPDATE SET embedding = EXCLUDED.embedding
    `, rec.ChunkID, rec.UserID, rec.KnowledgeBaseID, vec).Error
}
```

**2.4 InsertMemory**（同理，写入 `memory_embeddings` 表）

**2.5 SearchChunks — 向量检索核心逻辑**

```go
func (s *PGStore) SearchChunks(ctx context.Context, userID, kbID string, embedding []float32, topK int) ([]SearchResult, error) {
    vec := float32ToPGVector(embedding)

    query := `
        SELECT chunk_id, 1 - (embedding <=> ?::vector) AS score
        FROM chunk_embeddings
        WHERE user_id = ?
    `
    args := []interface{}{vec, userID}

    if kbID != "" {
        query += ` AND knowledge_base_id = ?`
        args = append(args, kbID)
    }

    query += ` ORDER BY embedding <=> ?::vector LIMIT ?`
    args = append(args, vec, topK)

    // 执行查询，返回 []SearchResult
    // ...
}
```

**关键说明：**
- `<=>` 是 pgvector 的余弦距离运算符
- `1 - 余弦距离 = 余弦相似度`，值域 [0, 1]，越大越相似
- 与 Milvus 的 `COSINE` 距离类型语义一致

**2.6 SearchMemories**（同理，查询 `memory_embeddings` 表）

**2.7 float32 → pgvector 字符串转换**

```go
func float32ToPGVector(v []float32) string {
    parts := make([]string, len(v))
    for i, f := range v {
        parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
    }
    return "[" + strings.Join(parts, ",") + "]"
}
```

---

### 第 3 步：清理旧 Milvus 代码

**文件：** `internal/retrieval/vectorstore/milvus.go`

| 操作 | 内容 |
|------|------|
| **保留** | `VectorRecord`、`SearchResult` 结构体 → 搬到 `store.go` |
| **删除** | 整个 `MilvusStore` 结构体及所有方法 |
| **删除** | `milvus-sdk-go` 相关 import |

此文件可完全删除，或在保留 `VectorRecord`/`SearchResult` 搬到 `store.go` 后删除。

---

### 第 4 步：更新 config.go

**文件：** `internal/config/config.go`

| 操作 | 内容 |
|------|------|
| 删除 | `MilvusHost string` |
| 删除 | `MilvusPort string` |
| 删除 | `getEnv("MILVUS_HOST", ...)` |
| 删除 | `getEnv("MILVUS_PORT", ...)` |

**不需要新增字段** — pgvector 直接使用已有的 `DB*` 配置。

---

### 第 5 步：更新 app.go 初始化逻辑

**文件：** `internal/app/app.go`

**5.1 结构体字段**

```diff
- Milvus     *vectorstore.MilvusStore
+ VectorStore vectorstore.VectorStore
```

**5.2 初始化代码**

```diff
- var vs *vectorstore.MilvusStore
- vs, err = vectorstore.New(cfg)
- if err != nil {
-     log.Printf("WARNING: Failed to connect to Milvus: %v ...", err)
-     vs = nil
- }

+ vs, err := vectorstore.NewPGStore(db, cfg.EmbeddingDimension)
+ if err != nil {
+     log.Printf("WARNING: Failed to initialize pgvector: %v ...", err)
+     vs = nil
+ }
```

**5.3 所有类型引用**

将文件中所有 `*vectorstore.MilvusStore` 替换为 `vectorstore.VectorStore`。

---

### 第 6 步：更新调用方类型签名

**三个文件的函数签名变更：**

| 文件 | 当前 | 迁移后 |
|------|------|--------|
| `processor/processor.go:29` | `vs *vectorstore.MilvusStore` | `vs vectorstore.VectorStore` |
| `memory/memory.go:21` | `vs *vectorstore.MilvusStore` | `vs vectorstore.VectorStore` |
| `hybrid/hybrid.go:28` | `vs *vectorstore.MilvusStore` | `vs vectorstore.VectorStore` |

**接口方法签名变更：**

所有调用方的方法调用需要加上 `ctx` 参数：

```diff
- dp.vs.InsertChunk(rec)
+ dp.vs.InsertChunk(ctx, rec)

- m.vs.InsertMemory(rec)
+ m.vs.InsertMemory(ctx, rec)

- s.vs.SearchChunks(userID, kbID, emb, topK)
+ s.vs.SearchChunks(ctx, userID, kbID, emb, topK)
```

注意：`processor.go` 中在 `HandleDocumentProcess(t *asynq.Task)` 里调用，需用 `context.Background()` 或从 task 中提取 context。

---

### 第 7 步：数据库迁移

**文件：** `internal/repository/db.go`

在 `AutoMigrate` 或 `NewDB` 中启用扩展：

```go
func AutoMigrate(db *gorm.DB) error {
    // 启用 pgvector 扩展
    db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
    
    return db.AutoMigrate(
        // ... 已有模型
    )
}
```

`chunk_embeddings` 和 `memory_embeddings` 表由 `NewPGStore` 自动创建，不需要在 AutoMigrate 中声明。

---

### 第 8 步：清理部署配置

**docker-compose.yml：**

```diff
- milvus:
-     image: milvusdb/milvus:v2.5.4
-     ...
- milvusdata:

  api:
      depends_on:
          ...
-         milvus:
-             condition: service_healthy
```

**Makefile：**

```diff
  dev-start:
-     docker compose up -d postgres redis milvus minio minio-init
+     docker compose up -d postgres redis minio minio-init

  dev-app:
-     DB_HOST=localhost ... MILVUS_HOST=localhost ... go run ./cmd/api
+     DB_HOST=localhost ... go run ./cmd/api
```

**.env：**

```diff
- # Milvus
- MILVUS_HOST=milvus
- MILVUS_PORT=19530
```

**.env.local：**

```diff
- MILVUS_HOST=localhost
```

**go.mod：**

```diff
- github.com/milvus-io/milvus-sdk-go/v2
```

运行 `go mod tidy` 清理依赖。

---

### 第 9 步：数据迁移（可选）

如果已有历史向量数据需要保留：

```sql
-- 导出 Milvus 数据后，批量插入 pgvector
INSERT INTO chunk_embeddings (chunk_id, user_id, knowledge_base_id, embedding)
SELECT c.id, c.user_id, k.knowledge_base_id, <从 Milvus 导出的向量>
FROM chunks c
JOIN knowledges k ON c.knowledge_id = k.id
WHERE ...;
```

当前测试阶段数据量极小，建议**直接清理重建**（`make clean && docker compose down -v`），上传文档重新处理即可。

---

## 迁移前后对比

| 维度 | 迁移前 | 迁移后 |
|------|--------|--------|
| 向量库数量 | 2 个（PG + Milvus） | 1 个（PG） |
| Docker 容器 | +1（Milvus ~2GB 内存） | 无额外容器 |
| 事务保证 | 跨库，无原子性 | 同库事务，向量 + 元数据原子写入 |
| 检索性能 | 优秀（专业向量库） | 良好（HNSW 索引，万级数据无差异） |
| 部署 | 需要启动 Milvus + 健康检查 | 无需额外步骤 |
| 外部依赖 | `milvus-sdk-go/v2` | 仅 `pgvector` 扩展（镜像已含） |
| 代码量 | ~212 行 milvus.go | ~150 行 pgvector.go + ~25 行 store.go |

## 风险与回滚

| 风险 | 措施 |
|------|------|
| pgvector 性能不足 | 当前数据量极小（< 1000 条），HNSW 索引完全够用。万级后若感觉慢，可调整 `m` 和 `ef_construction` 参数 |
| pgvector 扩展未启用 | `CREATE EXTENSION IF NOT EXISTS vector` 幂等，已在 `AutoMigrate` 和 `NewPGStore` 两处兜底 |
| HNSW 索引构建慢 | 索引在空表上创建，后续插入自动维护，无初始构建开销 |
| 回滚到 Milvus | 保留 `milvus.go` 的 git 历史即可还原。接口抽象后，新实现只需满足 `VectorStore` 接口 |

---

## 执行顺序

```
1. 创建 store.go         — 接口 + 数据结构定义
2. 创建 pgvector.go      — pgvector 实现
3. 更新 config.go        — 删除 Milvus 配置
4. 更新 app.go           — 初始化 pgvector store
5. 更新 processor.go     — 改类型签名 + ctx
6. 更新 memory.go        — 改类型签名 + ctx
7. 更新 hybrid.go        — 改类型签名 + ctx
8. 更新 db.go            — 启用 pgvector 扩展
9. 删除 milvus.go        — 清理旧实现
10. 更新 docker-compose  — 去掉 milvus 服务
11. 更新 Makefile        — 去掉 MILVUS_HOST
12. 更新 .env / .env.local — 去掉 Milvus 配置
13. go mod tidy          — 清理依赖
14. 构建验证             — go build ./...
15. 功能验证             — 上传文档 → 问答 → 检查检索结果
```
