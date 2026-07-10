import { ref, unref } from 'vue'

export function useReaderBookmarkActions(options) {
  const noteVisible = ref(false)
  const noteText = ref('')

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

  function showToast(message) {
    options.onToast?.(message)
  }

  function openNote() {
    noteText.value = ''
    noteVisible.value = true
  }

  async function createCurrent() {
    const payload = currentPayload()
    if (!payload) return null
    const created = await options.create(payload)
    showToast('书签已创建')
    return created
  }

  async function createFromSelectedText(text) {
    const payload = currentPayload({
      excerpt: String(text || '').trim().slice(0, 500),
    })
    if (!payload) return null
    const created = await options.create(payload)
    showToast('书签已创建')
    return created
  }

  async function saveNote() {
    const note = noteText.value.trim()
    if (!note) return null
    const payload = currentPayload({ note })
    if (!payload) return null
    const created = await options.create(payload)
    noteVisible.value = false
    showToast('笔记已保存')
    return created
  }

  return {
    noteText,
    noteVisible,
    createCurrent,
    createFromSelectedText,
    openNote,
    saveNote,
  }
}
