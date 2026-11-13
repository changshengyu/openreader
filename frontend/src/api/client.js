import axios from 'axios'
import { discardAuthToken, readAuthToken } from '../utils/authToken.js'

function notifyAuthRequired(detail) {
  if (typeof window === 'undefined') return
  window.__openreaderAuthRequired = detail
  window.dispatchEvent(new CustomEvent('openreader:auth-required', { detail }))
}

function addAuthInterceptors(client) {
  client.interceptors.request.use((config) => {
    const token = readAuthToken()
    if (token && !config.headers.Authorization) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  })

  client.interceptors.response.use(
    response => response,
    error => {
      const status = Number(error?.response?.status || 0)
      const requestURL = String(error?.config?.url || '')
      const isAuthRequest = requestURL.includes('/auth/login') || requestURL.includes('/auth/register')
      const authorization = String(error?.config?.headers?.Authorization || '')
      const rejectedToken = authorization.startsWith('Bearer ') ? authorization.slice(7) : ''
      if (status === 401 && rejectedToken && !isAuthRequest) {
        if (readAuthToken() !== rejectedToken) {
          return Promise.reject(error)
        }
        discardAuthToken(rejectedToken)
        notifyAuthRequired({ reason: 'session', rejectedToken })
      }
      return Promise.reject(error)
    },
  )
  return client
}

const api = addAuthInterceptors(axios.create({
  baseURL: '/api',
  timeout: 12000,
}))

export const rootApi = addAuthInterceptors(axios.create({
  timeout: 12000,
}))

export default api
