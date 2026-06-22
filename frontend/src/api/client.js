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

let lastRejectedToken = ''

api.interceptors.response.use(
  response => response,
  error => {
    const status = Number(error?.response?.status || 0)
    const requestURL = String(error?.config?.url || '')
    const isAuthRequest = requestURL.includes('/auth/login') || requestURL.includes('/auth/register')
    const authorization = String(error?.config?.headers?.Authorization || '')
    const rejectedToken = authorization.startsWith('Bearer ') ? authorization.slice(7) : ''
    if (status === 401 && rejectedToken && !isAuthRequest && rejectedToken !== lastRejectedToken) {
      lastRejectedToken = rejectedToken
      if (localStorage.getItem('openreader_token') === rejectedToken) {
        localStorage.removeItem('openreader_token')
      }
      window.dispatchEvent(new CustomEvent('openreader:auth-required', {
        detail: { reason: 'session', rejectedToken },
      }))
    }
    return Promise.reject(error)
  },
)

export default api
