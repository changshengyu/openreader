import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 12000,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('openreader_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let redirectingToLogin = false

api.interceptors.response.use(
  response => response,
  error => {
    const status = Number(error?.response?.status || 0)
    const requestURL = String(error?.config?.url || '')
    const isAuthRequest = requestURL.includes('/auth/login') || requestURL.includes('/auth/register')
    const hasSession = Boolean(localStorage.getItem('openreader_token'))
    if (status === 401 && hasSession && !isAuthRequest && !redirectingToLogin) {
      redirectingToLogin = true
      localStorage.removeItem('openreader_token')
      const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`
      const query = new URLSearchParams({ reason: 'session', returnTo })
      window.location.replace(`/login?${query.toString()}`)
    }
    return Promise.reject(error)
  },
)

export default api
