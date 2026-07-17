import { computed, ref, unref } from 'vue'
import {
  clampReaderPercent,
  readerBookProgress,
} from '../utils/readerPagination.js'
import { createReaderScrollAnimator } from '../utils/readerAnimation.js'

export function useReaderProgressControls(options) {
  const mobilePageSliderDraft = ref(null)
  const scrollAnimator = options.scrollAnimator || createReaderScrollAnimator()

  const bookProgress = computed(() => readerBookProgress({
    chapterIndex: options.currentIndex.value,
    chapterPercent: options.getCurrentChapterPercent(),
    totalChapters: options.chapters.value.length,
  }))
  const bookProgressLabel = computed(() => `${Math.round(bookProgress.value * 100)}%`)
  const mobilePageSliderMax = computed(() => Math.max(1, Number(options.pageCount.value) || 1))
  const mobilePageSliderValue = computed(() => {
    const max = mobilePageSliderMax.value
    const value = mobilePageSliderDraft.value !== null
      ? mobilePageSliderDraft.value
      : Number(options.page.value || 0) + 1
    return Math.max(1, Math.min(max, Math.round(Number(value) || 1)))
  })
  const mobilePageProgressLabel = computed(() => (
    `第 ${mobilePageSliderValue.value}/${mobilePageSliderMax.value} 页`
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

  function handleDesktopProgressInput(event) {
    seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: false })
  }

  function handleDesktopProgressChange(event) {
    seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: true })
  }

  function normalizedMobilePage(value) {
    return Math.max(
      1,
      Math.min(mobilePageSliderMax.value, Math.round(Number(value) || 1)),
    )
  }

  function seekRenderedPage(pageNumber) {
    const target = normalizedMobilePage(pageNumber) - 1
    if (options.getMode() === 'flip') {
      options.page.value = target
      options.progressVersion.value += 1
      options.saveProgress()
      return
    }
    if (!options.contentEl.value) return
    const bottom = Math.max(
      options.contentEl.value.scrollHeight - options.contentEl.value.clientHeight,
      0,
    )
    const pageMax = Math.max(0, mobilePageSliderMax.value - 1)
    const targetTop = pageMax > 0
      ? Math.round((target / pageMax) * bottom)
      : 0
    options.page.value = target
    scrollAnimator.cancel()
    scrollAnimator.scrollTo(
      options.contentEl.value,
      targetTop,
      options.getAnimateDuration?.() || 0,
      () => {
        options.progressVersion.value += 1
        options.applyLocalProgress()
        options.saveProgress()
      },
    )
  }

  function handleMobilePageProgressInput(event) {
    mobilePageSliderDraft.value = normalizedMobilePage(event.target.value)
  }

  function handleMobilePageProgressChange(event) {
    mobilePageSliderDraft.value = normalizedMobilePage(event.target.value)
    try {
      seekRenderedPage(mobilePageSliderDraft.value)
    } finally {
      mobilePageSliderDraft.value = null
    }
  }

  return {
    bookProgress,
    bookProgressLabel,
    desktopChapterProgressLabel,
    desktopChapterSliderValue,
    mobilePageProgressLabel,
    mobilePageSliderMax,
    mobilePageSliderValue,
    handleDesktopProgressChange,
    handleDesktopProgressInput,
    handleMobilePageProgressChange,
    handleMobilePageProgressInput,
    seekRenderedPage,
    seekCurrentChapterPercent,
  }
}
