import type { ReactNode } from 'react'
import { FileText, Search, Brain } from 'lucide-react'

export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-6 dark:bg-zinc-950">
      <div className="grid w-full max-w-4xl overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900 md:grid-cols-[1.05fr_0.95fr]">
        <div className="hidden border-r border-gray-200 bg-gray-50/80 p-8 dark:border-zinc-800 dark:bg-zinc-950/60 md:flex md:flex-col md:justify-between">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-gray-400 dark:text-zinc-500">
              Knowledge RAG Platform
            </p>
            <h2 className="mt-5 max-w-sm text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">
              把文档变成可检索、可引用的知识
            </h2>
            <p className="mt-3 max-w-sm text-sm leading-6 text-gray-500 dark:text-zinc-400">
              上传资料，完成切分、召回与回答，让知识库真正参与每一次提问。
            </p>
          </div>

          <div className="space-y-3">
            <Feature icon={<FileText size={17} />} title="文档入库" />
            <Feature icon={<Search size={17} />} title="混合检索" />
            <Feature icon={<Brain size={17} />} title="长期记忆" />
          </div>
        </div>

        <div className="p-7 sm:p-8">
          <div className="mx-auto w-full max-w-sm">{children}</div>
        </div>
      </div>
    </div>
  )
}

function Feature({ icon, title }: { icon: ReactNode; title: string }) {
  return (
    <div className="flex items-center gap-3 rounded-2xl border border-gray-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gray-100 text-gray-600 dark:bg-zinc-800 dark:text-zinc-300">
        {icon}
      </div>
      <p className="flex min-h-9 items-center text-sm font-medium text-gray-800 dark:text-zinc-200">{title}</p>
    </div>
  )
}
