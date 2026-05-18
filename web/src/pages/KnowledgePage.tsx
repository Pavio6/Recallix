import { useState, useEffect, useRef, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Upload, FileText, Loader2, CheckCircle, AlertCircle, Clock, X, Eye, FolderUp } from 'lucide-react'
import { kbAPI, knowledgeAPI } from '../services/api'

interface KB {
  id: string
  name: string
  description: string
}

interface Knowledge {
  id: string
  file_name: string
  file_size: number
  parse_status: string
  created_at: string
  updated_at: string
}

function FileViewer({ knowledge, onClose }: { knowledge: Knowledge | null; onClose: () => void }) {
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const docxContainerRef = useRef<HTMLDivElement | null>(null)

  const normalizeMissingDocxPageLayout = (container: HTMLDivElement) => {
    container.querySelectorAll<HTMLElement>('section.docx').forEach((page) => {
      // Word 会为缺失的 pgSz / pgMar 使用默认页面参数；
      // docx-preview 只有在 DOCX 显式声明时才会写入对应 style。
      // 只补缺失值，避免覆盖原文自带的页面设置。
      if (!page.style.width) page.style.width = '210mm'
      if (!page.style.minHeight) page.style.minHeight = '297mm'
      if (!page.style.paddingLeft) page.style.paddingLeft = '25.4mm'
      if (!page.style.paddingRight) page.style.paddingRight = '25.4mm'
      if (!page.style.paddingTop) page.style.paddingTop = '25.4mm'
      if (!page.style.paddingBottom) page.style.paddingBottom = '25.4mm'
      page.style.boxSizing = 'border-box'
    })
  }

  useEffect(() => {
    if (!knowledge) return
    const fileName = knowledge.file_name.toLowerCase()
    const isDocx = fileName.endsWith('.docx')

    setLoading(true)
    setError('')
    setContent('')

    if (isDocx) {
      knowledgeAPI.getFile(knowledge.id)
        .then(async ({ data }) => {
          const { renderAsync } = await import('docx-preview')
          setLoading(false)
          await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
          if (!docxContainerRef.current) return
          docxContainerRef.current.innerHTML = ''
          await renderAsync(data, docxContainerRef.current, undefined, {
            inWrapper: true,
            ignoreWidth: false,
            ignoreHeight: false,
            ignoreFonts: false,
            breakPages: true,
            ignoreLastRenderedPageBreak: true,
            experimental: false,
            trimXmlDeclaration: true,
            useBase64URL: true,
          })
          normalizeMissingDocxPageLayout(docxContainerRef.current)
        })
        .catch(() => setError('加载 DOCX 原文预览失败'))
        .finally(() => setLoading(false))
      return
    }

    knowledgeAPI.getContent(knowledge.id)
      .then(({ data }) => {
        if (data.success) setContent(data.data.content)
        else setError(data.error?.message || '加载失败')
      })
      .catch(() => setError('加载文件内容失败'))
      .finally(() => setLoading(false))
  }, [knowledge?.id])

  if (!knowledge) return null

  const isMarkdown = knowledge.file_name.endsWith('.md')
  const isDocx = knowledge.file_name.endsWith('.docx')

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 dark:bg-black/60" onClick={onClose}>
      <div className={`bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-800 rounded-2xl w-[min(1100px,calc(100vw-48px))] flex flex-col shadow-2xl ${
        isDocx ? 'h-[90vh]' : 'max-w-3xl max-h-[80vh]'
      }`}
           onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-zinc-800">
          <div className="flex items-center gap-3 min-w-0">
            <FileText size={20} className="text-gray-400 dark:text-zinc-500 flex-shrink-0" />
            <h3 className="text-sm font-medium text-gray-900 dark:text-zinc-200 truncate">{knowledge.file_name}</h3>
          </div>
          <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-gray-100 dark:hover:bg-zinc-800 text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 transition-colors">
            <X size={18} />
          </button>
        </div>
        <div className={`flex-1 overflow-y-auto ${isDocx ? 'bg-gray-100 p-5 dark:bg-zinc-950' : 'p-5'}`}>
          {loading ? (
            <div className="flex justify-center py-12">
              <Loader2 size={24} className="animate-spin text-gray-400 dark:text-zinc-500" />
            </div>
          ) : error ? (
            <p className="text-red-500 dark:text-red-400 text-sm">{error}</p>
          ) : isMarkdown ? (
            <div className="markdown-body text-sm text-gray-700 dark:text-zinc-300 leading-relaxed
              [&_h1]:text-xl [&_h1]:font-bold [&_h1]:text-gray-900 [&_h1]:dark:text-zinc-100 [&_h1]:mb-3 [&_h1]:mt-6
              [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-gray-800 [&_h2]:dark:text-zinc-200 [&_h2]:mb-2 [&_h2]:mt-5
              [&_h3]:text-base [&_h3]:font-medium [&_h3]:text-gray-700 [&_h3]:dark:text-zinc-300 [&_h3]:mb-2 [&_h3]:mt-4
              [&_p]:mb-3 [&_p]:text-gray-700 [&_p]:dark:text-zinc-300
              [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:mb-3 [&_ul]:space-y-1
              [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:mb-3 [&_ol]:space-y-1
              [&_li]:text-gray-700 [&_li]:dark:text-zinc-300
              [&_strong]:text-gray-800 [&_strong]:dark:text-zinc-200 [&_strong]:font-semibold
              [&_em]:text-gray-500 [&_em]:dark:text-zinc-400
              [&_code]:bg-gray-100 [&_code]:dark:bg-zinc-800 [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-xs [&_code]:text-gray-800 [&_code]:dark:text-zinc-200 [&_code]:font-mono
              [&_pre]:bg-gray-50 [&_pre]:dark:bg-zinc-950 [&_pre]:border [&_pre]:border-gray-200 [&_pre]:dark:border-zinc-800 [&_pre]:rounded-xl [&_pre]:p-4 [&_pre]:mb-4 [&_pre]:overflow-x-auto [&_pre]:text-xs [&_pre]:text-gray-700 [&_pre]:dark:text-zinc-300
              [&_blockquote]:border-l-2 [&_blockquote]:border-gray-300 [&_blockquote]:dark:border-zinc-700 [&_blockquote]:pl-4 [&_blockquote]:italic [&_blockquote]:text-gray-500 [&_blockquote]:dark:text-zinc-400 [&_blockquote]:mb-3
              [&_a]:text-blue-600 [&_a]:dark:text-blue-400 [&_a]:underline
              [&_hr]:border-gray-200 [&_hr]:dark:border-zinc-800 [&_hr]:my-4
              [&_table]:w-full [&_table]:text-sm [&_table]:mb-4
              [&_th]:border [&_th]:border-gray-200 [&_th]:dark:border-zinc-700 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:text-gray-800 [&_th]:dark:text-zinc-200 [&_th]:bg-gray-50 [&_th]:dark:bg-zinc-900
              [&_td]:border [&_td]:border-gray-200 [&_td]:dark:border-zinc-800 [&_td]:px-3 [&_td]:py-2 [&_td]:text-gray-700 [&_td]:dark:text-zinc-300
            ">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
            </div>
          ) : isDocx ? (
            <div
              ref={docxContainerRef}
              className="docx-preview-host min-h-full overflow-x-auto text-black"
            />
          ) : (
            <pre className="text-sm text-gray-700 dark:text-zinc-300 whitespace-pre-wrap font-mono leading-relaxed">{content}</pre>
          )}
        </div>
      </div>
    </div>
  )
}

export default function KnowledgePage() {
  const [kbs, setKbs] = useState<KB[]>([])
  const [selectedKB, setSelectedKB] = useState<string>('')
  const [knowledges, setKnowledges] = useState<Knowledge[]>([])
  const [loading, setLoading] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState('')
  const [error, setError] = useState('')
  const [viewingFile, setViewingFile] = useState<Knowledge | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const folderRef = useRef<HTMLInputElement>(null)
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const loadKBs = async () => {
    try {
      const { data } = await kbAPI.list()
      if (data.success) {
        const list = data.data || []
        setKbs(list)
        if (list.length > 0 && !selectedKB) {
          setSelectedKB(list[0].id)
        } else if (list.length === 0) {
          setLoading(false)
        }
      } else {
        setLoading(false)
      }
    } catch {
      setLoading(false)
    }
  }

  const loadKnowledges = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      const { data } = await knowledgeAPI.list(selectedKB || undefined)
      if (data.success) {
        setKnowledges(data.data || [])
        // Stop polling if all files are done or failed
        const allSettled = (data.data || []).every(
          (k: Knowledge) => k.parse_status === 'done' || k.parse_status === 'failed'
        )
        if (allSettled && pollingRef.current) {
          clearInterval(pollingRef.current)
          pollingRef.current = null
        }
      }
    } catch {
      if (!silent) setKnowledges([])
    } finally {
      if (!silent) setLoading(false)
    }
  }, [selectedKB])

  useEffect(() => {
    loadKBs()
  }, [])

  useEffect(() => {
    if (selectedKB) loadKnowledges()
  }, [selectedKB])

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  const startPolling = () => {
    if (pollingRef.current) clearInterval(pollingRef.current)
    pollingRef.current = setInterval(() => loadKnowledges(true), 3000)
  }

  const uploadFiles = async (files: File[]) => {
    const ext = (name: string) => name.split('.').pop()?.toLowerCase() || ''
    const allowed = files.filter(f => ['md', 'txt', 'docx'].includes(ext(f.name)))

    if (allowed.length === 0) {
      setError('没有支持的文件类型，仅支持 .md, .txt, .docx')
      return
    }

    setError('')

    let kbId = selectedKB
    if (!kbId) {
      try {
        const { data } = await kbAPI.create({ name: '默认知识库' })
        if (data.success) {
          kbId = data.data.id
          setSelectedKB(kbId)
          setKbs([data.data])
        }
      } catch {
        setError('创建知识库失败')
        return
      }
    }

    setUploading(true)

    for (let i = 0; i < allowed.length; i++) {
      const file = allowed[i]
      if (allowed.length > 1) {
        setUploadProgress(`上传中 (${i + 1}/${allowed.length})`)
      }
      try {
        await kbAPI.uploadFile(kbId!, file)
      } catch (err: any) {
        const code = err.response?.data?.error?.code
        if (code !== 'CONFLICT') {
          setError(`"${file.name}" 上传失败: ${err.response?.data?.error?.message || '未知错误'}`)
        }
      }
    }

    setUploadProgress('')
    setUploading(false)
    await loadKnowledges()
    startPolling()

    if (fileRef.current) fileRef.current.value = ''
    if (folderRef.current) folderRef.current.value = ''
  }

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    await uploadFiles([file])
  }

  const handleFolderUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    if (files.length === 0) return
    await uploadFiles(files)
  }

  const formatTime = (dateStr: string) => {
    const d = new Date(dateStr)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  const statusIcon = (status: string) => {
    switch (status) {
      case 'done': return <CheckCircle size={16} className="text-green-500" />
      case 'processing': return <Loader2 size={16} className="animate-spin text-blue-500 dark:text-blue-400" />
      case 'pending': return <Clock size={16} className="text-yellow-500" />
      case 'failed': return <AlertCircle size={16} className="text-red-500" />
      default: return <Clock size={16} className="text-gray-400 dark:text-zinc-600" />
    }
  }

  const statusLabel = (status: string) => {
    switch (status) {
      case 'done': return '已解析'
      case 'processing': return '解析中'
      case 'pending': return '等待中'
      case 'failed': return '失败'
      default: return status
    }
  }

  return (
    <div className="flex flex-col h-full">
      <div className="px-6 py-4 border-b border-gray-200 dark:border-zinc-800 flex-shrink-0">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white">知识库</h2>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="max-w-3xl mx-auto space-y-8">
          {/* KB Selector */}
          {kbs.length > 0 && (
            <div className="flex items-center gap-4">
              <span className="text-sm text-gray-500 dark:text-zinc-400">当前知识库：</span>
              <select
                value={selectedKB}
                onChange={(e) => setSelectedKB(e.target.value)}
                className="bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-800 rounded-lg px-3 py-2 text-sm text-gray-900 dark:text-zinc-200 focus:outline-none focus:border-gray-300 dark:focus:border-zinc-600"
              >
                {kbs.map((kb) => (
                  <option key={kb.id} value={kb.id}>{kb.name}</option>
                ))}
              </select>
            </div>
          )}

          {/* Upload area */}
          <div className="border-2 border-dashed border-gray-200 dark:border-zinc-800 rounded-2xl p-8 text-center hover:border-gray-300 dark:hover:border-zinc-700 transition-colors">
            <input
              ref={fileRef}
              type="file"
              accept=".md,.txt,.docx"
              onChange={handleUpload}
              className="hidden"
            />
            <input
              ref={folderRef}
              type="file"
              {...({ webkitdirectory: '' } as any)}
              accept=".md,.txt,.docx"
              onChange={handleFolderUpload}
              className="hidden"
            />
            {uploading ? (
              <div className="flex flex-col items-center gap-3">
                <Loader2 size={32} className="animate-spin text-gray-400 dark:text-zinc-400" />
                <p className="text-sm text-gray-500 dark:text-zinc-500">
                  {uploadProgress || '上传中...'}
                </p>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-3">
                <div className="w-12 h-12 rounded-2xl bg-gray-100 dark:bg-zinc-900 flex items-center justify-center">
                  <Upload size={24} className="text-gray-400 dark:text-zinc-500" />
                </div>
                <div className="inline-flex items-center">
                  <button
                    type="button"
                    onClick={() => fileRef.current?.click()}
                    className="inline-flex items-center text-sm leading-none text-gray-500 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-white transition-colors"
                  >
                    点击上传文档
                  </button>
                  <span className="mx-2 inline-flex items-center text-gray-300 dark:text-zinc-700">|</span>
                  <button
                    type="button"
                    onClick={() => folderRef.current?.click()}
                    className="inline-flex items-center gap-1.5 text-sm leading-none text-gray-500 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-white transition-colors"
                  >
                    <FolderUp size={14} className="shrink-0" />
                    上传文件夹
                  </button>
                </div>
                <p className="text-xs text-gray-400 dark:text-zinc-600">支持 .md .txt .docx 格式</p>
              </div>
            )}
          </div>

          {error && (
            <div className="bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 rounded-xl px-4 py-3">
              <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}

          {/* Knowledge list */}
          <div>
            <h3 className="text-sm font-medium text-gray-500 dark:text-zinc-400 mb-3">文档列表</h3>
            {loading ? (
              <div className="flex justify-center py-8">
                <Loader2 size={24} className="animate-spin text-gray-400 dark:text-zinc-500" />
              </div>
            ) : knowledges.length === 0 ? (
              <div className="text-center py-8">
                <FileText size={32} className="text-gray-300 dark:text-zinc-700 mx-auto mb-3" />
                <p className="text-sm text-gray-400 dark:text-zinc-600">暂无文档，上传你的第一个文档吧</p>
              </div>
            ) : (
              <div className="space-y-2">
                {knowledges.map((k) => (
                  <div
                    key={k.id}
                    className="flex items-center justify-between px-4 py-3 bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-800 rounded-xl hover:border-gray-300 dark:hover:border-zinc-700 transition-colors cursor-pointer group"
                    onClick={() => k.parse_status === 'done' && setViewingFile(k)}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <FileText size={20} className="text-gray-400 dark:text-zinc-500 flex-shrink-0" />
                      <div className="min-w-0">
                        <p className="text-sm text-gray-900 dark:text-zinc-200 truncate">{k.file_name}</p>
                        <p className="text-xs text-gray-400 dark:text-zinc-600">
                          {formatSize(k.file_size)} · {formatTime(k.updated_at || k.created_at)}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      {statusIcon(k.parse_status)}
                      <span className="text-xs text-gray-500 dark:text-zinc-500">{statusLabel(k.parse_status)}</span>
                      {k.parse_status === 'done' && (
                        <Eye size={16} className="text-gray-300 dark:text-zinc-700 group-hover:text-gray-500 dark:group-hover:text-zinc-400 transition-colors ml-1" />
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      <FileViewer knowledge={viewingFile} onClose={() => setViewingFile(null)} />
    </div>
  )
}
