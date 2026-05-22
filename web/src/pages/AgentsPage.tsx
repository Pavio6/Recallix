import { useEffect, useState } from 'react'
import { Bot, Plus, X } from 'lucide-react'
import { agentAPI } from '../services/api'

interface Skill { id: string; name: string; description: string }
interface Agent {
  id: string
  name: string
  nickname: string
  model: string
  prompt: string
  skills?: Skill[]
}

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [skills, setSkills] = useState<Skill[]>([])
  const [editing, setEditing] = useState<Agent | null>(null)
  const [draft, setDraft] = useState({ nickname: '', prompt: '', skillIds: [] as string[] })

  const load = async () => {
    const [agentsRes, skillsRes] = await Promise.all([agentAPI.list(), agentAPI.listSkills()])
    if (agentsRes.data.success) setAgents(agentsRes.data.data || [])
    if (skillsRes.data.success) setSkills(skillsRes.data.data || [])
  }
  useEffect(() => { load() }, [])

  const open = (agent: Agent) => {
    setEditing(agent)
    setDraft({
      nickname: agent.nickname || '',
      prompt: agent.prompt,
      skillIds: (agent.skills || []).map((s) => s.id),
    })
  }
  const create = async () => {
    const { data } = await agentAPI.create({ skill_ids: [] })
    if (data.success) {
      await load()
      open(data.data)
    }
  }
  const toggle = (items: string[], id: string) => items.includes(id) ? items.filter((item) => item !== id) : [...items, id]
  const save = async () => {
    if (!editing) return
    const { data } = await agentAPI.update(editing.id, {
      name: editing.name,
      nickname: draft.nickname,
      model: editing.model,
      prompt: draft.prompt,
      skill_ids: draft.skillIds,
    })
    if (data.success) {
      setEditing(null)
      await load()
    }
  }
  return (
    <div className="h-full overflow-y-auto bg-white p-8 dark:bg-zinc-950">
      <div className="mx-auto max-w-5xl">
        <div className="mb-8 flex items-start justify-between">
          <div>
            <h2 className="text-2xl font-semibold">智能体</h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-zinc-400">配置和管理你的智能体，对话时可在智能推理模式中选择使用。</p>
          </div>
          <button onClick={create} className="inline-flex items-center gap-2 rounded-xl bg-gray-900 px-4 py-2 text-sm text-white dark:bg-white dark:text-black"><Plus size={16} />新建智能体</button>
        </div>

        {agents.length === 0 ? (
          <div className="rounded-3xl border border-dashed border-gray-200 p-12 text-center text-sm text-gray-400 dark:border-zinc-800 dark:text-zinc-500">
            还没有配置智能体
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            {agents.map((agent) => (
              <button key={agent.id} onClick={() => open(agent)} className="rounded-3xl border border-gray-200 p-5 text-left transition hover:border-gray-300 dark:border-zinc-800 dark:hover:border-zinc-700">
                <div className="mb-4 flex items-center gap-3">
                  <div className="rounded-2xl bg-gray-100 p-3 dark:bg-zinc-900"><Bot size={20} /></div>
                  <div>
                    <div className="font-medium">{agent.nickname || '未命名智能体'}</div>
                    <div className="text-xs text-gray-400">{agent.model}</div>
                  </div>
                </div>
                <div className="flex gap-2 text-xs text-gray-500 dark:text-zinc-400">
                  <span>{agent.skills?.length || 0} Skills</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      {editing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4">
          <div className="max-h-[88vh] w-full max-w-3xl overflow-y-auto rounded-3xl border border-gray-200 bg-white p-6 dark:border-zinc-800 dark:bg-zinc-950">
            <div className="mb-5 flex justify-between">
              <div>
                <h3 className="text-lg font-semibold">{draft.nickname || '未命名智能体'}</h3>
                <p className="text-sm text-gray-500 dark:text-zinc-400">修改会影响所有选择该智能体的会话。</p>
              </div>
              <button onClick={() => setEditing(null)}><X size={18} /></button>
            </div>
            <div className="space-y-5">
              <section>
                <label className="mb-2 block text-sm font-medium">昵称</label>
                <input value={draft.nickname} onChange={(e) => setDraft({ ...draft, nickname: e.target.value })} placeholder="可选" className="w-full rounded-xl border border-gray-200 bg-transparent px-3 py-2 text-sm dark:border-zinc-800" />
              </section>
              <section>
                <label className="mb-2 block text-sm font-medium">Prompt</label>
                <textarea value={draft.prompt} onChange={(e) => setDraft({ ...draft, prompt: e.target.value })} rows={8} className="w-full rounded-2xl border border-gray-200 bg-transparent p-3 text-sm dark:border-zinc-800" />
              </section>
              <section>
                <div className="mb-2 text-sm font-medium">Skills</div>
                <div className="space-y-2">{skills.map((skill) => <label key={skill.id} className="flex gap-3 rounded-xl border border-gray-200 p-3 text-sm dark:border-zinc-800"><input type="checkbox" checked={draft.skillIds.includes(skill.id)} onChange={() => setDraft({ ...draft, skillIds: toggle(draft.skillIds, skill.id) })} />{skill.name}</label>)}</div>
              </section>
            </div>
            <div className="mt-6 flex justify-end gap-2"><button onClick={() => setEditing(null)} className="rounded-xl border px-4 py-2 text-sm dark:border-zinc-800">取消</button><button onClick={save} className="rounded-xl bg-gray-900 px-4 py-2 text-sm text-white dark:bg-white dark:text-black">保存</button></div>
          </div>
        </div>
      )}
    </div>
  )
}
