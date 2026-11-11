import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

// Contract: docs/compat/registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md
const __dirname = dirname(fileURLToPath(import.meta.url))
const read = path => readFileSync(resolve(__dirname, '..', path), 'utf8')
const optionalModule = async path => import(new URL(`../${path}`, import.meta.url)).catch(() => null)

const httpError = (status, error) => ({ response: { status, data: { error } } })

test('register and login errors show the upstream Chinese messages', async () => {
  const errors = await optionalModule('src/utils/authErrors.js')
  assert.ok(errors?.authErrorMessage, 'src/utils/authErrors.js must export authErrorMessage')
  const cases = [
    [httpError(400, 'username and password are required'), '请输入用户名和密码'],
    [httpError(400, 'username must be at least 5 characters'), '用户名不能低于5位'],
    [httpError(400, 'password must be at least 8 characters'), '密码不能低于8位'],
    [httpError(400, 'password must be at most 72 bytes'), '密码不能超过72字节'],
    [httpError(400, 'username is reserved'), '用户名不能为非法字符'],
    [httpError(400, 'username may contain only letters and numbers'), '用户名只能由字母和数字组成'],
    [httpError(409, 'username already exists'), '用户名已被占用'],
    [httpError(401, 'invalid username or password'), '用户名或密码错误'],
    [httpError(400, 'invite code required'), '请输入邀请码'],
    [httpError(403, 'invalid invite code'), '邀请码错误'],
    [httpError(403, 'user limit reached'), '超过用户数上限'],
    [httpError(413, 'request body too large'), '请求内容过大'],
    [httpError(429, { code: 'RATE_LIMITED', message: 'too many requests, try again later' }), '请求过于频繁，请稍后再试'],
  ]
  for (const [error, message] of cases) assert.equal(errors.authErrorMessage(error), message, JSON.stringify(error.response))
  // Unknown errors keep the server text; without one the fallback stays 请求失败.
  assert.equal(errors.authErrorMessage(httpError(500, 'failed to create token')), 'failed to create token')
  assert.equal(errors.authErrorMessage(httpError(500, { code: 'X', message: 'nested message' })), 'nested message')
  assert.equal(errors.authErrorMessage(new Error('network down')), '请求失败')
  assert.equal(errors.authErrorMessage(undefined), '请求失败')
})

test('the form checks empty fields before submitting, in upstream order', async () => {
  const errors = await optionalModule('src/utils/authErrors.js')
  assert.ok(errors?.authInputError, 'src/utils/authErrors.js must export authInputError')
  assert.equal(errors.authInputError({ username: '', password: '' }), '请输入用户名')
  assert.equal(errors.authInputError({ username: '   ', password: 'password8x' }), '请输入用户名')
  assert.equal(errors.authInputError({ username: 'reader01', password: '' }), '请输入密码')
  assert.equal(errors.authInputError({ username: 'reader01', password: 'password8x' }), '')
})

test('only registration sends the invite code', async () => {
  const request = await optionalModule('src/utils/authRequest.js')
  assert.ok(request?.authRequestPayload, 'src/utils/authRequest.js must export authRequestPayload')
  assert.deepEqual(request.authRequestPayload('register', { username: 'reader01', password: 'password8x', code: 'abc' }),
    { username: 'reader01', password: 'password8x', code: 'abc' })
  assert.deepEqual(request.authRequestPayload('register', { username: 'reader01', password: 'password8x' }),
    { username: 'reader01', password: 'password8x', code: '' })
  assert.deepEqual(request.authRequestPayload('login', { username: 'reader01', password: 'password8x', code: 'abc' }),
    { username: 'reader01', password: 'password8x' })
})

test('the auth form shows the optional invite field only in register mode and wires the shared helpers', () => {
  const form = read('src/components/AuthForm.vue')
  const field = form.indexOf('label="邀请码(没有则不填)"')
  assert.notEqual(field, -1, 'register mode needs the 邀请码(没有则不填) field')
  const item = form.lastIndexOf('<el-form-item', field)
  assert.match(form.slice(item, field), /v-if="mode === 'register'"/, 'the invite field belongs to register mode only')
  assert.ok(field > form.indexOf('label="密码"'), 'the invite field sits below the password field')
  assert.match(form, /import \{[^}]*\bauthErrorMessage\b[^}]*\bauthInputError\b[^}]*\} from '\.\.\/utils\/authErrors'/)
  assert.match(form, /authInputError\(\{ username: username\.value, password: password\.value \}\)/)
  assert.match(form, /ElMessage\.error\(authErrorMessage\(err\)\)/)
  assert.match(form, /user\.login\(username\.value, password\.value, mode\.value, inviteCode\.value\)/)

  const store = read('src/stores/user.js')
  assert.match(store, /async login\(username, password, mode = 'login', code = ''\)/)
  assert.match(store, /loginUser\(mode, authRequestPayload\(mode, \{ username, password, code \}\)\)/)
})

test('user management shows 超过用户数上限 at the user limit', async () => {
  const errors = await optionalModule('src/utils/authErrors.js')
  assert.ok(errors?.adminUserErrorMessage, 'src/utils/authErrors.js must export adminUserErrorMessage')
  assert.equal(errors.adminUserErrorMessage(httpError(403, { code: 'USER_LIMIT_REACHED', message: 'user limit reached' }), '新增用户失败'),
    '超过用户数上限')
  assert.equal(errors.adminUserErrorMessage(httpError(409, { code: 'CONFLICT', message: 'username already exists' }), '新增用户失败'),
    'username already exists')
  assert.equal(errors.adminUserErrorMessage(httpError(400, 'plain message'), '新增用户失败'), 'plain message')
  assert.equal(errors.adminUserErrorMessage(new Error('offline'), '新增用户失败'), '新增用户失败')

  const overlay = read('src/components/overlays/OverlayUserManagement.vue')
  assert.match(overlay, /import \{ adminUserErrorMessage \} from '\.\.\/\.\.\/utils\/authErrors'/)
  assert.match(overlay, /onError: \(error, fallback\) => ElMessage\.error\(adminUserErrorMessage\(error, fallback\)\)/)
})
