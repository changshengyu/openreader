// Reader chapter and catalog failure text (docs/compat/reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md
// R2-R4 and the D1 (b) table). The page and the toast never show backend English or source exception text: the
// reason comes from the stable backend code or the HTTP status, checked in the table's order, with a Chinese fallback.

export const READER_CHAPTER_ERROR_CONTENT = 'content'
export const READER_CHAPTER_ERROR_CATALOG = 'catalog'
export const READER_STALE_CHAPTER_REASON = '章节状态已更新，请重试'
export const READER_EPUB_RESOURCE_ERROR_REASON = 'EPUB 正文加载失败，请重试'

const LABELS = {
  content: '获取章节内容失败！',
  catalog: '获取章节目录失败！',
}
// reader-dev: getContent toasts `获取章节内容失败 <error>`, loadCatalog toasts `获取书籍目录列表 <error>`.
const TOAST_PREFIXES = {
  content: '获取章节内容失败',
  catalog: '获取书籍目录列表',
}
const FALLBACK_REASONS = {
  content: '章节加载失败，请检查书源或网络后重试',
  catalog: '目录加载失败，请检查书源或网络后重试',
}
// backend/api/source_errors.go: the stable codes a source failure carries next to its generic message.
const SOURCE_REASONS = {
  source_request_failed: '请求书源失败，请检查网络或稍后重试',
  source_rule_invalid: '书源规则有误，请检查书源',
  source_rule_unsupported: '书源规则暂不支持',
  content_unavailable: '书源没有返回可用正文',
}
const NOT_FOUND_REASON = '章节或书籍不存在，请刷新目录后重试'
const STATUS_REASONS = {
  400: '请求参数无效，请刷新后重试',
  401: '登录已失效，请重新登录',
  403: '访问被拒绝，请重新打开书籍后重试',
  404: NOT_FOUND_REASON,
  415: '文件格式无法解析',
  422: '文件格式无法解析',
  429: '请求过于频繁，请稍后重试',
}
const SERVER_REASON = '服务器处理失败，请稍后重试'
const STALE_CHAPTER_CONFLICT = 'chapter content changed; retry'

function failureKind(kind) {
  return kind === READER_CHAPTER_ERROR_CATALOG ? READER_CHAPTER_ERROR_CATALOG : READER_CHAPTER_ERROR_CONTENT
}

export function readerChapterErrorLabel(kind = READER_CHAPTER_ERROR_CONTENT) {
  return LABELS[failureKind(kind)]
}

export function readerChapterErrorToast(kind, reason) {
  return `${TOAST_PREFIXES[failureKind(kind)]} ${reason}`
}

export function readerChapterErrorReason(error, kind = READER_CHAPTER_ERROR_CONTENT) {
  const response = error?.response
  const status = Number(response?.status)
  const data = response?.data && typeof response.data === 'object' ? response.data : null
  // 1. A stale chapter conflict that survived the automatic refetch.
  if (
    error?.name === 'ChapterStaleConflictError'
    || (status === 409 && data?.error === STALE_CHAPTER_CONFLICT)
  ) return READER_STALE_CHAPTER_REASON
  // 2. The source failure code comes before the status (a source failure is itself a 502).
  const code = typeof data?.code === 'string' ? data.code : ''
  if (Object.hasOwn(SOURCE_REASONS, code)) return SOURCE_REASONS[code]
  // 3. The HTTP status; the errResp NOT_FOUND envelope counts as a 404.
  if (data?.error?.code === 'NOT_FOUND') return NOT_FOUND_REASON
  if (Object.hasOwn(STATUS_REASONS, status)) return STATUS_REASONS[status]
  if (status >= 500 && status <= 599) return SERVER_REASON
  // 4-5. No response, an unknown code or another status.
  return FALLBACK_REASONS[failureKind(kind)]
}
