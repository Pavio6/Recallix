# RAG 文档入库流程

1. 上传文件
   用户上传文档，后端进入 `CreateKnowledgeFromFile`。

2. 创建知识记录
   校验知识库、文件类型和重复文件后，在数据库中创建 `Knowledge` 记录。

3. 保存原始文件
   将上传的文件保存到本地存储，并把 `filePath` 回写到 `Knowledge`。

4. 投递异步解析任务
   使用 Asynq 投递 `TypeDocumentProcess` 任务，后续由 worker 执行 `ProcessDocument`。

5. 读取并解析文件
   `ProcessDocument` 根据 `filePath` 读取原始文件，并统一得到 `MarkdownContent`。

6. 文档分块
   将 `MarkdownContent` 按分块规则切成多个 `ParsedChunk`。

7. 保存 chunks
   将 chunk 原文、顺序、位置、所属知识等信息保存到业务数据库的 `chunks` 表。

8. 向量化和索引写入
   调用 embedding 模型把 chunk 文本转成向量，并将向量和 `chunk_id` 写入检索引擎。

9. 更新知识状态
   入库完成后，将 `Knowledge` 更新为可用状态，后续即可被 RAG 检索使用。

# RAG 提问链路

1. 接收用户问题
   接口进入 `KnowledgeQA`，拿到用户问题、会话、知识库 ID、文件 ID 等参数。

2. 确定知识库和模型
   根据请求和会话配置，确定本次要检索的知识库、具体文件、聊天模型、重排模型等。

3. 构建 ChatManage
   把问题、知识库范围、检索参数、模型配置、会话信息统一放到 `ChatManage` 中。

4. 组装 RAG pipeline
   有知识库时，按 RAG 流程组装：历史加载、问题理解、检索、重排、合并、组装 prompt、流式回答。

5. 加载历史消息
   读取当前会话历史，用于多轮问答上下文。

6. 问题理解
   对用户问题做意图判断、改写或扩展，得到更适合检索的 `RewriteQuery`。

7. 并行检索 chunk
   根据知识库范围执行 chunk 检索，核心会调用 `HybridSearch`。

8. 生成问题向量
   使用知识库配置的 embedding 模型，将用户问题转成 query embedding。

9. 混合检索
   同时支持向量检索和关键词检索，召回候选 chunk。

10. 重排结果
    如果配置了 rerank 模型，会对候选 chunk 重新打分，选出更相关的结果。

11. 合并上下文
    对检索结果去重、合并相邻或重叠 chunk，并补充父 chunk 或邻近 chunk 作为上下文。

12. 过滤 TopK
    按配置保留最终要放进 prompt 的上下文数量。

13. 组装 Prompt
    将用户问题和检索到的 chunk 内容填入上下文模板，生成最终发给大模型的消息。

14. 调用聊天模型
    使用聊天模型进行流式生成，答案会通过事件流返回给前端。

15. 返回引用和答案
    检索到的 chunk 作为 references 返回，模型生成的内容作为最终回答返回。
