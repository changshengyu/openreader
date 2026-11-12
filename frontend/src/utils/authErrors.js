// The only place that turns register/login failures into the upstream reader-dev
// wording (UserController.kt#login and #addUser, shown by plugins/axios.js as
// Message.error). Contract: docs/compat/registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md
const AUTH_ERROR_TEXT = [
  [400, 'username and password are required', '请输入用户名和密码'],
  [400, 'username must be at least 5 characters', '用户名不能低于5位'],
  [400, 'password must be at least 8 characters', '密码不能低于8位'],
  [400, 'password must be at most 72 bytes', '密码不能超过72字节'],
  [400, 'username is reserved', '用户名不能为非法字符'],
  [400, 'username may contain only letters and numbers', '用户名只能由字母和数字组成'],
  [409, 'username already exists', '用户名已被占用'],
  [401, 'invalid username or password', '用户名或密码错误'],
  [400, 'invite code required', '请输入邀请码'],
  [403, 'invalid invite code', '邀请码错误'],
  [403, 'user limit reached', '超过用户数上限'],
]

function serverError(error) {
  const value = error?.response?.data?.error
  if (typeof value === 'string') return { code: '', message: value }
  if (value && typeof value === 'object') {
    return { code: String(value.code || ''), message: typeof value.message === 'string' ? value.message : '' }
  }
  return { code: '', message: '' }
}

export function authErrorMessage(error) {
  const status = error?.response?.status
  const { code, message } = serverError(error)
  const known = AUTH_ERROR_TEXT.find(([knownStatus, knownMessage]) => knownStatus === status && knownMessage === message)
  if (known) return known[2]
  if (status === 413) return '请求内容过大'
  if (status === 429 || code === 'RATE_LIMITED') return '请求过于频繁，请稍后再试'
  return message || '请求失败'
}

// reader-dev checks the username first, then the password, before any request.
export function authInputError({ username, password } = {}) {
  if (!String(username ?? '').trim()) return '请输入用户名'
  if (!String(password ?? '')) return '请输入密码'
  return ''
}

// User management keeps its server messages, except the account cap that
// reader-dev reports as 超过用户数上限.
export function adminUserErrorMessage(error, fallback) {
  const { code, message } = serverError(error)
  if (code === 'USER_LIMIT_REACHED') return '超过用户数上限'
  return message || fallback
}
