# RAG 提问链路

1. **接收提问请求并校验会话**【Gin / GORM】
   - 入口：会话聊天接口对应 `ChatService.Chat`。
   - 先按 `session_id + user_id` 查询会话，确认当前用户只能访问自己的会话。
   - 请求体只要求一个必填字段：`question`。

2. **保存用户消息**【GORM / relational DB】
   - 用户本轮问题会先写入 `messages` 表，角色为 `user`。
   - 这意味着后续查询改写和最终回答都能基于完整历史，而不是只看本轮输入。

3. **加载最近对话历史**【GORM】
   - 当前实现按时间倒序取最近 `20` 条消息，再反转成正序。
   - 这些历史会在后续两处使用：
     - 查询理解阶段，用于结合上下文把“它、这个、刚才第二点”这类省略说法改写成完整问题。
     - 最终回答阶段，作为模型的对话上下文。

4. **做查询理解：改写问题 + 判断意图**【LLM / Query Understanding】
   - 系统会额外调用一次聊天模型，把“问题改写”和“意图分类”放在同一次 LLM 请求中完成。
   - 该阶段最多只取最近 `6` 条历史消息参与理解。
   - 输出格式固定为 JSON：
     - `rewrite_query`：改写后的自包含问题。
     - `intent`：当前问题意图。
   - 当前支持的意图：
     - `kb_search`：知识库检索类问题。  
       例：`线程和进程有什么区别？`、`文档里对 Agent 的定义是什么？`
     - `greeting`：纯问候、感谢、告别。  
       例：`你好`、`谢谢你`
     - `chitchat`：不依赖知识库的普通闲聊。  
       例：`你是谁？`、`讲个笑话`
     - `follow_up`：只依赖已有对话历史即可回答的追问。  
       例：`把你刚才回答的第二点展开说说`、`那它的优点呢？`
     - `summarize`：总结当前对话本身。  
       例：`总结一下我们刚才聊了什么`、`回顾一下前面的结论`
     - `clarification`：问题本身有歧义或信息不足，但本质上仍属于知识查询。  
       例：`它的优缺点是什么？`、`这个方法和前一个有什么区别？`
   - 检索开关规则：
     - `greeting / chitchat / follow_up / summarize`：跳过知识库检索。
     - 其他情况，包括 `kb_search / clarification / 未知意图`：继续进入知识库检索。
   - 如果模型调用失败、返回为空或 JSON 解析失败，系统会回退为：
     - `rewrite_query = 原问题`
     - `intent = kb_search`
   - 这个回退策略偏保守：宁可多检索，也尽量避免把本该查知识库的问题误判成闲聊。

5. **召回长期记忆**【Embedding HTTP API / Milvus / GORM】
   - 系统会先对 `rewrite_query` 生成 embedding。
   - 再到 Milvus 的 `memory_collection` 中按 `user_id` 搜索与当前问题语义最相关的长期记忆，当前取 `topK = 3`。
   - 命中的 memory ID 会回表查询 `memories` 表，组装成 `【User Memory】` 系统消息。
   - 如果 Milvus 不可用、搜索失败或没有命中，则本轮不注入长期记忆上下文。

6. **选择知识库范围**【GORM】
   - 当前实现会查询该用户的所有知识库。
   - 如果存在知识库，只取列表中的**第一个知识库 ID** 作为本轮检索范围。
   - 也就是说，当前 RAG 不是“跨全部知识库检索”，也不是“按用户选择的知识库检索”，而是默认只查第一个知识库。

7. **决定是否进入 RAG 检索**【Intent Routing】
   - 如果第 4 步判定当前问题不需要检索，则直接跳过 RAG，进入回答阶段。
   - 如果需要检索且 `hybrid` 服务可用，才会执行知识库搜索。
   - 如果 `hybrid` 服务为空，本轮也无法做知识库检索。

