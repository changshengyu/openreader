import { ref } from 'vue'
import {
  analyzeSourceCompatibility,
  importSourceCompatibilityHint,
  importSourceTags,
} from '../utils/bookSourceCompatibility.js'

export {
  analyzeSourceCompatibility,
  importSourceCompatibilityHint,
  importSourceTags,
  sourceCompatibilityMessage,
} from '../utils/bookSourceCompatibility.js'

export function useSourceTransfer(options) {
  const showRemote = ref(false)
  const remoteURL = ref('')
  const remoteLoading = ref(false)
  const sourceUploadRef = ref(null)
  const showImportPreview = ref(false)
  const importPreviewSources = ref([])
  const checkedImportSourceIndexes = ref([])
  const importCheckAll = ref(false)
  const importCheckIndeterminate = ref(false)
  const importPreviewSaving = ref(false)

  async function importFile(data) {
    const file = data.raw
    if (!file) return
    try {
      const list = parseImportSourceList(JSON.parse(await file.text()))
      if (!list.length) {
        options.onError(null, '书源文件错误')
        return
      }
      openImportPreview(list)
    } catch (error) {
      options.onError(error, '导入失败')
    }
  }

  function openSourceImportPicker() {
    const input = sourceUploadRef.value?.$el?.querySelector?.(
      'input[type="file"]',
    )
    if (input) {
      input.click()
      return
    }
    options.onInfo('请点击页面右上角“导入”选择书源 JSON 文件')
  }

  async function importRemote() {
    if (!remoteURL.value.trim()) return
    remoteLoading.value = true
    try {
      const { data: preview } = await options.previewRemoteSource(
        remoteURL.value.trim(),
      )
      const list = parseImportSourceList(preview.sources || [])
      if (!list.length) {
        options.onError(null, '远程订阅未识别到书源')
        return
      }
      showRemote.value = false
      remoteURL.value = ''
      openImportPreview(list)
    } catch (error) {
      options.onError(error, '远程导入失败')
    } finally {
      remoteLoading.value = false
    }
  }

  function openImportPreview(list) {
    importPreviewSources.value = list
    checkedImportSourceIndexes.value = selectableIndexes(list)
    updateImportCheckState()
    showImportPreview.value = true
    if (checkedImportSourceIndexes.value.length < list.length) {
      options.onInfo('部分使用 Javascript 或 WebView 的书源未默认勾选')
    }
  }

  function closeImportPreview() {
    showImportPreview.value = false
    importPreviewSources.value = []
    checkedImportSourceIndexes.value = []
    updateImportCheckState()
  }

  function toggleImportCheckAll(checked) {
    checkedImportSourceIndexes.value = checked
      ? selectableIndexes(importPreviewSources.value)
      : []
    updateImportCheckState()
    if (checked &&
      checkedImportSourceIndexes.value.length < importPreviewSources.value.length) {
      options.onInfo('部分使用 Javascript 或 WebView 的书源未勾选')
    }
  }

  function handleImportSelectionChange() {
    updateImportCheckState()
  }

  function updateImportCheckState() {
    const totalSelectable = selectableIndexes(importPreviewSources.value).length
    const selected = checkedImportSourceIndexes.value.length
    importCheckAll.value = totalSelectable > 0 && selected === totalSelectable
    importCheckIndeterminate.value = selected > 0 && selected < totalSelectable
  }

  async function saveSelectedImportSources() {
    if (!checkedImportSourceIndexes.value.length) {
      options.onWarning('请选择需要导入的源')
      return
    }
    importPreviewSaving.value = true
    try {
      const selectedSources = checkedImportSourceIndexes.value
        .map(index => importPreviewSources.value[index])
      const form = createSourceImportForm(selectedSources)
      const { data: result } = await options.importSources(form)
      options.onSuccess(sourceImportMessage(result))
      closeImportPreview()
      await options.reloadSources()
    } catch (error) {
      options.onError(error, '导入失败')
    } finally {
      importPreviewSaving.value = false
    }
  }

  async function exportSources() {
    try {
      const selectedIds = options.getSelection()
        .map(source => source.id)
        .filter(Boolean)
      const response = await options.exportSources(selectedIds)
      const filename = selectedIds.length
        ? 'bookSources-selected.json'
        : 'bookSources.json'
      options.download(response.data, filename)
      options.onSuccess(
        selectedIds.length
          ? `已导出 ${selectedIds.length} 个书源`
          : '已导出全部书源',
      )
    } catch (error) {
      options.onError(error, '导出失败')
    }
  }

  return {
    showRemote,
    remoteURL,
    remoteLoading,
    sourceUploadRef,
    showImportPreview,
    importPreviewSources,
    checkedImportSourceIndexes,
    importCheckAll,
    importCheckIndeterminate,
    importPreviewSaving,
    importFile,
    openSourceImportPicker,
    importRemote,
    openImportPreview,
    closeImportPreview,
    toggleImportCheckAll,
    handleImportSelectionChange,
    updateImportCheckState,
    importSourceName,
    importSourceURL,
    importSourceTags,
    importSourceCompatibilityHint,
    saveSelectedImportSources,
    exportSources,
  }
}

function selectableIndexes(sources) {
  return sources
    .map((source, index) => analyzeSourceCompatibility(source).blocking ? null : index)
    .filter(index => index !== null)
}

export function parseImportSourceList(value) {
  if (Array.isArray(value)) return value
  if (Array.isArray(value?.bookSources)) return value.bookSources
  if (Array.isArray(value?.sources)) return value.sources
  if (value?.name || value?.bookSourceName) return [value]
  return []
}

export function importSourceName(source) {
  return source?.name || source?.bookSourceName || ''
}

export function importSourceURL(source) {
  return source?.baseUrl || source?.bookSourceUrl || source?.searchUrl || ''
}

export function createSourceImportForm(sources) {
  const form = new FormData()
  form.append(
    'file',
    new Blob([JSON.stringify(sources)], { type: 'application/json' }),
    'bookSources.json',
  )
  return form
}

export function sourceImportMessage(result = {}) {
  const imported = result.imported || 0
  const updated = result.updated || 0
  const skipped = result.skipped || 0
  return `新增 ${imported} 个，更新 ${updated} 个${skipped ? `，跳过 ${skipped} 个` : ''}`
}
