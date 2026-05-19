# 文档入库流程

1. **选择上传入口并发起请求**【React / HTML File API / Gin】
   - 前端提供两种入口：
     - **单文件上传**：`<input type="file">`，一次选择 1 个文件。
     - **文件夹上传**：`<input type="file" webkitdirectory>`，一次选择整个目录。
   - 前端会先过滤扩展名，只保留 `.md`、`.txt`、`.docx`。
   - 如果当前还没有知识库，前端会先自动创建一个“默认知识库”，再开始上传。
   - 文件夹上传本质上是**前端批量上传**：浏览器拿到文件列表后，前端逐个调用上传接口；后端接口本身仍然按“单文件请求”处理。
   - 后端入口：`POST /knowledge-bases/:id/files`，对应 `document.Handler.Upload`。
   - 后端收到每个文件后，先按 `user_id + knowledge_base_id` 校验知识库归属，避免跨用户写入。

2. **读取文件并做去重**【SHA-256】
   - 服务端一次性读取文件内容，同时计算 `sha256`。
   - 以 `file_hash + user_id` 做重复文件检查；同一用户重复上传同一内容时直接返回冲突。
   - 这里的去重范围是“用户级”，不是“知识库级”。

3. **保存原始文件**【MinIO】
   - 原文件写入 MinIO，不直接存数据库。
   - 对象键格式：`{userID}/{knowledgeBaseID}/{knowledgeID}{ext}`。
   - 数据库中保存的是 `minio://bucket/objectKey` 形式的 URI，后续解析和预览都从这里回读原文件。

4. **创建知识记录**【GORM / relational DB】
   - 新建 `knowledge` 记录，保存文件名、路径、哈希、大小、所属知识库等元数据。
   - 初始状态：`parse_status = pending`。
   - 状态流转：`pending -> processing -> done / failed`。

5. **投递异步入库任务**【Asynq + Redis】
   - 上传接口只负责把“文档已接收”这件事处理完，然后尽快返回；解析、切分、embedding、写向量库这些耗时操作不放在 HTTP 请求线程里做。
   - 项目使用 `Asynq` 作为异步任务框架，使用 `Redis` 作为任务队列后端：
     - `Redis` 负责保存待执行任务、重试任务和任务调度状态。
     - `Asynq` 负责把任务写入队列、从队列中取出任务、分发给 worker 执行，并提供失败重试等任务处理能力。
   - 任务实际流向：
     1. 业务代码通过 `Asynq Client` 创建任务。
     2. `Asynq Client` 把任务写入 `Redis` 中的队列。
     3. `Asynq Server / Worker` 从 `Redis` 队列里取出任务。
     4. `Worker` 执行对应的处理函数。
   - 当前上传成功后会创建一个 `document:process` 任务，payload 只保存 `knowledge_id`：
     - 不直接把文件内容塞进队列，避免 Redis 中任务体过大。
     - worker 拿到 `knowledge_id` 后，再去数据库查元数据、去 MinIO 读原文件。
   - 应用启动时会同时启动 Asynq worker，并注册 `document:process` 的处理函数；worker 的并发数由 `WORKER_CONCURRENCY` 控制。
   - 这套设计的直接效果：
     - 上传接口响应更快，不会被 DOCX 解析或 embedding 阻塞。
     - 多个文档可以排队后台处理。
     - 某些任务失败后，可以交给任务队列机制继续重试，而不是让用户重新上传。

6. **Worker 加载文档并进入处理态**【Asynq / MinIO】
   - Worker 根据 `knowledge_id` 查记录；如果已经是 `done`，直接跳过，避免重复处理。
   - 将状态改为 `processing`。
   - 再次按扩展名确认是否有可用解析器；不支持的格式直接标记 `failed`，并使用 `asynq.SkipRetry` 跳过重试。
   - 从 MinIO 读取原始文件字节流，进入统一解析阶段。