8. **执行混合召回**【Embedding / Milvus / 倒排索引 / BM25】
   - 复用第 5 步已经生成的 `rewrite_query` embedding，用于知识库 chunk 检索。
   - 向量召回：
     - 在 Milvus 的 `chunk_collection` 中按 `user_id + knowledge_base_id` 检索。
     - 请求数量是 `topK * 2`；当前聊天侧传入 `topK = 10`，所以向量召回最多先取 `20` 条。
   - 关键词召回：
     - 通过 `chunk_term_postings` 倒排索引，直接定位包含查询词的 chunk，不再扫描最近 `500` 个 chunk。
     - 入库时建立 lexical index 的文本不是只有正文，而是 `ContextHeader + Content`；因此标题路径也能参与关键词命中。
     - 查询词会先经过轻量级 tokenizer：
       - 长度大于 `1` 的英文和数字词按词保留。
       - 中文连续文本按双字切片生成 token，使无空格中文也能参与关键词检索。
     - 关键词相关性使用 `BM25` 评分，综合考虑：
       - 查询词在当前 chunk 中出现的频率。
       - 查询词在整个语料中的稀有程度。
       - 当前 chunk 的长度。
   - 融合逻辑：
     - 向量结果和关键词结果会合并到同一候选集。
     - 当前权重固定为：`0.7 * vector_score + 0.3 * keyword_score`。
     - 最后按融合分数排序，并截断到当前传入的 `topK`。

9. **执行重排**【Rerank HTTP API】
   - 如果混合召回有结果，且 `rerank` 客户端可用，则对候选 chunk 再做一次重排。
   - 输入：`rewrite_query + 候选文档列表`。
   - 当前兼容两类 rerank 接口：
     - 通用接口格式。
     - 阿里云 DashScope 格式。
   - 如果 rerank 成功，候选结果的 `Score` 会被 rerank 分数覆盖。
   - 如果 rerank 失败，系统不会中断，而是继续使用混合召回阶段的原始分数。

10. **按阈值过滤最终上下文**【RAG Filter】
    - 先按分数降序排序，并截断到 `RAGTopK`，默认 `5` 条。
    - 默认只使用 `RAGScoreThreshold` 做过滤，当前默认 `0.45`。
    - 如果没有结果达到阈值，则本轮视为 RAG miss，不注入知识库上下文。
    - 代码仍保留可配置的降级阈值和 top1 fallback 能力，但默认关闭：
      - `RAGThresholdDegradeBy = 0`
      - `RAGThresholdFloor = 0`
      - `RAGFallbackMinScore = 0`
   - 当前默认策略更偏“宁可 miss，也不要把弱相关片段包装成正式依据”。

11. **构造最终提示词**【Prompt Engineering】
   - 系统提示词分两类：
     - 需要检索：使用普通 RAG system prompt，核心要求包括：
       1. 当前助手名为 `Recallix`，可以访问知识库。
       2. 有知识库上下文时，优先基于提供的上下文回答。
       3. 不知道时要明确说不知道，不要编造。
       4. 使用正确的 Markdown 格式输出。
       5. 回答要尽量简洁但完整。
       6. 如果提供了长期记忆，也要结合用户长期记忆作答。
     - 不需要检索：根据 `intent` 使用专门的 no-retrieval prompt：
       - `greeting`：自然、简短地回应问候 / 感谢 / 告别，不主动提知识库。
       - `chitchat`：按普通对话回答，不主动提知识库。
       - `follow_up`：只基于现有对话历史回答，不能声称自己检索过知识库。
       - `summarize`：只总结当前对话历史，不能声称自己检索过知识库。
       - 其他兜底情况：自然回答；除非确实提供了检索上下文，否则不能声称查过知识库。
   - 消息组装顺序：
      1. system prompt
      2. memory context（如果有）
      3. knowledge base context（如果有）
      4. 最近对话历史
   - 当前知识库上下文格式：
     - `【Knowledge Base Context】`
     - 后接 `[Source 1] ...`、`[Source 2] ...`
     - 如果 chunk 存在 `ContextHeader`，会先写标题路径，再写正文内容。

12. **先返回引用信息，再流式返回答案**【SSE / Streaming Chat】
    - 在正式生成答案前，服务端会先通过 SSE 发送一条 `references` 事件。
   - `retrieval_status` 共有三种：
     - `skipped`：本轮按意图跳过检索。
     - `hit`：本轮检索命中。
     - `miss`：本轮需要检索但最终未保留上下文。
   - 随后调用聊天模型的流式接口，模型产生一个 delta，就向前端推送一个 `answer` 事件。
   - 模型输出全部结束后，再发送 `stop` 事件标记本轮完成。

