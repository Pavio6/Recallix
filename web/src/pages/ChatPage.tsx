import { Children, useState, useEffect, useRef, useCallback, type ComponentProps, type ReactNode } from 'react'
import { useSearchParams, useOutletContext } from 'react-router-dom'
import { Send, Loader2, BookOpen, ChevronDown } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { sessionAPI } from '../services/api'
import { streamChat, type RetrievalStatus } from '../services/stream'

const SAFE_BREAK_TOKEN = '[[RECALLIX_BR]]'

interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  references?: Reference[]
  retrievalStatus?: RetrievalStatus
  usedSkills?: SkillSummary[]
  isError?: boolean
}

interface Reference {
  chunkId: string
  content: string
  contextHeader?: string
  seq: number
  score: number
  knowledgeId: string
  knowledgeBaseId: string
}

interface SkillSummary {
  id: string
  name: string
  description?: string
  score?: number
}

function removeDanglingBoldMarkers(text: string): string {
  return text
    .split('\n')
    .map((line) => {
      const boldMarkerCount = line.match(/\*\*/g)?.length ?? 0
      if (boldMarkerCount % 2 === 0) return line

      const trimmed = line.trim()
      if (trimmed.startsWith('**')) {
        return line.replace('**', '')
      }
      if (trimmed.endsWith('**')) {
        const lastMarkerIndex = line.lastIndexOf('**')
        return line.slice(0, lastMarkerIndex) + line.slice(lastMarkerIndex + 2)
      }

      const lastMarkerIndex = line.lastIndexOf('**')
      return lastMarkerIndex === -1
        ? line
        : line.slice(0, lastMarkerIndex) + line.slice(lastMarkerIndex + 2)
    })
    .join('\n')
}

function cleanContent(text: string): string {
  const normalizedText = text
    // Preserve model-produced line breaks without enabling arbitrary HTML rendering.
    .replace(/<br\s*\/?>/gi, SAFE_BREAK_TOKEN)
    // Remove [Source N] markers
    .replace(/\[Source\s+\d+\]\s*/g, '')
    // Remove stray bold markers (standalone ** lines or line-ending **)
    .replace(/^\s*\*{2,3}\s*$/gm, '')
    // Fix concatenated table rows: split at | | boundaries between rows
    // Lookahead accepts space (data rows) and - (separator rows)
    .replace(/(\|\s+)(?=\|[-\s])/g, '$1\n')
    // Fix separator row concatenated with first data row
    .replace(/(\|[-\s|:]+\|)\s*(\|)/g, '$1\n$2')
    // Remove blank lines between table rows (breaks table rendering)
    .replace(/(\|[^\n]+\|)\n\n(\|[^\n])/g, '$1\n$2')

  return removeDanglingBoldMarkers(normalizedText)
}

function renderSafeBreaks(children: ReactNode, options?: { tableCell?: boolean }): ReactNode {
  return Children.map(children, (child) => {
    if (typeof child !== 'string') return child

    const parts = child.split(SAFE_BREAK_TOKEN)
    if (parts.length === 1) return child

    return parts.map((part, index) => (
      <span key={index}>
        {index > 0 && <br />}
        {options?.tableCell ? part.replace(/^\s*-\s+/, '• ') : part}
      </span>
    ))
  })
}

const markdownComponents = {
  p: ({ children, ...props }: ComponentProps<'p'>) => (
    <p {...props}>{renderSafeBreaks(children)}</p>
  ),
  li: ({ children, ...props }: ComponentProps<'li'>) => (
    <li {...props}>{renderSafeBreaks(children)}</li>
  ),
  td: ({ children, ...props }: ComponentProps<'td'>) => (
    <td {...props}>{renderSafeBreaks(children, { tableCell: true })}</td>
  ),
}

function normalizeReferences(refs: any[] = []): Reference[] {
  return refs.map((ref) => ({
    chunkId: ref.chunk_id ?? ref.ChunkID ?? '',
    content: ref.content_snapshot ?? ref.Content ?? '',
    contextHeader: ref.context_header_snapshot ?? ref.ContextHeader ?? '',
    seq: ref.seq ?? ref.Seq ?? 0,
    score: ref.score ?? ref.Score ?? 0,
    knowledgeId: ref.knowledge_id ?? ref.KnowledgeID ?? '',
    knowledgeBaseId: ref.knowledge_base_id ?? ref.KnowledgeBaseID ?? '',
  }))
}

