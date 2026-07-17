import { unref } from 'vue'

export function useReaderBookmarkActions(options) {
  function currentPayload(extra = {}) {
    const chapter = unref(options.chapter)
    if (!chapter) return null
    const currentContext = options.getCurrentContext?.()
    if (typeof options.getCurrentContext === 'function' && !currentContext) return null
    return {
      chapterId: chapter.id,
      chapterIndex: Number(unref(options.currentIndex) || 0),
      offset: options.getOffset(),
      percent: options.getPercent(),
      title: chapter.title,
      excerpt: options.getExcerpt?.() || '',
      ...(currentContext || {}),
      ...extra,
    }
  }

  function openForm(extra = {}) {
    const book = unref(options.book)
    const payload = currentPayload({ note: '', ...extra })
    if (!book?.id || !payload || !String(payload.excerpt || '').trim()) {
      return Promise.resolve({ saved: false })
    }
    return options.openForm(book, payload, { mode: 'create' })
  }

  function currentDraft() {
    const payload = currentPayload({ note: '' })
    return String(payload?.excerpt || '').trim() ? payload : null
  }

  function openNote() {
    return openForm()
  }

  function createCurrent() {
    return openForm()
  }

  function createFromSelectedText(text) {
    const context = options.getSelectedTextContext?.(text)
    if (!context?.excerpt) {
      options.onSelectedTextNotFound?.()
      return Promise.resolve({ saved: false, reason: 'context-not-found' })
    }
    return openForm(context)
  }

  return {
    createCurrent,
    createFromSelectedText,
    currentDraft,
    openNote,
  }
}
