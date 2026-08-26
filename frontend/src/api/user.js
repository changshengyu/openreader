import api from './client'

export function loginUser(mode, payload) {
  return api.post(`/auth/${mode}`, payload)
}

export function getMe() {
  return api.get('/me')
}

export function logoutUser(token) {
  return api.post('/auth/logout', null, {
    headers: { Authorization: `Bearer ${token}` },
  })
}
