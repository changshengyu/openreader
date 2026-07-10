import { unref } from 'vue'

export function useReaderBookmarkActions(options) {
  function currentPayload(extra = {}) {
    const chapter = unref(options.chapter)
    if (!chapter) return null
    return {
      chapterId: chapter.id,
      chapterIndex: Number(unref(options.currentIndex) || 0),
      offset: options.getOffset(),
      percent: options.getPercent(),
      title: chapter.title,
      excerpt: options.getExcerpt(),
      ...extra,
    }
  }

  function openForm(extra = {}) {
    const book = unref(options.book)
    const payload = currentPayload({ note: '', ...extra })
    if (!book?.id || !payload) return Promise.resolve({ saved: false })
    return options.openForm(book, payload, { mode: 'create' })
  }

  function openNote() {
    return openForm()
  }

  function createCurrent() {
    return openForm()
  }

  function createFromSelectedText(text) {
    return openForm({
      excerpt: String(text || '').trim().slice(0, 500),
    })
  }

  return {
    createCurrent,
    createFromSelectedText,
    openNote,
  }
}
