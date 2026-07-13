import { computed, ref } from 'vue'
import { isEPUBLocalPath } from '../utils/localBookToc.js'

export function useStorageImportWorkflow(options) {
  const phase = ref('idle')
  const source = ref('')
  const rows = ref([])
  const queue = ref([])
  const currentIndex = ref(0)
  const batchCategoryIds = ref([])
  const busy = ref(false)
  const summary = ref(emptySummary())

  const validRows = computed(() => rows.value.filter(row => row.valid && !row.error))
  const retryRows = computed(() => rows.value.filter(row => !row.valid || row.error))
  const currentRow = computed(() => queue.value[currentIndex.value] || null)
  const currentLabel = computed(() => queue.value.length > 1
    ? `（${currentIndex.value + 1}/${queue.value.length}）`
    : '')

  async function start(request) {
    reset()
    source.value = String(request?.source || '')
    const paths = Array.isArray(request?.paths) ? request.paths.filter(Boolean) : []
    if (!source.value || !paths.length) return false

    phase.value = 'loading'
    try {
      await options.loadCategories?.()
      const response = await options.preview(source.value, paths)
      rows.value = normalizePreviewRows(response?.items || [])
      chooseInitialPhase()
      return true
    } catch (error) {
      phase.value = 'idle'
      options.onError?.(safeError(error, '解析导入预览失败'))
      return false
    }
  }

  function continueAfterPreflight() {
    if (!validRows.value.length) return false
    beginConfirmation(validRows.value)
    return true
  }

  function chooseBatch() {
    if (phase.value !== 'choose-mode') return
    batchCategoryIds.value = []
    phase.value = 'batch-groups'
  }

  function chooseSequential() {
    if (phase.value !== 'choose-mode') return
    queue.value = [...validRows.value]
    currentIndex.value = 0
    phase.value = 'single'
  }

  function cancelMode() {
    if (phase.value !== 'choose-mode') return
    cancelAll()
  }

  function cancelBatchGroups() {
    if (phase.value !== 'batch-groups') return
    cancelAll()
  }

  async function confirmBatch() {
    if (phase.value !== 'batch-groups' || busy.value) return false
    busy.value = true
    const categoryIds = normalizeCategoryIds(batchCategoryIds.value)
    try {
      for (const row of validRows.value) {
        const outcome = await importRow(row, categoryIds)
        if (outcome.ok) {
          summary.value.succeeded += 1
        } else {
          summary.value.failed += 1
        }
      }
      completeFlow()
      return true
    } finally {
      busy.value = false
    }
  }

  async function confirmCurrent() {
    const row = currentRow.value
    if (phase.value !== 'single' || !row || busy.value) return false
    busy.value = true
    try {
      const outcome = await importRow(row, row.categoryIds)
      if (!outcome.ok) return false
      summary.value.succeeded += 1
      advanceCurrent()
      return true
    } finally {
      busy.value = false
    }
  }

  function skipCurrent() {
    if (phase.value !== 'single' || !currentRow.value || busy.value) return
    summary.value.skipped += 1
    advanceCurrent()
  }

  async function reparse(row) {
    if (!row?.importToken || busy.value) return false
    row.reparsing = true
    try {
      const response = await options.preview(source.value, [toReparsePayload(row)])
      const result = (response?.items || []).find(item => item.path === row.path)
      applyPreviewResult(row, result || {
        path: row.path,
        importToken: row.importToken,
        error: '重新解析未返回结果',
      })
      return !row.error
    } catch (error) {
      applyPreviewResult(row, {
        path: row.path,
        importToken: row.importToken,
        error: safeError(error, '重新解析失败'),
      })
      return false
    } finally {
      row.reparsing = false
    }
  }

  function cancelAll() {
    const cancelled = { ...summary.value }
    reset()
    options.onCancel?.(cancelled)
  }

  function reset() {
    phase.value = 'idle'
    source.value = ''
    rows.value = []
    queue.value = []
    currentIndex.value = 0
    batchCategoryIds.value = []
    busy.value = false
    summary.value = emptySummary()
  }

  function chooseInitialPhase() {
    if (!validRows.value.length || retryRows.value.length) {
      phase.value = 'preflight'
      return
    }
    beginConfirmation(validRows.value)
  }

  function beginConfirmation(nextRows) {
    queue.value = [...nextRows]
    currentIndex.value = 0
    batchCategoryIds.value = []
    phase.value = queue.value.length === 1 ? 'single' : 'choose-mode'
  }

  function advanceCurrent() {
    if (currentIndex.value + 1 < queue.value.length) {
      currentIndex.value += 1
      return
    }
    completeFlow()
  }

  async function importRow(row, categoryIds) {
    row.lastError = ''
    try {
      const response = await options.importItem(source.value, toImportPayload(row), normalizeCategoryIds(categoryIds))
      const result = (response?.imported || []).find(item => item.path === row.path) || response?.imported?.[0]
      if (!result?.book) {
        row.lastError = result?.error || '导入失败'
        options.onError?.(row.lastError)
        return { ok: false, error: row.lastError }
      }
      row.imported = true
      options.onImported?.(result.book)
      return { ok: true, book: result.book }
    } catch (error) {
      row.lastError = safeError(error, '导入失败')
      options.onError?.(row.lastError)
      return { ok: false, error: row.lastError }
    }
  }

  function completeFlow() {
    const completed = { ...summary.value }
    phase.value = 'idle'
    options.onComplete?.(completed)
  }

  return {
    phase,
    source,
    rows,
    validRows,
    retryRows,
    queue,
    currentIndex,
    currentRow,
    currentLabel,
    batchCategoryIds,
    busy,
    summary,
    start,
    continueAfterPreflight,
    chooseBatch,
    chooseSequential,
    cancelMode,
    cancelBatchGroups,
    confirmBatch,
    confirmCurrent,
    skipCurrent,
    reparse,
    cancelAll,
    reset,
  }
}

