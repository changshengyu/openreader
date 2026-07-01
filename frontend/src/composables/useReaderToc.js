import { nextTick, onBeforeUnmount, ref, unref } from 'vue'
import { readerTocTargetIndex, toggleReaderTocReverse } from '../utils/readerToc'

export function useReaderToc(options) {
  const visible = ref(false)
  const panelRef = ref(null)
  const locateKey = ref(0)
  const reverse = ref(false)
  const refreshing = ref(false)
  const locateTimers = new Set()

  function scheduleLocate(delay) {
    const timer = setTimeout(() => {
      locateTimers.delete(timer)
      locateCurrentChapter()
    }, Math.max(0, Number(delay) || 0))
    locateTimers.add(timer)
  }

  function clearLocateTimers() {
    locateTimers.forEach(timer => clearTimeout(timer))
    locateTimers.clear()
  }

  function open() {
    options.beforeOpen?.()
    options.refreshCachedChapters?.()
    visible.value = true
    clearLocateTimers()
    scheduleLocate(0)
    scheduleLocate(180)
  }

  function locateCurrentChapter() {
    options.syncCurrentChapter?.()
    locateKey.value += 1
    nextTick(() => panelRef.value?.locateCurrentChapter?.())
  }

  function toggleReverse() {
    reverse.value = toggleReaderTocReverse(reverse.value)
    locateCurrentChapter()
  }

  function scrollTop() {
    panelRef.value?.scrollToTop?.()
  }

  function scrollBottom() {
    panelRef.value?.scrollToBottom?.()
  }

  async function jump(index) {
    visible.value = false
    const targetIndex = readerTocTargetIndex(index, unref(options.chapters)?.length)
    await options.goChapter(targetIndex)
  }

  async function runRefreshing(task) {
    if (refreshing.value) return
    refreshing.value = true
    try {
      return await task()
    } finally {
      refreshing.value = false
    }
  }

  async function refresh() {
    return runRefreshing(async () => {
      if (unref(options.isRemoteBook)) {
        await options.refreshRemoteCatalog()
      } else {
        await options.refreshLocalCatalog()
      }
      await options.refreshCachedChapters?.()
      locateCurrentChapter()
    })
  }

  onBeforeUnmount(clearLocateTimers)

  return {
    visible,
    panelRef,
    locateKey,
    reverse,
    refreshing,
    open,
    locateCurrentChapter,
    toggleReverse,
    scrollTop,
    scrollBottom,
    jump,
    refresh,
    runRefreshing,
  }
}