7. **统一解析为 Markdown 语义文本**【自定义 Parser / DOCX XML 解析】
   - `.md`、`.txt`：直接按文本读取。
   - `.docx`：把 DOCX 当作 ZIP 包读取，抽取：
     - `word/document.xml`
     - `word/styles.xml`
     - `word/numbering.xml`
   - DOCX 解析时会保留这些语义：
     - 标题：根据 `outlineLvl`、`Heading1~6`、`标题1~6` 等样式转成 `#` 到 `######`。
     - 列表：根据 `numbering.xml` 识别无序列表和有序列表，并保留层级缩进。
     - 分页/分节：遇到 page break 或 section break 时写入 `\f`，作为后续强分块边界。
   - 解析后的统一产物是 `MarkdownContent.Text`，后续 chunker 不再关心原文件格式。

8. **选择 chunk 策略**【自定义 Chunker】
   - 当前处理器固定使用：`chunker.DefaultConfig()` + `Strategy = auto`。
   - 默认参数：
     - `ChunkSize = 512`
     - `ChunkOverlap = 80`
     - `Separators = ["\n\n", "\n", "。", "！", "？", ";", "；"]`
   - `auto` 策略优先级：
     1. 文本中存在 Markdown 标题 `# ...`：走 `heading`。
     2. 否则若存在章节/编号迹象：走 `heuristic`。
     3. 前两者不可用或结果质量太差：回退到 `recursive`。
   - 质量兜底：如果结果很多、且超过一半 chunk 小于 10 个 rune，则判定切分结果不可靠，自动回退。

9. **chunk 规则细节**【Heading / Heuristic / Recursive】
   - `heading`：
     - 按 Markdown 标题切段。
     - 为每个 chunk 生成标题面包屑 `ContextHeader`，例如：`# 一级标题 > ## 二级标题`。
     - 每个标题段内部仍会继续递归切分，避免大章节过长。
   - `heuristic`：
     - 以强结构边界切分，包括：
       - `\f` 分页符
       - `---` 分隔线
       - `===` 分隔线
       - `第X章 / 第X节`
       - `1. / 1、 / 1)` 这类编号开头
       - 全大写英文标题行
   - `recursive`：
     - 按默认分隔符从强到弱递归拆分：先段落，再换行，再中文句号/感叹号/问号，再分号。
     - 当文本单元仍大于默认 512 rune 时，继续使用下一级分隔符拆小。
   - 合并与重叠规则：
     - 小单元会合并，直到不超过 `ChunkSize`。
     - 从第二个 chunk 开始，向前带入最多 `80` 个 rune 的重叠文本，增强上下文连续性。
     - `StartPos / EndPos` 都按 rune 偏移记录，而不是 byte 偏移，避免中文位置错乱。

10. **写入结构化 chunk**【GORM / relational DB】
    - 每个 chunk 会写入 `chunks` 表，字段包括：
      - `content`
      - `seq`
      - `start_pos / end_pos`
      - `context_header`
      - `chunk_type = text`
    - 真正送去做 embedding 的文本是：
      - 有标题上下文时：`ContextHeader + "\n\n" + Content`
      - 无标题上下文时：`Content`
    - 也就是说，数据库里保存的是正文 chunk，向量化时会把标题路径拼进去，提升召回语义。

11. **建立关键词倒排索引**【Sparse Retrieval / BM25】
    - chunk 落库后，会同步为它建立稀疏检索索引：
      - `chunk_lexical_indices`：保存每个 chunk 的文档长度等统计信息。
      - `chunk_term_postings`：保存 `term -> chunk` 的 posting，记录某个词在某个 chunk 中出现了多少次。
    - 中文关键词处理当前采用轻量级 tokenizer：
      - 长度大于 `1` 的英文和数字词会按词保留。
      - 中文连续文本按双字切片生成 token，使无空格中文也能被关键词检索命中。
    - 这样查询阶段不需要再全量扫描 chunk 正文，而是可以直接按词查 posting，并使用 BM25 计算关键词相关性。

12. **生成向量表示**【Embedding HTTP API】
    - 对每个 chunk 调用 embedding 服务的 `/embeddings` 接口。
    - 使用的模型名和 base URL 来自配置项，可对接 OpenAI 兼容接口。
    - 当前是逐条生成，不是批量并发生成。

