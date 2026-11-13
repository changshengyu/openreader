import assert from 'node:assert/strict'
import test from 'node:test'

// docs/compat/reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md R2-R4 and the D1 (b) table
// (section 7, section 13 item 4): the error page label, the toast prefix and the Chinese reason a failed chapter or
// catalog load shows. Backend English messages and source exception text never reach the page or the toast.

const MODULE = '../src/utils/readerChapterError.js'

async function errorText() {
  try {
    return await import(MODULE)
  } catch (error) {
    assert.fail(`reader chapter error text helper must exist at frontend/src/utils/readerChapterError.js: ${error.message}`)
  }
}

function httpError(status, data) {
  const error = new Error(`Request failed with status code ${status}`)
  error.response = { status, data }
  return error
}

const SOURCE_FAILURE = (code) => ({ error: 'failed to load chapter content', code, stage: 'content' })
const BACKEND_ENGLISH = [
  'failed to load chapter content',
  'failed to list chapters',
  'book not found',
  'chapter content changed',
  'Request failed',
  'Network Error',
  'timeout',
  'remote reader session is invalid',
  'fixture',
]

function assertChinese(reason, label) {
  assert.equal(typeof reason, 'string', `${label}: the reason must be a string`)
  assert(reason.length > 0, `${label}: the reason must not be empty`)
  for (const english of BACKEND_ENGLISH) {
    assert(!reason.includes(english), `${label}: the reason ${JSON.stringify(reason)} leaks ${JSON.stringify(english)}`)
  }
}

const CONTENT_FALLBACK = '章节加载失败，请检查书源或网络后重试'
const CATALOG_FALLBACK = '目录加载失败，请检查书源或网络后重试'

const D1_ROWS = [
  ['source_request_failed', httpError(502, SOURCE_FAILURE('source_request_failed')), '请求书源失败，请检查网络或稍后重试'],
  ['source_rule_invalid', httpError(502, SOURCE_FAILURE('source_rule_invalid')), '书源规则有误，请检查书源'],
  ['source_rule_unsupported', httpError(502, SOURCE_FAILURE('source_rule_unsupported')), '书源规则暂不支持'],
  ['content_unavailable', httpError(502, SOURCE_FAILURE('content_unavailable')), '书源没有返回可用正文'],
  ['404', httpError(404, { error: 'chapter not found' }), '章节或书籍不存在，请刷新目录后重试'],
  ['NOT_FOUND envelope', httpError(404, { error: { code: 'NOT_FOUND', message: 'book not found' } }), '章节或书籍不存在，请刷新目录后重试'],
  ['NOT_FOUND envelope with another status', httpError(500, { error: { code: 'NOT_FOUND', message: 'book not found' } }), '章节或书籍不存在，请刷新目录后重试'],
  ['400', httpError(400, { error: 'invalid chapter index' }), '请求参数无效，请刷新后重试'],
  ['401', httpError(401, { error: 'unauthorized' }), '登录已失效，请重新登录'],
  ['403', httpError(403, { error: 'forbidden' }), '访问被拒绝，请重新打开书籍后重试'],
  ['415', httpError(415, { error: 'unsupported epub' }), '文件格式无法解析'],
  ['422', httpError(422, { error: 'invalid cbz archive' }), '文件格式无法解析'],
  ['429', httpError(429, { error: 'too many requests' }), '请求过于频繁，请稍后重试'],
  ['500', httpError(500, { error: 'failed to list chapters' }), '服务器处理失败，请稍后重试'],
  ['502 without a source code', httpError(502, { error: 'failed to load chapter content' }), '服务器处理失败，请稍后重试'],
  ['503', httpError(503, { error: 'service unavailable' }), '服务器处理失败，请稍后重试'],
]