function normalizeSkills(skills: any[] = []): SkillSummary[] {
  return skills.map((skill) => ({
    id: skill.id ?? skill.ID ?? '',
    name: skill.name ?? skill.Name ?? '',
    description: skill.description ?? skill.Description ?? '',
    score: skill.score ?? skill.Score,
  }))
}

export default function ChatPage() {
  const [searchParams] = useSearchParams()
  const { loadSessions } = useOutletContext<{ loadSessions: () => Promise<void> }>()
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [streamingContent, setStreamingContent] = useState('')
  const [streamingReferences, setStreamingReferences] = useState<Reference[]>([])
  const [streamingRetrievalStatus, setStreamingRetrievalStatus] = useState<RetrievalStatus | undefined>()
  const [streamingSkills, setStreamingSkills] = useState<SkillSummary[]>([])
  const [loadingMessages, setLoadingMessages] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const streamContentRef = useRef('')
  const streamReferencesRef = useRef<Reference[]>([])
  const streamRetrievalStatusRef = useRef<RetrievalStatus | undefined>(undefined)
  const streamSkillsRef = useRef<SkillSummary[]>([])
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const sendingRef = useRef(false)
  const isMultilineInput = input.includes('\n')

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(() => { scrollToBottom() }, [messages, streamingContent])

  const resizeTextarea = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea) return

    const maxHeight = 176 // roughly 8 lines before switching to internal scroll
    textarea.style.height = '0px'
    const nextHeight = Math.min(textarea.scrollHeight, maxHeight)
    textarea.style.height = `${nextHeight}px`
    textarea.style.overflowY = textarea.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }, [])

  useEffect(() => {
    resizeTextarea()
  }, [input, resizeTextarea])

  useEffect(() => {
    return () => {
      abortRef.current?.abort()
    }
  }, [])

  const loadMessages = useCallback(async (sid: string) => {
    setLoadingMessages(true)
    try {
      const { data } = await sessionAPI.getMessages(sid)
      if (data.success) {
        setMessages((data.data || []).map((msg: Message & { references?: any[]; retrieval_status?: RetrievalStatus }) => ({
          ...msg,
          references: normalizeReferences(msg.references),
          retrievalStatus: msg.retrieval_status,
          usedSkills: normalizeSkills((msg as any).used_skills),
        })))
      }
    } catch {
      setMessages([])
    } finally {
      setLoadingMessages(false)
    }
  }, [])

  useEffect(() => {
    const sid = searchParams.get('session')
    if (sid) {
      setSessionId(sid)
      loadMessages(sid)
    }
  }, [searchParams, loadMessages])

  const handleSend = useCallback(async () => {
    const question = input.trim()
    if (!question || sendingRef.current) return

    sendingRef.current = true
    setInput('')
    setSending(true)
    setStreamingContent('')
    setStreamingReferences([])
    setStreamingRetrievalStatus(undefined)
    setStreamingSkills([])
    streamContentRef.current = ''
    streamReferencesRef.current = []
    streamRetrievalStatusRef.current = undefined
    streamSkillsRef.current = []

    // Ensure session exists
    let sid = sessionId
    if (!sid) {
      try {
        const { data } = await sessionAPI.create()
        if (data.success) {
          sid = data.data.id
          setSessionId(sid)
          await loadSessions()
        } else return
      } catch { return }
    }

    // Add user message
    const userMsg: Message = { id: Date.now().toString(), role: 'user', content: question }
    setMessages((prev) => [...prev, userMsg])

    abortRef.current = streamChat(sid!, question, {
      onAnswer(content) {
        streamContentRef.current += content
        setStreamingContent((prev) => prev + content)
      },
      onReferences(refs, retrievalStatus) {
        const nextRefs = normalizeReferences(refs)
        streamReferencesRef.current = nextRefs
        streamRetrievalStatusRef.current = retrievalStatus
        setStreamingReferences(nextRefs)
        setStreamingRetrievalStatus(retrievalStatus)
      },
      onSkills(skills) {
        const nextSkills = normalizeSkills(skills)
        streamSkillsRef.current = nextSkills
        setStreamingSkills(nextSkills)
      },
      onError(msg) {
        console.error('Chat error:', msg)
        const errorMsg: Message = {
          id: (Date.now() + 2).toString(),
          role: 'assistant',
          content: `抱歉，本轮处理失败。\n\n${msg || '未知错误'}`,
          references: streamReferencesRef.current,
          retrievalStatus: streamRetrievalStatusRef.current,
          usedSkills: streamSkillsRef.current,
          isError: true,
        }
        setMessages((p) => [...p, errorMsg])
        setStreamingContent('')
        streamContentRef.current = ''
        sendingRef.current = false
        setSending(false)
        loadSessions()
      },
      onDone() {
        sendingRef.current = false
        setSending(false)
        const finalContent = streamContentRef.current
        streamContentRef.current = ''
        setStreamingContent('')
        if (finalContent) {
          const assistantMsg: Message = {
            id: (Date.now() + 1).toString(),
            role: 'assistant',
            content: finalContent,
            references: streamReferencesRef.current,
            retrievalStatus: streamRetrievalStatusRef.current,
            usedSkills: streamSkillsRef.current,
          }
          setMessages((p) => [...p, assistantMsg])
        }
        loadSessions()
      },
    })
  }, [input, sessionId, loadMessages, loadSessions])

  const renderReferences = (references?: Reference[], retrievalStatus?: RetrievalStatus) => {
    const refs = references || []
    if (retrievalStatus === 'skipped') {
      return null
    }
    if (retrievalStatus === 'miss' || (!retrievalStatus && refs.length === 0)) {
      return (
        <div className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-gray-50 px-2.5 py-1 text-xs text-gray-400 dark:bg-zinc-900 dark:text-zinc-500">
          <BookOpen size={12} />
          未命中知识库
        </div>
      )
    }

    return (
      <details className="mt-3 rounded-xl border border-gray-200 bg-gray-50/80 text-xs text-gray-600 dark:border-zinc-800 dark:bg-zinc-900/80 dark:text-zinc-400">
        <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-3 py-2">
          <span className="inline-flex items-center gap-1.5">
            <BookOpen size={12} />
            已参考知识库 · {refs.length} 个片段
          </span>
          <ChevronDown size={14} className="shrink-0" />
        </summary>
        <div className="space-y-2 border-t border-gray-200 px-3 py-3 dark:border-zinc-800">
          {refs.map((ref, index) => (
            <div key={`${ref.chunkId}-${index}`} className="rounded-lg bg-white p-2.5 dark:bg-zinc-950">
              <div className="mb-1 flex items-center justify-between gap-3">
                <span className="font-medium text-gray-600 dark:text-zinc-300">
                  来源 {index + 1}
                </span>
                <span className="text-gray-400 dark:text-zinc-500">
                  score {ref.score.toFixed(4)}
                </span>
              </div>
              {ref.contextHeader && (
                <div className="mb-1 text-gray-500 dark:text-zinc-400">
                  {ref.contextHeader}
                </div>
              )}
              <p className="line-clamp-3 leading-relaxed text-gray-500 dark:text-zinc-400">
                {ref.content}
              </p>
            </div>
          ))}
        </div>
      </details>
    )
  }

  const renderUsedSkills = (skills?: SkillSummary[]) => {
    if (!skills || skills.length === 0) return null
    return (
      <div className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-blue-50 px-2.5 py-1 text-xs text-blue-600 dark:bg-blue-950/40 dark:text-blue-300">
        本轮使用技能：{skills.map((skill) => skill.name).join('、')}
      </div>
    )
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className="flex flex-col h-full bg-white dark:bg-zinc-950">
      {/* Header */}
      <div className="px-6 py-4 border-b border-gray-200 dark:border-zinc-800 flex-shrink-0">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
              快速问答
            </h2>
            <p className="text-xs text-gray-400 dark:text-zinc-500">
              固定 RAG 链路，适合直接提问
            </p>
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto">
        {loadingMessages ? (
          <div className="flex justify-center py-12">
            <Loader2 size={24} className="animate-spin text-gray-400 dark:text-zinc-500" />
          </div>
        ) : messages.length === 0 && !streamingContent ? (
          <div className="flex flex-col items-center justify-center h-full text-center px-6">
            <h3 className="text-xl font-semibold text-gray-700 dark:text-zinc-300 mb-2">
              基于知识库内容问答
            </h3>
            <p className="text-sm text-gray-400 dark:text-zinc-600 max-w-md">
              上传文档到知识库后，直接在此提问即可获得智能回答
            </p>
          </div>
        ) : (
          <div className="max-w-3xl mx-auto px-6 py-4 space-y-6">
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[85%] rounded-2xl px-4 py-3 ${
                    msg.role === 'user'
                      ? 'bg-sky-100 text-sky-950 dark:bg-zinc-800 dark:text-zinc-100'
                      : msg.isError
                        ? 'border border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300'
                        : 'text-gray-700 dark:text-zinc-300'
                  }`}
                >
                  {msg.role === 'assistant' ? (
                    <>
                    <div className="prose-custom text-sm leading-relaxed
                      [&_h1]:text-lg [&_h1]:font-bold [&_h1]:text-gray-900 [&_h1]:dark:text-zinc-100 [&_h1]:mb-2 [&_h1]:mt-4
                      [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-gray-800 [&_h2]:dark:text-zinc-200 [&_h2]:mb-2 [&_h2]:mt-3
                      [&_h3]:text-sm [&_h3]:font-medium [&_h3]:text-gray-700 [&_h3]:dark:text-zinc-300 [&_h3]:mb-1 [&_h3]:mt-2
                      [&_p]:mb-2 [&_p]:text-gray-700 [&_p]:dark:text-zinc-300
                      [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:mb-2 [&_ul]:space-y-0.5
                      [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:mb-2 [&_ol]:space-y-0.5
                      [&_li]:text-gray-700 [&_li]:dark:text-zinc-300
                      [&_strong]:text-gray-800 [&_strong]:dark:text-zinc-200 [&_strong]:font-semibold
                      [&_code]:bg-gray-100 [&_code]:dark:bg-zinc-800 [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-xs [&_code]:text-gray-800 [&_code]:dark:text-zinc-200 [&_code]:font-mono
                      [&_pre]:bg-gray-50 [&_pre]:dark:bg-zinc-900 [&_pre]:border [&_pre]:border-gray-200 [&_pre]:dark:border-zinc-800 [&_pre]:rounded-xl [&_pre]:p-3 [&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:text-xs [&_pre]:text-gray-700 [&_pre]:dark:text-zinc-300
                      [&_blockquote]:border-l-2 [&_blockquote]:border-gray-300 [&_blockquote]:dark:border-zinc-700 [&_blockquote]:pl-3 [&_blockquote]:text-gray-500 [&_blockquote]:dark:text-zinc-400 [&_blockquote]:italic [&_blockquote]:mb-2
                      [&_a]:text-blue-600 [&_a]:dark:text-blue-400 [&_a]:underline
                      [&_hr]:border-gray-200 [&_hr]:dark:border-zinc-800 [&_hr]:my-3
                      [&_table]:w-full [&_table]:text-sm [&_table]:my-3
                      [&_th]:border [&_th]:border-gray-200 [&_th]:dark:border-zinc-700 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:text-gray-800 [&_th]:dark:text-zinc-200 [&_th]:bg-gray-50 [&_th]:dark:bg-zinc-900
                      [&_td]:border [&_td]:border-gray-200 [&_td]:dark:border-zinc-800 [&_td]:px-3 [&_td]:py-2 [&_td]:text-gray-700 [&_td]:dark:text-zinc-300
                    ">
                      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>{cleanContent(msg.content)}</ReactMarkdown>
                    </div>
                    {renderReferences(msg.references, msg.retrievalStatus)}
                    {renderUsedSkills(msg.usedSkills)}
                    </>
                  ) : (
                    <div className="text-sm whitespace-pre-wrap leading-relaxed">{msg.content}</div>
                  )}
                </div>
              </div>
            ))}

            {sending && !streamingContent && (
              <div className="flex justify-start">
                <div className="bg-gray-100 dark:bg-zinc-900 rounded-2xl px-5 py-4">
                  <div className="flex items-center gap-3">
                    <span className="text-xs text-gray-400 dark:text-zinc-500">思考中</span>
                    <span className="flex gap-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-gray-400 dark:bg-zinc-600 animate-bounce [animation-delay:0ms]" />
                      <span className="w-1.5 h-1.5 rounded-full bg-gray-400 dark:bg-zinc-600 animate-bounce [animation-delay:150ms]" />
                      <span className="w-1.5 h-1.5 rounded-full bg-gray-400 dark:bg-zinc-600 animate-bounce [animation-delay:300ms]" />
                    </span>
                  </div>
                </div>
              </div>
            )}

            {streamingContent && (
              <div className="flex justify-start">
                <div className="max-w-[85%] rounded-2xl px-4 py-3 text-gray-700 dark:text-zinc-300">
                  <div className="prose-custom text-sm leading-relaxed
                    [&_h1]:text-lg [&_h1]:font-bold [&_h1]:text-gray-900 [&_h1]:dark:text-zinc-100 [&_h1]:mb-2 [&_h1]:mt-4
                    [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-gray-800 [&_h2]:dark:text-zinc-200 [&_h2]:mb-2 [&_h2]:mt-3
                    [&_h3]:text-sm [&_h3]:font-medium [&_h3]:text-gray-700 [&_h3]:dark:text-zinc-300 [&_h3]:mb-1 [&_h3]:mt-2
                    [&_p]:mb-2 [&_p]:text-gray-700 [&_p]:dark:text-zinc-300
                    [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:mb-2 [&_ul]:space-y-0.5
                    [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:mb-2 [&_ol]:space-y-0.5
                    [&_li]:text-gray-700 [&_li]:dark:text-zinc-300
                    [&_strong]:text-gray-800 [&_strong]:dark:text-zinc-200 [&_strong]:font-semibold
                    [&_code]:bg-gray-100 [&_code]:dark:bg-zinc-800 [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-xs [&_code]:text-gray-800 [&_code]:dark:text-zinc-200 [&_code]:font-mono
                    [&_pre]:bg-gray-50 [&_pre]:dark:bg-zinc-900 [&_pre]:border [&_pre]:border-gray-200 [&_pre]:dark:border-zinc-800 [&_pre]:rounded-xl [&_pre]:p-3 [&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:text-xs [&_pre]:text-gray-700 [&_pre]:dark:text-zinc-300
                    [&_blockquote]:border-l-2 [&_blockquote]:border-gray-300 [&_blockquote]:dark:border-zinc-700 [&_blockquote]:pl-3 [&_blockquote]:text-gray-500 [&_blockquote]:dark:text-zinc-400 [&_blockquote]:italic [&_blockquote]:mb-2
                    [&_a]:text-blue-600 [&_a]:dark:text-blue-400 [&_a]:underline
                    [&_hr]:border-gray-200 [&_hr]:dark:border-zinc-800 [&_hr]:my-3
                    [&_table]:w-full [&_table]:text-sm [&_table]:my-3
                    [&_th]:border [&_th]:border-gray-200 [&_th]:dark:border-zinc-700 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:text-gray-800 [&_th]:dark:text-zinc-200 [&_th]:bg-gray-50 [&_th]:dark:bg-zinc-900
                    [&_td]:border [&_td]:border-gray-200 [&_td]:dark:border-zinc-800 [&_td]:px-3 [&_td]:py-2 [&_td]:text-gray-700 [&_td]:dark:text-zinc-300
                  ">
                    <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>{cleanContent(streamingContent)}</ReactMarkdown>
                  </div>
                  {renderReferences(streamingReferences, streamingRetrievalStatus)}
                  {renderUsedSkills(streamingSkills)}
                </div>
              </div>
            )}

            <div ref={messagesEndRef} />
          </div>
        )}
      </div>
      {/* Input */}
      <div className="px-6 py-4 border-t border-gray-200 dark:border-zinc-800 flex-shrink-0">
        <div className="max-w-3xl mx-auto">
          <div className={`flex gap-3 bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-800 rounded-2xl px-4 py-3 focus-within:border-gray-300 dark:focus-within:border-zinc-700 transition-colors ${
            isMultilineInput ? 'items-end' : 'items-center'
          }`}>
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入你的问题... (Enter 发送, Shift+Enter 换行)"
              rows={1}
              disabled={sending}
              className="flex-1 min-h-[24px] bg-transparent text-sm leading-6 text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 resize-none outline-none"
            />
            <button
              onClick={handleSend}
              disabled={!input.trim() || sending}
              className="p-2 rounded-xl bg-gray-900 dark:bg-white text-white dark:text-black hover:bg-gray-800 dark:hover:bg-zinc-200 transition-colors disabled:opacity-30 flex-shrink-0"
            >
              {sending ? <Loader2 size={18} className="animate-spin" /> : <Send size={18} />}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
