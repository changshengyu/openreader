import { computed, nextTick, onBeforeUnmount, ref, unref } from 'vue'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'

export function useReaderShelf(options) {
  const visible = ref(false)
  const loading = ref(false)
  const panelRef = ref(null)
  let locateTimer = null

  const books = computed(() => {
    const rows = Array.isArray(options.bookshelf.books) ? options.bookshelf.books : []
    return sortByShelfOrder(rows, options.reader.progressByBook)
  })

  async function open() {
    options.beforeOpen?.()
    visible.value = true
    if (options.bookshelf.books.length) {
      scheduleLocate(0)
      return
    }
    loading.value = true
    try {
      await options.bookshelf.ensureBooksLoaded({ all: true })
      locateCurrentBook()
    } catch (error) {
      options.onError?.(error, '加载书架失败')
    } finally {
      loading.value = false
    }
  }

  function locateCurrentBook(attempt = 0) {
    clearLocateTimer()
    nextTick(() => {
      const panel = panelRef.value
      if (panel?.locateCurrentBook) {
        panel.locateCurrentBook()
        return
      }
      if (attempt < 20 && visible.value && books.value.length) {
        locateTimer = setTimeout(() => locateCurrentBook(attempt + 1), 50)
      }
    })
  }

  function scheduleLocate(delay) {
    clearLocateTimer()
    locateTimer = setTimeout(() => {
      locateTimer = null
      locateCurrentBook()
    }, Math.max(0, Number(delay) || 0))
  }

  function clearLocateTimer() {
    if (!locateTimer) return
    clearTimeout(locateTimer)
    locateTimer = null
  }

  async function select(item) {
    visible.value = false
    if (Number(item?.id) === Number(unref(options.currentBookId))) return
    await options.saveProgress?.()
    const progress = newestBookProgress(item, options.reader.progressByBook)
    const fallbackCount = Number(options.currentChapterCount?.() || 0)
    await options.router.push({
      name: 'reader',
      params: { id: item.id },
      query: readerRouteQueryFromBook(item, progress, item?.chapterCount || fallbackCount),
    })
  }

  async function refresh() {
    loading.value = true
    try {
      await options.bookshelf.loadBooks({ force: true, all: true, settleProgress: true })
      locateCurrentBook()
    } catch (error) {
      options.onError?.(error, '刷新书架失败')
    } finally {
      loading.value = false
    }
  }

  onBeforeUnmount(clearLocateTimer)

  return {
    visible,
    loading,
    panelRef,
    books,
    open,
    locateCurrentBook,
    select,
    refresh,
  }
}