for (const [label, error, expected] of D1_ROWS) {
  test(`D1 maps ${label} to ${expected}`, async () => {
    const { readerChapterErrorReason } = await errorText()
    const content = readerChapterErrorReason(error, 'content')
    assert.equal(content, expected, `content ${label}`)
    assertChinese(content, `content ${label}`)
    const catalog = readerChapterErrorReason(error, 'catalog')
    assert.equal(catalog, expected, `catalog ${label}`)
    assertChinese(catalog, `catalog ${label}`)
  })
}

test('D1 checks the source code before the HTTP status', async () => {
  const { readerChapterErrorReason } = await errorText()
  const reason = readerChapterErrorReason(httpError(500, SOURCE_FAILURE('content_unavailable')))
  assert.equal(reason, '书源没有返回可用正文')
})

test('a stale chapter conflict that survives the automatic refetch asks for a retry', async () => {
  const { readerChapterErrorReason } = await errorText()
  const stale = new Error('章节状态已更新，请重试')
  stale.name = 'ChapterStaleConflictError'
  stale.cause = httpError(409, { error: 'chapter content changed; retry' })
  assert.equal(readerChapterErrorReason(stale), '章节状态已更新，请重试')
  assert.equal(
    readerChapterErrorReason(httpError(409, { error: 'chapter content changed; retry' })),
    '章节状态已更新，请重试',
  )
})

test('failures without a response use the Chinese fallback of their kind', async () => {
  const { readerChapterErrorReason } = await errorText()
  const network = new Error('Network Error')
  network.code = 'ERR_NETWORK'
  assert.equal(readerChapterErrorReason(network), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason(network, 'content'), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason(network, 'catalog'), CATALOG_FALLBACK)
  const timeout = new Error('timeout of 30000ms exceeded')
  timeout.code = 'ECONNABORTED'
  assert.equal(readerChapterErrorReason(timeout, 'content'), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason(new Error('remote reader session is invalid'), 'catalog'), CATALOG_FALLBACK)
  assert.equal(readerChapterErrorReason(undefined, 'content'), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason('fixture chapter failure', 'catalog'), CATALOG_FALLBACK)
})

test('unknown codes and statuses use the Chinese fallback of their kind', async () => {
  const { readerChapterErrorReason } = await errorText()
  const unknownCode = httpError(418, { error: 'fixture chapter failure', code: 'source_mystery' })
  assert.equal(readerChapterErrorReason(unknownCode, 'content'), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason(unknownCode, 'catalog'), CATALOG_FALLBACK)
  const unknownStatus = httpError(409, { error: 'fixture chapter failure' })
  assert.equal(readerChapterErrorReason(unknownStatus, 'content'), CONTENT_FALLBACK)
  assert.equal(readerChapterErrorReason(unknownStatus, 'catalog'), CATALOG_FALLBACK)
})

test('error page labels follow the failed load', async () => {
  const { readerChapterErrorLabel } = await errorText()
  assert.equal(readerChapterErrorLabel('content'), '获取章节内容失败！')
  assert.equal(readerChapterErrorLabel('catalog'), '获取章节目录失败！')
  assert.equal(readerChapterErrorLabel(), '获取章节内容失败！')
})

test('failure toasts use reader-dev prefixes with the mapped reason', async () => {
  const { readerChapterErrorReason, readerChapterErrorToast } = await errorText()
  const source = httpError(502, SOURCE_FAILURE('content_unavailable'))
  assert.equal(
    readerChapterErrorToast('content', readerChapterErrorReason(source, 'content')),
    '获取章节内容失败 书源没有返回可用正文',
  )
  assert.equal(
    readerChapterErrorToast('catalog', readerChapterErrorReason(httpError(500, { error: 'failed to list chapters' }), 'catalog')),
    '获取书籍目录列表 服务器处理失败，请稍后重试',
  )
})

test('EPUB resource failures show a fixed Chinese reason instead of the resource error', async () => {
  const { READER_EPUB_RESOURCE_ERROR_REASON } = await errorText()
  assert.equal(READER_EPUB_RESOURCE_ERROR_REASON, 'EPUB 正文加载失败，请重试')
})
