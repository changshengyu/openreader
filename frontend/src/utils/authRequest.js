// Registration sends the optional invite code (reader-dev loginForm.code with
// isLogin=false); login never does.
export function authRequestPayload(mode, { username, password, code = '' } = {}) {
  if (mode === 'register') return { username, password, code: String(code ?? '') }
  return { username, password }
}
