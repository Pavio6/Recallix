import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { Loader2, AlertCircle } from 'lucide-react'
import { AuthLayout } from '../components/AuthLayout'

export default function RegisterPage() {
  const { register } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [nickname, setNickname] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

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
    const nicknameTrimmed = nickname.trim()
    if (!nicknameTrimmed) {
      setError('请输入用户名')
      return
    }
    if (!password) {
      setError('请输入密码')
      return
    }
    if (password.length < 6) {
      setError('密码至少6位')
      return
    }
    if (password !== confirmPassword) {
      setError('两次密码不一致')
      return
    }

    setLoading(true)
    try {
      await register(emailTrimmed, password, nicknameTrimmed)
      navigate('/login', { state: { email: emailTrimmed, password } })
    } catch (err: any) {
      const msg = err.response?.data?.error?.message
      if (msg && msg.includes('already')) {
        setError('该邮箱已被注册')
      } else {
        setError(msg || '注册失败，请稍后重试')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout>
            <h1 className="mb-8 text-center text-2xl font-bold text-gray-900 dark:text-white">Recallix</h1>

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
            <label className="block text-sm text-gray-700 dark:text-zinc-400 mb-1.5">用户名</label>
            <input
              type="text"
              value={nickname}
              onChange={(e) => { setNickname(e.target.value); setError('') }}
              className={`w-full px-3 py-2.5 bg-white dark:bg-zinc-900 border rounded-lg text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 focus:outline-none text-sm transition-colors ${
                error && !nickname.trim() ? 'border-red-400 dark:border-red-500/50 focus:border-red-500' : 'border-gray-200 dark:border-zinc-800 focus:border-gray-400 dark:focus:border-zinc-600'
              }`}
              placeholder="请输入用户名"
              autoComplete="nickname"
            />
          </div>

          <div>
            <label className="block text-sm text-gray-700 dark:text-zinc-400 mb-1.5">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => { setPassword(e.target.value); setError('') }}
              className={`w-full px-3 py-2.5 bg-white dark:bg-zinc-900 border rounded-lg text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 focus:outline-none text-sm transition-colors ${
                error && password.length < 6 ? 'border-red-400 dark:border-red-500/50 focus:border-red-500' : 'border-gray-200 dark:border-zinc-800 focus:border-gray-400 dark:focus:border-zinc-600'
              }`}
              placeholder="至少6位"
              autoComplete="new-password"
            />
          </div>

          <div>
            <label className="block text-sm text-gray-700 dark:text-zinc-400 mb-1.5">确认密码</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => { setConfirmPassword(e.target.value); setError('') }}
              className={`w-full px-3 py-2.5 bg-white dark:bg-zinc-900 border rounded-lg text-gray-900 dark:text-zinc-100 placeholder-gray-400 dark:placeholder-zinc-600 focus:outline-none text-sm transition-colors ${
                error && password !== confirmPassword ? 'border-red-400 dark:border-red-500/50 focus:border-red-500' : 'border-gray-200 dark:border-zinc-800 focus:border-gray-400 dark:focus:border-zinc-600'
              }`}
              placeholder="再次输入密码"
              autoComplete="new-password"
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
            注册
          </button>
            </form>

            <p className="mt-6 text-center text-sm text-gray-500 dark:text-zinc-500">
              已有账户？{' '}
              <Link to="/login" className="text-gray-700 underline underline-offset-2 hover:text-gray-900 dark:text-zinc-300 dark:hover:text-white">
                登录
              </Link>
            </p>
    </AuthLayout>
  )
}
