import {
  computed,
  getCurrentScope,
  onScopeDispose,
  reactive,
  ref,
  watch,
} from 'vue'
import {
  isDirectImportableLocalPath,
  isEPUBLocalPath,
  isTextLocalPath,
} from '../utils/localBookToc.js'

const unsupportedVisibleImportMessage = '仅支持 TXT / EPUB / UMD / CBZ 格式'

export function useOverlayBookImport(options) {
  const importing = ref(false)
  const previewing = ref(false)
  const previewData = ref(null)
  const previewError = ref('')
  const importToken = ref('')
  const draft = reactive({
    title: '',
    author: '',
    categoryIds: [],
    file: null,
    tocRule: '',
  })
  const tocRuleOptions = ref([])
  const tocRulesLoading = ref(false)
  let previewGeneration = 0
  let previewController = null
  const isText = computed(() => isTextLocalPath(draft.file?.name))
  const isEPUB = computed(() => isEPUBLocalPath(draft.file?.name))
  const supportsTocRule = computed(() => isText.value || isEPUB.value)

  function invalidatePreview() {
    previewGeneration += 1
    previewController?.abort()
    previewController = null
    previewing.value = false
  }

  function reset() {
    invalidatePreview()
    Object.assign(draft, {
      title: '',
      author: '',
      categoryIds: [],
      file: null,
      tocRule: '',
    })
    previewData.value = null
    previewError.value = ''
    importToken.value = ''
  }

  async function open() {
    try {
      await options.loadCategories()
    } catch (error) {
      options.onError(error, '加载分组失败')
    }
  }

  async function loadTocRules() {
    if (tocRuleOptions.value.length || tocRulesLoading.value) return
    tocRulesLoading.value = true
    try {
      const { data } = await options.listTocRules()
      tocRuleOptions.value = Array.isArray(data)
        ? data.filter(rule => rule?.enable !== false && rule?.rule)
        : []
    } catch (error) {
      options.onError(error, '加载目录规则失败')
    } finally {
      tocRulesLoading.value = false
    }
  }

  function pickFile(data) {
    const file = data.raw || null
    invalidatePreview()
    draft.file = null
    draft.title = ''
    draft.author = ''
    previewData.value = null
    importToken.value = ''
    if (file && !isDirectImportableLocalPath(file.name)) {
      options.onError(new Error('unsupported visible local import format'), unsupportedVisibleImportMessage)
      return
    }
    draft.file = file
    if (isEPUB.value) draft.tocRule = 'spin+toc'
    else if (!isText.value) draft.tocRule = ''
    if (draft.file) return preview()
  }

  async function preview() {
    if (!draft.file) return
    const generation = ++previewGeneration
    previewController?.abort()
    const controller = new AbortController()
    previewController = controller
    const previewFile = draft.file
    previewing.value = true
    previewError.value = ''
    try {
      const { data } = await options.previewBook(previewFile, {
        title: draft.title,
        author: draft.author,
        tocRule: supportsTocRule.value ? draft.tocRule : '',
        ...(importToken.value ? { importToken: importToken.value } : {}),
      }, { signal: controller.signal })
      if (generation !== previewGeneration) return
      previewData.value = data
      importToken.value = data.importToken || importToken.value
      if (!draft.title && data.title) draft.title = data.title
      if (!draft.author && data.author) draft.author = data.author
    } catch (error) {
      if (generation !== previewGeneration || isCancelledPreview(error)) return
      previewData.value = null
      importToken.value = error?.response?.data?.importToken || importToken.value
      previewError.value = importPreviewErrorMessage(error)
      options.onError(error, previewError.value)
    } finally {
      if (generation === previewGeneration) {
        previewing.value = false
        previewController = null
      }
    }
  }

  async function importBook() {
    if (!draft.file || !previewData.value) return
    importing.value = true
    try {
      const book = await options.importBook({
        file: draft.file,
        importToken: importToken.value,
        title: draft.title,
        author: draft.author,
        categoryIds: draft.categoryIds,
        tocRule: supportsTocRule.value ? draft.tocRule : '',
      })
      options.onSuccess(
        `已导入《${book.title}》，共 ${book.chapterCount || 0} 章`,
      )
      reset()
      options.close()
    } catch (error) {
      options.onError(error, '导入失败')
    } finally {
      importing.value = false
    }
  }

  watch(
    () => [isText.value, isEPUB.value],
    ([text, epub]) => {
      if (text) loadTocRules()
      else if (epub) draft.tocRule = 'spin+toc'
      else draft.tocRule = ''
    },
  )

  if (getCurrentScope()) {
    onScopeDispose(invalidatePreview)
  }

  watch(
    options.visible,
    visible => {
      if (!visible) reset()
    },
  )

  return {
    importing,
    previewing,
    previewData,
    previewError,
    importToken,
    draft,
    tocRuleOptions,
    tocRulesLoading,
    isText,
    isEPUB,
    supportsTocRule,
    open,
    loadTocRules,
    pickFile,
    preview,
    importBook,
    reset,
  }
}

function isCancelledPreview(error) {
  return error?.name === 'AbortError' ||
    error?.name === 'CanceledError' ||
    error?.code === 'ERR_CANCELED'
}

function importPreviewErrorMessage(error) {
  const message = String(error?.response?.data?.error || error?.message || '')
  if (message.includes('no readable chapters')) {
    return '未找到匹配的目录，请调整目录规则后重新解析'
  }
  return '解析书籍失败'
}
