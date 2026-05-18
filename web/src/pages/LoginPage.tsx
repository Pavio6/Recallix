import { useState, type FormEvent, useEffect } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { Loader2, AlertCircle } from 'lucide-react'
import { AuthLayout } from '../components/AuthLayout'

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const state = location.state as { email?: string; password?: string } | null

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (state?.email) {
      setEmail(state.email)
      setPassword(state.password || '')
      navigate(location.pathname, { replace: true, state: null })
    }
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    const emailTrimmed = email.trim()
    if (!emailTrimmed) {
      setError('请输入邮箱')
      return
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailTrimmed)) {
      setError('邮箱格式不正确')
      return
    }
    if (!password) {
      setError('请输入密码')
      return
    }

    setLoading(true)
    try {
      await login(emailTrimmed, password)
      navigate('/app/chat')
    } catch (err: any) {
      const msg = err.response?.data?.error?.message
      if (msg && (msg.includes('Invalid') || msg.includes('invalid') || msg.includes('email') || msg.includes('password'))) {
        setError('邮箱或密码错误')
      } else {
        setError(msg || '登录失败，请稍后重试')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout>
        <h1 className="text-2xl font-bold text-center text-gray-900 dark:text-white mb-2">Recallix</h1>
        <p className="text-sm text-gray-500 dark:text-zinc-500 text-center mb-8">登录你的账户</p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-700 dark:text-zinc-400 mb-1.5">邮箱</label>
            <input
              type="email"
              value={email}
              onChange={(e) => { setEmail(e.target.value); setError('') }}
              className={`w-full px-3 py-2.5 bg-white dark:bg-zinc-900 border rounded-lg text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 focus:outline-none text-sm transition-colors ${
                error && !email.trim() ? 'border-red-400 dark:border-red-500/50 focus:border-red-500' : 'border-gray-200 dark:border-zinc-800 focus:border-gray-400 dark:focus:border-zinc-600'
              }`}
              placeholder="your@email.com"
              autoComplete="email"
            />
          </div>

          <div>
            <label className="block text-sm text-gray-700 dark:text-zinc-400 mb-1.5">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => { setPassword(e.target.value); setError('') }}
              className={`w-full px-3 py-2.5 bg-white dark:bg-zinc-900 border rounded-lg text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 focus:outline-none text-sm transition-colors ${
                error && !password ? 'border-red-400 dark:border-red-500/50 focus:border-red-500' : 'border-gray-200 dark:border-zinc-800 focus:border-gray-400 dark:focus:border-zinc-600'
              }`}
              placeholder="******"
              autoComplete="current-password"
            />
          </div>

          {error && (
            <div className="flex items-start gap-2 bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 rounded-lg px-3 py-2.5">
              <AlertCircle size={16} className="text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-2.5 bg-gray-900 dark:bg-white text-white dark:text-black rounded-lg font-medium text-sm hover:bg-gray-800 dark:hover:bg-zinc-200 transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
          >
            {loading && <Loader2 size={16} className="animate-spin" />}
            登录
          </button>
        </form>

        <p className="text-center text-sm text-gray-500 dark:text-zinc-500 mt-6">
          还没有账户？{' '}
          <Link to="/register" className="text-gray-700 dark:text-zinc-300 hover:text-gray-900 dark:hover:text-white underline underline-offset-2">
            注册
          </Link>
        </p>
    </AuthLayout>
  )
}
