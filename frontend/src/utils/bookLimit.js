// reader-dev BookController.kt#saveBook refuses a new book once the shelf holds
// reader.app.userBookLimit books with 超过用户书籍数上限, which plugins/axios.js
// shows as Message.error. OpenReader answers 403 {"error":"book limit reached"},
// or that text on one item of a batch import. This is the only place that turns
// it into the upstream wording.
// Contract: docs/compat/user-book-limit-fixed-baseline-second-audit-p2-contract.md
export const BOOK_LIMIT_MESSAGE = '超过用户书籍数上限'

const SERVER_ERROR = 'book limit reached'

// Returns the upstream text for a book-limit refusal (an API error or a batch
// item's error string), otherwise ''.
export function bookLimitMessage(errorOrText) {
  if (typeof errorOrText === 'string') return errorOrText === SERVER_ERROR ? BOOK_LIMIT_MESSAGE : ''
  const response = errorOrText?.response
  return response?.status === 403 && response?.data?.error === SERVER_ERROR ? BOOK_LIMIT_MESSAGE : ''
}

// Add-to-shelf callers format failures with their own readError, which reads
// response.data.error first. A book-limit refusal reaches them with the
// upstream text in that place; any other error is passed on unchanged.
export function bookLimitDisplayError(error) {
  if (!bookLimitMessage(error)) return error
  const shown = new Error(BOOK_LIMIT_MESSAGE)
  shown.response = { status: 403, data: { error: BOOK_LIMIT_MESSAGE } }
  return shown
}