13. **写入向量库**【Milvus】
    - 向量写入 `chunk_collection`。
    - 记录内容包括：`chunk_id`、`user_id`、`knowledge_base_id`、`embedding`。
    - Milvus 建的是 `HNSW + COSINE` 索引：
      - `HNSW`：通过图结构组织向量，查询时沿图逐步逼近最近邻；这是文本向量检索中很常见的默认选择，通常能在召回率和查询速度之间取得较好平衡。
      - `COSINE`：使用余弦相似度衡量两个向量方向是否接近，越接近 `1` 表示语义越相似；它也是文本 embedding 场景里更常见的度量方式。
    - embedding 维度来自配置，未配置时默认 `1024`。
    - 如果 Milvus 不可用，当前实现会跳过向量写入，但仍保留关系型数据库中的 chunk。

14. **完成处理并更新状态**【GORM】
    - 所有 chunk 循环结束后，只有在每个 chunk 都完整完成以下步骤时，文档才会被标记为 `done`：
      - 写入 `chunks`
      - 建立关键词倒排索引
      - 生成 embedding
      - 写入 Milvus
    - 如果“完全没有生成 chunk”，则会标记为 `failed`。
    - 如果任意一个 chunk 在写库、关键词索引、embedding 或向量写入任一步失败，当前实现会继续把本轮剩余 chunk 处理完，但最终将整篇文档标记为 `failed`。
    - 因此，当前 `done` 的语义是“整篇文档完整入库成功”，而不是“主流程已经跑完”。

## 最终入库结果

- **原始文件**：MinIO
- **文档元数据**：`knowledge` 表
- **切分后的文本块**：`chunks` 表
- **关键词稀疏索引**：`chunk_lexical_indices`、`chunk_term_postings`
- **向量索引**：Milvus `chunk_collection`
- **异步调度**：Asynq + Redis
- **统一文本中间表示**：Markdown 风格文本

## 当前实现里值得继续优化的点

1. **embedding 仍是逐条串行生成**
   - 当前每个 chunk 都单独请求一次 embedding 服务，文档较大时总耗时会被线性拉长。
   - 后续可以考虑：
     - 使用 embedding 接口的批量输入能力。
     - 或在可控并发下并行生成，再统一写入结果。

2. **chunk 处理仍是逐条写入，缺少批处理**
   - 当前每个 chunk 依次执行：写 `chunks`、建关键词索引、做 embedding、写 Milvus。
   - 这对 MVP 足够直观，但数据库往返和外部服务调用次数都偏多。
   - 后续可评估批量写库、批量 posting 写入、批量向量写入，以降低大文档入库成本。

3. **失败后的清理和重试还可以继续完善**
   - 当前只要任意关键步骤失败，整篇文档就会标记为 `failed`，这已经保证了状态语义是可靠的。
   - 但失败前已经成功写入的 chunk、关键词索引或向量，当前还不会自动回滚或清理。
   - 后续可以继续补：
     - 失败后的清理策略
     - 幂等重试
     - 重新入库前的旧数据删除 / 重建机制
   - 这样才能把“状态正确”进一步提升为“数据也始终一致”。

4. **关系库、关键词索引、向量库之间没有一致性补偿**
   - 当前三类存储是分步完成的：
     - `chunks`
     - lexical index
     - Milvus vector
   - 如果中间某一步失败，可能出现“正文已存在，但关键词或向量缺失”的不一致状态。
   - 后续可以考虑增加失败重试、补偿任务或可重建机制，而不是只依赖日志。

5. **DOCX 解析能力目前仍偏文本优先**
   - 当前实现主要抽取段落、标题、列表和分页/分节边界。
   - 表格、图片、复杂文本框、页眉页脚等内容还没有进入当前检索语义链路。
   - 如果后续文档类型更复杂，这会成为召回质量的上限。

6. **chunk 参数仍是全局固定值**
   - 当前 `ChunkSize = 512`、`ChunkOverlap = 80` 写在默认配置中，所有文档共用。
   - 后续可以按文档类型或业务场景做更细配置，例如：
     - 技术文档偏小块。
     - 长篇说明文偏大块。
     - DOCX 和 Markdown 使用不同策略参数。
