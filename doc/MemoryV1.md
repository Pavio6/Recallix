# MemoryV1 设计方案

## 1. 设计目标

这是一版适合面试讲解的 RAG + 记忆系统设计。

目标：

1. 支持多轮对话
2. 支持问题改写
3. 支持知识库检索、重排、合并、TopK 过滤
4. 支持父子 chunk
5. 支持长期记忆
6. 方案不能过度复杂，但要有完整工程思路

## 2. 技术架构

1. 后端服务
   - `Go`
   - 负责问答链路、记忆读写和整体业务编排

2. 业务数据库
   - `PostgreSQL`
   - 保存用户、会话、消息、chunk 原文、长期记忆原文等业务数据

3. 向量数据库
   - `Milvus`
   - 保存 chunk embedding 和 memory embedding，用于语义检索

4. 模型能力
   - `Embedding Model`：把 query、chunk、memory 转成向量
   - `Rerank Model`：对候选 chunk 重新排序
   - `Chat Model`：问题改写、记忆摘要、最终回答生成

## 3. RAG 提问主链路

1. 加载历史
   - 从 PostgreSQL 读取当前会话最近几轮消息
   - 保留多轮上下文

2. 问题理解
   - 使用 Chat Model 结合历史消息，对当前问题做改写
   - 输出：
     - `rewrite_query`
     - `intent`
   - `rewrite_query` 用于后续检索和重排
   - 如果识别成闲聊、问候等非检索意图，可以跳过知识库检索

3. 长期记忆召回
   - 将 `rewrite_query` 做 embedding
   - 到 Milvus 中检索当前用户最相关的 memory
   - 取少量高相关记忆加入后续 prompt

4. 知识库检索
   - 将 `rewrite_query` 做 embedding
   - 在 Milvus 中检索相似 child chunk
   - 同时可以增加关键词检索作为补充

5. 重排
   - 把召回的候选 chunk 交给 Rerank Model
   - 按“query 与 chunk 的真实相关性”重新排序

6. 合并
   - 去重
   - 合并相邻或重叠 chunk
   - 如果命中的是 child chunk，则找到对应 parent chunk，返回更完整上下文

7. 过滤 TopK
   - 按最终得分保留最相关的前 K 条上下文
   - 控制 prompt 长度，避免无关内容进入模型

8. 组装 Prompt
   - 将以下内容统一组装：
     - system prompt
     - 最近历史消息
     - 长期记忆
     - TopK 知识库上下文
     - 用户当前问题

9. 流式回答
   - 调用 Chat Model
   - 通过 SSE / WebSocket 流式返回答案

10. 回答后处理
    - 保存用户问题和助手回答到 PostgreSQL
    - 异步触发长期记忆写入

## 4. 长期记忆机制

这版设计不采用 Neo4j 图谱记忆，而采用更简单、面试也更容易讲清楚的：

```text
memory_text + embedding + metadata
```

### 4.1 什么时候写入记忆

1. 每轮回答完成后
2. 将本轮：
   - 用户问题
   - 助手回答
3. 交给 Chat Model 判断：
   - 是否值得长期保存
   - 如果值得，提炼成一条简洁 memory

例如：

```text
用户最近在学习 RAG，并重点关注文档切分。
```

### 4.2 memory 保存什么

PostgreSQL 中保存：

1. `memory_id`
2. `user_id`
3. `memory_text`
4. `memory_type`
   - preference
   - profile
   - project
   - fact
5. `importance`
6. `created_at`
7. `updated_at`

Milvus 中保存：

1. `memory_id`
2. `embedding`
3. `user_id`

### 4.3 什么时候召回记忆

1. 用户发来新问题后
2. 先得到 `rewrite_query`
3. 再对 `rewrite_query` 做 embedding
4. 到 Milvus 中按 `user_id` 过滤，召回最相关 memory
5. 取 TopK 后放进 prompt

### 4.4 为什么这样设计

1. 比 Neo4j 简单
2. 比纯关键词召回更稳
3. 可以跨会话记住用户长期偏好和项目状态
4. 业务数据和向量索引分离，职责清晰

## 5. Prompt 组成

最终给模型的上下文可以按这个顺序组织：

1. 系统提示词
2. 长期记忆
3. 最近历史对话
4. 检索到的知识库上下文
5. 当前用户问题

这样安排的原因：

1. 系统提示词约束整体行为
2. 长期记忆提供跨会话背景
3. 最近历史保证当前会话连续性
4. RAG 上下文提供事实依据
5. 当前问题放在最后，方便模型聚焦本轮任务

## 6. 面试时如何讲父子 chunk

可以这样说：

1. 如果只用小 chunk，检索会更准，但给模型的上下文可能不完整
2. 如果只用大 chunk，上下文完整，但检索粒度太粗
3. 所以我采用父子 chunk：
   - child chunk 负责检索
   - parent chunk 负责最终返回
4. 查询时先命中 child，再根据 `parent_chunk_id` 找回 parent

一句话：

```text
小块负责召回，大块负责阅读。
```

## 7. 面试时如何讲记忆机制

可以这样说：

1. 会话历史解决的是“当前会话内的连续性”
2. 长期记忆解决的是“跨会话仍然记得用户”
3. 每轮回答结束后，不直接保存整段聊天，而是提炼出值得长期保留的 memory
4. memory 原文放 PostgreSQL，embedding 放 Milvus
5. 下一轮提问时，先根据 query 召回相关 memory，再和 RAG 上下文一起放进 prompt

## 8. 相比复杂方案的取舍

1. 没有采用 Neo4j
   - 因为普通聊天长期记忆，更核心的是“语义召回”，不是复杂关系推理

2. 没有做过多 Agent 化
   - 先保证标准 RAG 问答链路稳定

3. 保留了关键工程能力
   - 历史加载
   - 问题改写
   - 检索
   - 重排
   - 合并
   - TopK
   - 父子 chunk
   - 长期记忆

## 9. 一句话总结

这版系统采用：

```text
PostgreSQL + Milvus + LLM
```

问答侧使用历史加载、问题改写、检索、重排、合并、TopK、Prompt 组装和流式生成；
记忆侧使用“memory_text + embedding”实现跨会话长期记忆。
