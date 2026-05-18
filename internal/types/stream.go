package types

type ResponseType string
type RetrievalStatus string

const (
	// ResponseTypeAnswer 表示模型正在流式返回回答正文。
	ResponseTypeAnswer     ResponseType = "answer"
	// ResponseTypeReferences 表示先返回本轮检索到的知识库引用片段。
	ResponseTypeReferences ResponseType = "references"
	// ResponseTypeThinking 预留给“思考中”一类的中间状态事件。
	ResponseTypeThinking   ResponseType = "thinking"
	// ResponseTypeError 表示本轮对话处理过程中发生错误。
	ResponseTypeError      ResponseType = "error"
	// ResponseTypeStop 表示本轮流式响应已经结束。
	ResponseTypeStop       ResponseType = "stop"

	// RetrievalStatusSkipped 表示本轮按意图判断跳过了知识库检索。
	RetrievalStatusSkipped RetrievalStatus = "skipped"
	// RetrievalStatusHit 表示本轮知识库检索命中，并保留了可用上下文。
	RetrievalStatusHit     RetrievalStatus = "hit"
	// RetrievalStatusMiss 表示本轮尝试检索，但最终没有保留可用片段。
	RetrievalStatusMiss    RetrievalStatus = "miss"
)

type StreamResponse struct {
	ID              string          `json:"id"`
	ResponseType    ResponseType    `json:"response_type"`
	Content         string          `json:"content"`
	Done            bool            `json:"done"`
	RetrievalStatus RetrievalStatus `json:"retrieval_status,omitempty"`
	References      interface{}     `json:"references,omitempty"`
	Data            interface{}     `json:"data,omitempty"`
}