function normalizePreviewRows(items) {
  return items.map(item => {
    const book = item?.book || {}
    const error = String(item?.error || '')
    return {
      path: String(item?.path || ''),
      importToken: String(item?.importToken || book.importToken || ''),
      title: String(book.title || item?.title || fileNameWithoutExtension(item?.path) || ''),
      author: String(book.author || item?.author || ''),
      tocRule: String(item?.tocRule || (isEPUBLocalPath(item?.path) ? 'spin+toc' : '')),
      chapterCount: Number(book.chapterCount || 0),
      chapters: Array.isArray(book.chapters) ? book.chapters : [],
      categoryIds: [],
      error,
      lastError: '',
      valid: Boolean(item?.book) && !error,
      imported: false,
      reparsing: false,
    }
  })
}

function applyPreviewResult(row, result) {
  const book = result?.book || {}
  row.importToken = String(result?.importToken || book.importToken || row.importToken || '')
  row.error = String(result?.error || '')
  row.lastError = ''
  row.valid = Boolean(result?.book) && !row.error
  if (result?.book) {
    row.title = String(book.title || row.title)
    row.author = String(book.author || row.author)
    row.chapterCount = Number(book.chapterCount || 0)
    row.chapters = Array.isArray(book.chapters) ? book.chapters : []
  }
}

function toReparsePayload(row) {
  return {
    path: row.path,
    importToken: row.importToken,
    title: row.title.trim(),
    author: row.author.trim(),
    tocRule: row.tocRule || '',
  }
}

function toImportPayload(row) {
  return {
    path: row.path,
    importToken: row.importToken,
    title: row.title.trim(),
    author: row.author.trim(),
    tocRule: row.tocRule || '',
  }
}

function normalizeCategoryIds(categoryIds) {
  const values = Array.isArray(categoryIds) ? categoryIds : [categoryIds]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}

function fileNameWithoutExtension(path) {
  return String(path || '').split('/').pop()?.replace(/\.[^.]+$/, '') || ''
}

function emptySummary() {
  return { succeeded: 0, failed: 0, skipped: 0 }
}

function safeError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || error?.message || fallback
}
