import axios from 'axios'

const api = axios.create({ baseURL: '/api/v1' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config
    const isAuthRequest = original.url?.includes('/auth/login') || original.url?.includes('/auth/register')
    if (error.response?.status === 401 && !original._retry && !isAuthRequest) {
      original._retry = true
      const refreshToken = localStorage.getItem('refresh_token')
      if (refreshToken) {
        try {
          const { data } = await axios.post('/api/v1/auth/refresh', { refresh_token: refreshToken })
          if (data.success) {
            localStorage.setItem('access_token', data.data.access_token)
            localStorage.setItem('refresh_token', data.data.refresh_token)
            original.headers.Authorization = `Bearer ${data.data.access_token}`
            return api(original)
          }
        } catch {
          localStorage.clear()
          window.location.href = '/login'
        }
      }
      localStorage.clear()
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export const authAPI = {
  register: (data: { email: string; password: string; nickname: string }) =>
    api.post('/auth/register', data),
  login: (data: { email: string; password: string }) =>
    api.post('/auth/login', data),
  me: () => api.get('/auth/me'),
}

export const kbAPI = {
  list: () => api.get('/knowledge-bases'),
  create: (data: { name: string; description?: string }) =>
    api.post('/knowledge-bases', data),
  uploadFile: (kbId: string, file: File) => {
    const form = new FormData()
    form.append('file', file)
    return api.post(`/knowledge-bases/${kbId}/files`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}

export const knowledgeAPI = {
  list: (kbId?: string) =>
    api.get('/knowledges', { params: kbId ? { knowledge_base_id: kbId } : {} }),
  getContent: (id: string) => api.get(`/knowledges/${id}/content`),
  getFile: (id: string) => api.get(`/knowledges/${id}/file`, { responseType: 'blob' }),
}

export const sessionAPI = {
  create: (title?: string) => api.post('/sessions', { title }),
  listRecent: () => api.get('/sessions/recent'),
  getMessages: (id: string) => api.get(`/sessions/${id}/messages`),
}

export const chatAPI = {
  send: (sessionId: string, question: string) =>
    fetch(`/api/v1/sessions/${sessionId}/chat`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
      body: JSON.stringify({ question }),
    }),
}

export default api
