import { computed, ref, unref } from 'vue'
import {
  clampReaderPercent,
  readerBookProgress,
  readerBookSeekTarget,
} from '../utils/readerPagination.js'

export function useReaderProgressControls(options) {
  const mobileBookSliderDraft = ref(null)

  const bookProgress = computed(() => readerBookProgress({
    chapterIndex: options.currentIndex.value,
    chapterPercent: options.getCurrentChapterPercent(),
    totalChapters: options.chapters.value.length,
  }))
  const bookProgressLabel = computed(() => `${Math.round(bookProgress.value * 100)}%`)
  const mobileBookSliderValue = computed(() => (
    mobileBookSliderDraft.value !== null
      ? mobileBookSliderDraft.value
      : Math.round(bookProgress.value * 1000)
  ))
  const mobileBookProgressLabel = computed(() => (
    `${Math.round(Number(mobileBookSliderValue.value || 0) / 10)}%`
  ))
  const desktopChapterSliderValue = computed(() => {
    options.progressVersion.value
    return Math.round(clampReaderPercent(options.getCurrentChapterPercent()) * 1000)
  })
  const desktopChapterProgressLabel = computed(() => (
    `${Math.round(desktopChapterSliderValue.value / 10)}%`
  ))

  function seekCurrentChapterPercent(percent, seekOptions = {}) {
    const value = clampReaderPercent(percent)
    if (options.getMode() === 'flip') {
      options.page.value = Math.round(value * Math.max(0, options.pageCount.value - 1))
      options.progressVersion.value += 1
      if (seekOptions.save !== false) options.saveProgress()
      return
    }
    if (!options.contentEl.value) return
    if (unref(options.isContinuousScrollRead)) {
      const chapterEl = options.contentBody.value
        ?.querySelector(`.chapter-content[data-index="${options.currentIndex.value}"]`)
      if (chapterEl) {
        const room = Math.max(
          chapterEl.offsetHeight - options.contentEl.value.clientHeight,
          0,
        )
        options.contentEl.value.scrollTop = Math.max(
          0,
          chapterEl.offsetTop + Math.round(value * room),
        )
      }
    } else {
      const bottom = Math.max(
        options.contentEl.value.scrollHeight - options.contentEl.value.clientHeight,
        0,
      )
      options.contentEl.value.scrollTop = Math.round(value * bottom)
    }
    options.progressVersion.value += 1
    options.applyLocalProgress()
    if (seekOptions.save === false) {
      options.scheduleProgressSave(500)
    } else {
      options.saveProgress()
    }
  }

  async function seekBookProgress(percent) {
    const target = readerBookSeekTarget(percent, options.chapters.value.length)
    if (target.chapterIndex === options.currentIndex.value) {
      seekCurrentChapterPercent(target.chapterPercent, { save: true })
      return
    }
    await options.navigate({
      chapter: target.chapterIndex,
      percent: target.chapterPercent,
    })
  }

  function handleDesktopProgressInput(event) {
    seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: false })
  }

  function handleDesktopProgressChange(event) {
    seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: true })
  }

  function handleMobileBookProgressInput(event) {
    mobileBookSliderDraft.value = Number(event.target.value || 0)
  }

  async function handleMobileBookProgressChange(event) {
    const value = Number(event.target.value || 0)
    mobileBookSliderDraft.value = value
    try {
      await seekBookProgress(value / 1000)
    } finally {
      mobileBookSliderDraft.value = null
    }
  }

  return {
    bookProgress,
    bookProgressLabel,
    desktopChapterProgressLabel,
    desktopChapterSliderValue,
    mobileBookProgressLabel,
    mobileBookSliderValue,
    handleDesktopProgressChange,
    handleDesktopProgressInput,
    handleMobileBookProgressChange,
    handleMobileBookProgressInput,
    seekBookProgress,
    seekCurrentChapterPercent,
  }
}
