import { useEffect, useState } from 'react'
import { AlertCircle, CheckCircle2, Download, Ellipsis, Lightbulb, Loader2, Trash2 } from 'lucide-react'
import { agentAPI } from '../services/api'

interface Skill {
  id: string
  name: string
  description: string
  source_repo: string
  source_ref: string
  source_path: string
  file_count: number
  agent_count: number
}

export default function SkillsPage() {
  const [skills, setSkills] = useState<Skill[]>([])
  const [githubUrl, setGithubUrl] = useState('')
  const [loading, setLoading] = useState(false)
  const [deletingId, setDeletingId] = useState('')
  const [openMenuId, setOpenMenuId] = useState('')
  const [pendingDelete, setPendingDelete] = useState<Skill | null>(null)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const load = async () => {
    const { data } = await agentAPI.listSkills()
    if (data.success) setSkills(data.data || [])
  }
  useEffect(() => { load() }, [])
  useEffect(() => {
    if (!success) return
    const timer = window.setTimeout(() => setSuccess(''), 2000)
    return () => window.clearTimeout(timer)
  }, [success])
  useEffect(() => {
    if (!error) return
    const timer = window.setTimeout(() => setError(''), 2000)
    return () => window.clearTimeout(timer)
  }, [error])

  const importSkill = async () => {
    if (!githubUrl.trim()) return
    setError('')
    setSuccess('')
    setLoading(true)
    try {
      const { data } = await agentAPI.importSkill({ github_url: githubUrl.trim() })
      if (!data.success) {
        setError(data.error?.message || '导入失败，请稍后重试')
        return
      }
      setGithubUrl('')
      setSuccess(`已导入 ${data.data?.name || 'Skill'}`)
      await load()
    } catch (err: any) {
      setError(err.response?.data?.error?.message || '导入失败，请稍后重试')
    } finally {
      setLoading(false)
    }
  }

  const deleteSkill = async (skill: Skill) => {
    setOpenMenuId('')
    setError('')
    setSuccess('')
    setDeletingId(skill.id)
    try {
      const { data } = await agentAPI.deleteSkill(skill.id)
      if (!data.success) {
        setError(data.error?.message || '删除失败，请稍后重试')
        return
      }
      setSkills((prev) => prev.filter((item) => item.id !== skill.id))
      setSuccess(`已删除 ${skill.name}`)
    } catch (err: any) {
      setError(err.response?.data?.error?.message || '删除失败，请稍后重试')
    } finally {
      setDeletingId('')
      setPendingDelete(null)
    }
  }

  return (
    <div className="h-full overflow-y-auto bg-white p-8 dark:bg-zinc-950">
      <div className="mx-auto max-w-5xl">
        <div className="mb-8">
          <h2 className="text-2xl font-semibold">Skills</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-zinc-400">从 GitHub 导入目录型 Skill，系统会保留完整文件夹内容。</p>
        </div>
        <div className="mb-8 rounded-3xl border border-gray-200 p-5 dark:border-zinc-800">
          <div className="mb-3 text-sm font-medium">从 GitHub 文件夹导入 Skill</div>
          <div className="flex gap-2">
            <input
              value={githubUrl}
              onChange={(e) => {
                setGithubUrl(e.target.value)
                setError('')
                setSuccess('')
              }}
              disabled={loading}
              placeholder="例如 https://github.com/xxx"
              className="flex-1 rounded-xl border border-gray-200 bg-transparent px-3 py-2 text-sm disabled:opacity-60 dark:border-zinc-800"
            />
            <button onClick={importSkill} disabled={loading || !githubUrl.trim()} className="inline-flex min-w-24 items-center justify-center gap-2 rounded-xl bg-gray-900 px-4 py-2 text-sm text-white disabled:opacity-40 dark:bg-white dark:text-black">
              {loading ? <Loader2 size={16} className="animate-spin" /> : <Download size={16} />}
              {loading ? '导入中' : '导入'}
            </button>
          </div>
          {error && (
            <div className="mt-3 flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2.5 dark:border-red-500/20 dark:bg-red-500/10">
              <AlertCircle size={16} className="mt-0.5 shrink-0 text-red-600 dark:text-red-400" />
              <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}
          {success && (
            <div className="mt-3 flex items-start gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 dark:border-emerald-500/20 dark:bg-emerald-500/10">
              <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
              <p className="text-sm text-emerald-700 dark:text-emerald-300">{success}</p>
            </div>
          )}
        </div>
        <section>
          <div className="mb-3 flex items-center gap-2">
            <h3 className="text-sm font-medium">已导入 Skills</h3>
            <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-zinc-900 dark:text-zinc-400">{skills.length}</span>
          </div>
          {skills.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-gray-200 p-5 text-sm text-gray-400 dark:border-zinc-800">还没有导入 Skill</div>
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              {skills.map((skill) => (
                <div key={skill.id} className="flex min-h-52 flex-col rounded-3xl border border-gray-200 p-5 dark:border-zinc-800">
                  <div className="mb-2 flex items-center gap-2">
                    <Lightbulb size={16} />
                    <div className="font-medium">{skill.name}</div>
                    <div className="relative ml-auto">
                      <button
                        onClick={() => setOpenMenuId((current) => current === skill.id ? '' : skill.id)}
                        className="rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-zinc-900 dark:hover:text-zinc-200"
                        aria-label="更多操作"
                      >
                        <Ellipsis size={16} />
                      </button>
                      {openMenuId === skill.id && (
                        <div className="absolute right-0 top-8 z-10 min-w-28 rounded-xl border border-gray-200 bg-white p-1 shadow-lg dark:border-zinc-800 dark:bg-zinc-950">
                          <button
                            onClick={() => {
                              setOpenMenuId('')
                              setPendingDelete(skill)
                            }}
                            disabled={deletingId === skill.id}
                            className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-red-600 transition hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-500/10"
                          >
                            {deletingId === skill.id ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                            删除
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                  <p className="text-sm text-gray-500 dark:text-zinc-400">{skill.description || '无描述'}</p>
                  <div className="mt-auto space-y-1 pt-5 text-xs text-gray-400 dark:text-zinc-500">
                    <div>{skill.source_repo}@{skill.source_ref}</div>
                    <div>{skill.source_path}</div>
                    <div>{skill.file_count} 个文件</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      {pendingDelete && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4">
          <div className="w-full max-w-md rounded-3xl border border-gray-200 bg-white p-6 shadow-xl dark:border-zinc-800 dark:bg-zinc-950">
            <div className="mb-2 text-lg font-semibold">删除 Skill</div>
            <p className="text-sm leading-6 text-gray-500 dark:text-zinc-400">
              {pendingDelete.agent_count > 0
                ? `「${pendingDelete.name}」已被 ${pendingDelete.agent_count} 个智能体使用，删除后会自动解绑。`
                : `确定删除「${pendingDelete.name}」吗？`}
            </p>
            <div className="mt-6 flex justify-end gap-2">
              <button
                onClick={() => setPendingDelete(null)}
                className="rounded-xl border border-gray-200 px-4 py-2 text-sm dark:border-zinc-800"
              >
                取消
              </button>
              <button
                onClick={() => deleteSkill(pendingDelete)}
                disabled={deletingId === pendingDelete.id}
                className="inline-flex items-center gap-2 rounded-xl bg-red-600 px-4 py-2 text-sm text-white disabled:opacity-60"
              >
                {deletingId === pendingDelete.id && <Loader2 size={16} className="animate-spin" />}
                删除
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
