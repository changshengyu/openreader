import { reactive, ref, unref } from 'vue'
import {
  bookmarkReaderQuery,
  normalizeImportedBookmarks,
  parseBookmarkPercent,
} from '../utils/bookmark.js'

export function useReaderBookmarkActions(options) {
  const noteVisible = ref(false)
  const noteText = ref('')
  const editorVisible = ref(false)
  const editingBookmark = ref(null)
  const draft = reactive({ title: '', excerpt: '', note: '' })

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

  async function removeOne(bookmark) {
    if (!bookmark?.id) return
    await options.remove(bookmark.id)
  }

  async function removeMany(rows) {
    const selected = Array.isArray(rows) ? rows : []
    if (!selected.length) return []
    try {
      await options.confirm(
        `确认要删除所选择的 ${selected.length} 条书签吗？`,
        '批量删除书签',
        { type: 'warning' },
      )
      const deleted = await options.removeMany(selected)
      options.onSuccess?.('书签已删除')
      return deleted
    } catch (error) {
      if (isDialogCancellation(error)) return []
      options.onError?.(error, '批量删除书签失败')
      return []
    }
  }

  async function importRows(rows) {
    const payloads = normalizeImportedBookmarks(rows)
    if (!payloads.length) {
      options.onError?.(null, '书签文件没有可导入内容')
      return []
    }
    try {
      await options.confirm(
        `确认要导入文件中的 ${payloads.length} 条书签到当前书籍吗？`,
        '导入书签',
        { type: 'info' },
      )
      const created = await options.importPayloads(payloads)
      options.onSuccess?.(`已导入 ${created.length} 条书签`)
      return created
    } catch (error) {
      if (isDialogCancellation(error)) return []
      options.onError?.(error, '导入书签失败')
      return []
    }
  }

  function openEditor(bookmark) {
    editingBookmark.value = bookmark
    Object.assign(draft, {
      title: bookmark?.title || '',
      excerpt: bookmark?.excerpt || '',
      note: bookmark?.note || '',
    })
    editorVisible.value = true
  }

  async function saveEdit() {
    if (!editingBookmark.value?.id) return null
    try {
      const updated = await options.update(editingBookmark.value.id, {
        title: draft.title,
        excerpt: draft.excerpt,
        note: draft.note,
      })
      editorVisible.value = false
      showToast('书签已更新')
      return updated
    } catch (error) {
      options.onError?.(error, '更新书签失败')
      return null
    }
  }

  async function jump(bookmark) {
    options.closeDrawer?.()
    const query = bookmarkReaderQuery(bookmark)
    if (bookmark?.chapterIndex === unref(options.currentIndex)) {
      await options.reloadCurrent({
        offset: Number(query.offset || 0),
        percent: parseBookmarkPercent(query.percent),
      })
      return
    }
    await options.navigate(query)
  }

  return {
    draft,
    editingBookmark,
    editorVisible,
    noteText,
    noteVisible,
    createCurrent,
    createFromSelectedText,
    importRows,
    jump,
    openEditor,
    openNote,
    removeMany,
    removeOne,
    saveEdit,
    saveNote,
  }
}

function isDialogCancellation(error) {
  return error === 'cancel' || error === 'close'
}