13. **保存助手回答与引用快照**【GORM / Transaction】
    - 助手完整回答会写入 `messages` 表，角色为 `assistant`。
    - 如果本轮有检索结果，会在同一个数据库事务中写入 `message_references`：
      - `chunk_id`
      - `knowledge_id`
      - `knowledge_base_id`
      - `rank`
      - `score`
      - `seq`
      - `context_header_snapshot`
      - `content_snapshot`
   - 这里保存的是“回答当时实际使用的证据快照”，即使以后 chunk 被重建或原文变化，历史回答仍能解释。

14. **更新会话元数据**【GORM】
    - 每轮回答后都会刷新 `session.updated_at`。
    - 如果会话标题仍是默认的“新对话”，系统会用首个问题的前 `50` 个 rune 作为标题。

15. **异步提取长期记忆**【Asynq / Redis / LLM / Embedding / Milvus】
    - 当前回答完成后，会投递一个 `memory:extract` 异步任务，而不是直接起 goroutine。
    - 任务 payload 包含：
      - `user_id`
      - `session_id`
      - `question`
      - `answer`
    - worker 消费任务后，先让模型抽取候选记忆，输出结构化 JSON：
      - `action = create / skip`
      - `memory_type = preference / profile / project / fact`
      - `memory_text`
    - 如果候选记忆不是 `skip`：
      1. 先对候选记忆生成 embedding。
      2. 到 `memory_collection` 中召回相似旧记忆。
      3. 如果存在相似旧记忆，再让模型判断最终动作：
         - `create`：新增一条记忆。
         - `update`：更新一条已有记忆。
         - `skip`：认为是重复或不值得保存。
      4. 最终将 memory 写入或更新到 `memories` 表，并同步维护 Milvus 中的 `memory_collection`。

## 当前链路里缺少的东西

1. **没有真正支持多知识库选择**
   - 当前只取用户的第一个知识库做检索，无法按用户当前选择的知识库检索，也无法跨多个知识库检索。

2. **引用结果已返回前端，但回答阶段没有显式要求逐条引用**
   - 当前 system prompt 只要求“基于上下文回答”，没有要求模型在正文中标注 `[Source N]` 或做逐条引用。
   - 因此前端能展示 references，但答案文本本身未必带可读引用。

3. **缺少更明确的低置信度应答策略**
   - 当 RAG miss 时，系统只是不给 context，但仍继续让模型回答。
   - 当前 prompt 虽然要求“不知道就说不知道”，但没有更强的业务级保护，例如强制拒答、追问澄清或明确提示“知识库未命中”。

4. **中文 tokenizer 仍是轻量级实现**
   - 当前为了保持依赖简单，中文使用的是双字切片，而不是真正的语义分词。
   - 它已经比“按空格切词”强很多，但在专有名词、长词和停用词控制上仍不如成熟中文分词方案。

## 当前实现里需要优先优化的点

1. **把多知识库策略做完整**
   - 最低限度：让请求显式传入 `knowledge_base_id`。
   - 更进一步：支持跨知识库检索、知识库筛选和前端可控范围。

2. **继续细化低置信度应答策略**
   - 对 `miss` 或 `clarification` 意图，可以考虑改为更明确的澄清 / 拒答 / 低置信度提示，而不是统一直接生成答案。
   - 如果未来重新开启 top1 fallback，也应把“弱命中”与“高置信命中”区分开，避免前端和回答策略把它们当成同一种结果。

3. **把中文 tokenizer 升级为更成熟的分词方案**
   - 当前双字切片适合 MVP，但后续可评估 `jieba` 一类分词器或自定义词典，以改善专有名词、领域词和停用词处理。


## 最终链路产物

- **用户问题**：`messages` 表中的 `user` 消息
- **查询理解结果**：运行时生成，不单独持久化
- **长期记忆上下文**：`memories` 表 + Milvus `memory_collection`
- **检索候选**：Milvus 向量召回 + BM25 关键词召回 + rerank
- **最终引用快照**：`message_references` 表
- **助手回答**：`messages` 表中的 `assistant` 消息
- **前端实时返回**：SSE `references / answer / stop`
