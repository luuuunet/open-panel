import axios from 'axios'

/** AI 助手后端最长可轮询约 5 分钟（Cursor Agent），前端需留足余量 */
export const AI_REQUEST_TIMEOUT = 360000

export function apiBaseURL(): string {
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || '/'
  const prefix = base.endsWith('/') ? base : base + '/'
  return prefix + 'api/v1'
}

function apiBaseURLInternal(): string {
  return apiBaseURL()
}

export function isApiTimeout(err: unknown): boolean {
  const e = err as { error?: string; code?: string; message?: string } | null | undefined
  if (!e) return false
  if (e.error === 'REQUEST_TIMEOUT' || e.code === 'ECONNABORTED') return true
  return typeof e.message === 'string' && /timeout of \d+ms exceeded/i.test(e.message)
}

export function resolveApiError(err: unknown, fallback: string, timeoutFallback?: string): string {
  if (isApiTimeout(err)) return timeoutFallback || fallback
  const e = err as { error?: string; message?: string } | null | undefined
  return e?.error || e?.message || fallback
}

const api = axios.create({
  baseURL: apiBaseURLInternal(),
  timeout: 30000,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => res.data,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      const w = window as Window & { __OWPANEL_BASE__?: string }
      const base = w.__OWPANEL_BASE__ || '/'
      window.location.href = base.replace(/\/?$/, '/') + 'login'
    }
    if (err.code === 'ECONNABORTED' || (typeof err.message === 'string' && /timeout of \d+ms exceeded/i.test(err.message))) {
      return Promise.reject({ error: 'REQUEST_TIMEOUT', message: err.message })
    }
    return Promise.reject(err.response?.data || err)
  }
)

export default api
