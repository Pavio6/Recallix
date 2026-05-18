import { fetchEventSource } from '@microsoft/fetch-event-source'

interface StreamResponse {
  response_type: 'answer' | 'thinking' | 'references' | 'error' | 'stop'
  content?: string
  done?: boolean
  retrieval_status?: RetrievalStatus
  references?: any[]
  data?: any
}

export type RetrievalStatus = 'skipped' | 'hit' | 'miss'

export function streamChat(
  sessionId: string,
  question: string,
  handlers: {
    onAnswer?: (content: string) => void
    onReferences?: (refs: any[], retrievalStatus?: RetrievalStatus) => void
    onError?: (msg: string) => void
    onDone?: () => void
  }
) {
  const controller = new AbortController()

  let doneCalled = false

  fetchEventSource(`/api/v1/sessions/${sessionId}/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${localStorage.getItem('access_token')}`,
    },
    body: JSON.stringify({ question }),
    signal: controller.signal,
    openWhenHidden: true,

    onopen: async (res) => {
      if (!res.ok) {
        const body = await res.text()
        throw new Error(`HTTP ${res.status}: ${body}`)
      }
    },

    onmessage: (ev) => {
      if (!ev.data) return
      try {
        const parsed: StreamResponse = JSON.parse(ev.data)
        switch (parsed.response_type) {
          case 'answer':
            handlers.onAnswer?.(parsed.content || '')
            break
          case 'references':
            handlers.onReferences?.(parsed.references || [], parsed.retrieval_status)
            break
          case 'error':
            if (!doneCalled) {
              doneCalled = true
              handlers.onError?.(parsed.content || 'Unknown error')
            }
            break
          case 'stop':
            if (!doneCalled) {
              doneCalled = true
              handlers.onDone?.()
            }
            break
        }
      } catch {}
    },

    onerror: (err) => {
      console.error('[Chat SSE]', err)
      handlers.onError?.(err.message || 'Connection error')
      throw err
    },

    onclose: () => {
      if (!doneCalled) {
        doneCalled = true
        handlers.onDone?.()
      }
    },
  })

  return controller
}
