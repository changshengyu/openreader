import { onBeforeUnmount, ref, unref } from 'vue'
import { nextReaderBlock, paragraphAutoReadDelay } from '../utils/readerAutoReading.js'

export function useAutoReading(options) {
  const active = ref(false)
  let timer = null
  let advancing = false

  function settings() {
    return options.settings?.() || {}
  }

  function notify(message) {
    options.onNotify?.(message)
  }

  function toggle() {
    if (active.value) {
      stop()
      notify('自动阅读已停止')
      return
    }
    active.value = true
    schedule()
    notify('自动阅读已开始')
  }

  function schedule(delay = 0) {
    clearTimeout(timer)
    if (!active.value) return
    timer = setTimeout(runStep, Math.max(0, Number(delay) || 0))
  }

  async function runStep() {
    if (!active.value) return
    if (advancing || options.shouldPause?.()) {
      schedule(300)
      return
    }
    advancing = true
    try {
      if (settings().method === '段落滚动') {
        await readByParagraph()
      } else {
        await readByPixel()
      }
    } finally {
      advancing = false
    }
  }

  async function readByPixel() {
    const currentSettings = settings()
    const content = unref(options.contentEl)
    if (unref(options.isVerticalRead) && content) {
      const bottom = Math.max(0, content.scrollHeight - content.clientHeight)
      if (content.scrollTop < bottom - 4) {
        content.scrollTop = Math.min(bottom, content.scrollTop + currentSettings.pixel)
        schedule(currentSettings.interval)
        return
      }
    }
    await advanceOrStop(currentSettings.interval)
  }

  async function readByParagraph() {
    const currentSettings = settings()
    const content = unref(options.contentEl)
    const body = unref(options.contentBody)
    if (!unref(options.isVerticalRead) || !content || !body) {
      await advanceOrStop(currentSettings.interval)
      return
    }

    const current = options.currentVisibleParagraph?.()
    const next = nextReaderBlock(body, current)
    if (!next) {
      await advanceOrStop(currentSettings.interval)
      return
    }

    const viewport = content.getBoundingClientRect()
    const rect = next.getBoundingClientRect()
    const nextTop = content.scrollTop + rect.top - viewport.top - 24
    content.scrollTo({
      top: Math.max(0, nextTop),
      behavior: options.scrollBehavior?.() || 'auto',
    })
    options.onProgress?.()

    const currentHeight = current?.getBoundingClientRect?.().height
    schedule(paragraphAutoReadDelay({
      paragraphHeight: currentHeight,
      fontSize: currentSettings.fontSize,
      lineHeight: currentSettings.lineHeight,
      baseDelay: currentSettings.interval,
    }))
  }

  async function advanceOrStop(delay) {
    const advanced = await options.advancePage?.()
    if (advanced) {
      schedule(delay)
      return
    }
    stop()
    notify('已到本书末尾')
  }

  function stop() {
    active.value = false
    advancing = false
    clearTimeout(timer)
    timer = null
  }

  onBeforeUnmount(stop)

  return {
    active,
    stop,
    toggle,
  }
}
