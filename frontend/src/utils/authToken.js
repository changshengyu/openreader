// The only place that reads or writes the login JWT (openreader_token).
// reader-dev App.vue#login keeps the token (localStorage api_token) only when
// 记住登录信息 is checked. OpenReader keeps a remembered login in localStorage
// and an unremembered one in this tab's sessionStorage, which survives a reload
// and windows the app opens, but not closing the tab or the browser.
// Contract: docs/compat/remember-login-fixed-baseline-second-audit-p2-contract.md
export const AUTH_TOKEN_KEY = 'openreader_token'

const AREAS = ['sessionStorage', 'localStorage']

function storageArea(name) {
  try {
    const scope = typeof window !== 'undefined' && window ? window : globalThis
    const area = scope[name] ?? globalThis[name]
    return area && typeof area.getItem === 'function' ? area : null
  } catch {
    return null
  }
}

function readArea(name) {
  try {
    return String(storageArea(name)?.getItem(AUTH_TOKEN_KEY) || '')
  } catch {
    return ''
  }
}

function removeArea(name, onlyToken) {
  try {
    const area = storageArea(name)
    if (!area) return
    if (onlyToken !== undefined && area.getItem(AUTH_TOKEN_KEY) !== onlyToken) return
    area.removeItem(AUTH_TOKEN_KEY)
  } catch {
    // Unavailable storage holds no token to remove.
  }
}

// This tab's unremembered login wins over a remembered one.
export function readAuthToken() {
  return readArea('sessionStorage') || readArea('localStorage')
}

// remember=true keeps the token in localStorage for every tab; remember=false
// keeps it only in this tab and drops any remembered token, so the browser
// keeps no lasting login after the user chose not to be remembered.
export function writeAuthToken(token, { remember = true } = {}) {
  const value = String(token || '')
  if (remember) {
    storageArea('localStorage')?.setItem(AUTH_TOKEN_KEY, value)
    removeArea('sessionStorage')
  } else {
    storageArea('sessionStorage')?.setItem(AUTH_TOKEN_KEY, value)
    removeArea('localStorage')
  }
}

// Removes the token only where it still is exactly this value, so a late 401
// for an older token cannot sign a newer login out.
export function discardAuthToken(token) {
  if (!token) return
  for (const name of AREAS) removeArea(name, token)
}

// Logout and session invalidation sign the whole browser out.
export function clearAuthToken() {
  for (const name of AREAS) removeArea(name)
}
