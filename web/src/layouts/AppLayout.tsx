import { useEffect, useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { LogOut, Book, MessageSquare, Plus, Loader2, Sun, Moon } from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useTheme } from '../hooks/useTheme'
import { sessionAPI } from '../services/api'

interface Session {
  id: string
  title: string
  updated_at: string
  message_count: number
  user_id: string
  created_at: string
}

export default function AppLayout() {
  const { user, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  const [sessions, setSessions] = useState<Session[]>([])
  const [loadingSessions, setLoadingSessions] = useState(false)

  const isChat = location.pathname.includes('/chat')
  const isKnowledge = location.pathname.includes('/knowledge')

  const loadSessions = async () => {
    setLoadingSessions(true)
    try {
      const { data } = await sessionAPI.listRecent()
      if (data.success) setSessions(data.data || [])
    } catch {
      setSessions([])
    } finally {
      setLoadingSessions(false)
    }
  }

  useEffect(() => {
    loadSessions()
  }, [])

  const handleNewSession = async () => {
    try {
      const { data } = await sessionAPI.create()
      if (data.success) {
        await loadSessions()
        navigate(`/app/chat?session=${data.data.id}`)
      }
    } catch {}
  }

  return (
    <div className="flex h-screen bg-gray-50 dark:bg-zinc-950 text-gray-900 dark:text-zinc-100">
      {/* Sidebar */}
      <aside className="w-64 flex flex-col border-r border-gray-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/50">
        {/* Brand */}
        <div className="p-5 border-b border-gray-200 dark:border-zinc-800">
          <h1 className="text-xl font-bold tracking-tight text-gray-900 dark:text-white">Recallix</h1>
          <p className="text-xs text-gray-400 dark:text-zinc-500 mt-1">Knowledge RAG Platform</p>
        </div>

        {/* Navigation */}
        <nav className="p-3 space-y-1">
          <button
            onClick={() => navigate('/app/chat')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors ${
              isChat
                ? 'bg-gray-100 dark:bg-zinc-800 text-gray-900 dark:text-white'
                : 'text-gray-500 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-zinc-200 hover:bg-gray-100 dark:hover:bg-zinc-800/50'
            }`}
          >
            <MessageSquare size={18} />
            <span>对话</span>
          </button>
          <button
            onClick={() => navigate('/app/knowledge')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors ${
              isKnowledge
                ? 'bg-gray-100 dark:bg-zinc-800 text-gray-900 dark:text-white'
                : 'text-gray-500 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-zinc-200 hover:bg-gray-100 dark:hover:bg-zinc-800/50'
            }`}
          >
            <Book size={18} />
            <span>知识库</span>
          </button>
        </nav>

        {/* Recent sessions */}
        <div className="flex-1 overflow-hidden flex flex-col px-3">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-gray-400 dark:text-zinc-500 uppercase tracking-wider">最近 7 天</span>
            <button
              onClick={handleNewSession}
              className="p-1 rounded hover:bg-gray-100 dark:hover:bg-zinc-800 text-gray-400 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-white transition-colors"
              title="新建会话"
            >
              <Plus size={16} />
            </button>
          </div>
          <div className="flex-1 overflow-y-auto space-y-0.5">
            {loadingSessions ? (
              <div className="flex justify-center py-6">
                <Loader2 size={20} className="animate-spin text-gray-400 dark:text-zinc-500" />
              </div>
            ) : sessions.length === 0 ? (
              <p className="text-xs text-gray-400 dark:text-zinc-600 py-4 text-center">暂无会话</p>
            ) : (
              sessions.map((s) => (
                <button
                  key={s.id}
                  onClick={() => navigate(`/app/chat?session=${s.id}`)}
                  className="w-full text-left px-3 py-2 rounded-lg text-sm text-gray-500 dark:text-zinc-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-zinc-800/50 transition-colors truncate"
                >
                  <span className="block truncate">{s.title || '新对话'}</span>
                  <span className="text-[10px] text-gray-400 dark:text-zinc-600">
                    {new Date(s.updated_at).toLocaleDateString()}
                  </span>
                </button>
              ))
            )}
          </div>
        </div>

        {/* User */}
        <div className="p-3 border-t border-gray-200 dark:border-zinc-800">
          <div className="flex items-center justify-between">
            <div className="min-w-0">
              <p className="text-sm text-gray-700 dark:text-zinc-300 truncate">{user?.nickname || user?.email}</p>
              <p className="text-xs text-gray-400 dark:text-zinc-600 truncate">{user?.email}</p>
            </div>
            <div className="flex items-center gap-0.5">
              <button
                onClick={toggleTheme}
                className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-zinc-800 text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 transition-colors"
                title={theme === 'dark' ? '切换亮色模式' : '切换暗色模式'}
              >
                {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
              </button>
              <button
                onClick={() => { logout(); navigate('/login') }}
                className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-zinc-800 text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 transition-colors"
                title="退出登录"
              >
                <LogOut size={16} />
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-hidden">
        <Outlet context={{ loadSessions }} />
      </main>
    </div>
  )
}
