import { computed, reactive, ref, watch } from 'vue'
import {
  isEPUBLocalPath,
  isTextLocalPath,
} from '../utils/localBookToc.js'

export function useOverlayBookImport(options) {
  const importing = ref(false)
  const previewing = ref(false)
  const previewData = ref(null)
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
  const isText = computed(() => isTextLocalPath(draft.file?.name))
  const isEPUB = computed(() => isEPUBLocalPath(draft.file?.name))
  const supportsTocRule = computed(() => isText.value || isEPUB.value)

  function reset() {
    Object.assign(draft, {
      title: '',
      author: '',
      categoryIds: [],
      file: null,
      tocRule: '',
    })
    previewData.value = null
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
    draft.file = data.raw || null
    draft.title = ''
    draft.author = ''
    previewData.value = null
    importToken.value = ''
    if (isEPUB.value) draft.tocRule = 'spin+toc'
    else if (!isText.value) draft.tocRule = ''
    if (draft.file) return preview()
  }

  async function preview() {
    if (!draft.file) return
    previewing.value = true
    try {
      const { data } = await options.previewBook(draft.file, {
        title: draft.title,
        author: draft.author,
        tocRule: supportsTocRule.value ? draft.tocRule : '',
        ...(importToken.value ? { importToken: importToken.value } : {}),
      })
      previewData.value = data
      importToken.value = data.importToken || importToken.value
      if (!draft.title && data.title) draft.title = data.title
      if (!draft.author && data.author) draft.author = data.author
    } catch (error) {
      previewData.value = null
      importToken.value = error?.response?.data?.importToken || importToken.value
      options.onError(error, '解析书籍失败')
    } finally {
      previewing.value = false
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
