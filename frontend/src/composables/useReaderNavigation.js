import { unref } from 'vue'
import {
  readerChapterBoundaryScrollTop,
  readerParagraphAtPosition,
  READER_CHAPTER_END_OFFSET,
  restoredReaderFlipPage,
  restoredReaderSingleChapterScrollTop,
} from '../utils/readerPosition.js'
import { createReaderScrollAnimator } from '../utils/readerAnimation.js'

export function useReaderNavigation(options) {
  const scrollAnimator = options.scrollAnimator || createReaderScrollAnimator()
  let activeVerticalDirection = 0
  let queuedVerticalDirection = 0
  let animationGeneration = 0

  function cancelPageAnimation() {
    animationGeneration += 1
    activeVerticalDirection = 0
    queuedVerticalDirection = 0
    scrollAnimator.cancel()
  }

  function verticalAnimationOptions() {
    if (
      options.getMode() !== 'page'
      || !options.useFastPageAnimation?.()
    ) return undefined
    return { easing: 'fast' }
  }

  function queueActiveVerticalPage(direction) {
    if (!scrollAnimator.isActive()) return false
    queuedVerticalDirection = activeVerticalDirection === direction ? direction : 0
    return true
  }

  function runVerticalPageAnimation(element, direction) {
    if (queueActiveVerticalPage(direction)) return true
    const generation = animationGeneration
    activeVerticalDirection = direction
    const started = scrollAnimator.scrollBy(
      element,
      direction * options.scrollStep(),
      options.getAnimateDuration(),
      () => {
        if (generation !== animationGeneration) return
        activeVerticalDirection = 0
        if (options.onVerticalPageSettled) {
          options.onVerticalPageSettled()
        } else {
          options.progressVersion.value += 1
          options.scheduleProgressSave(60)
        }
        const queuedDirection = queuedVerticalDirection
        queuedVerticalDirection = 0
        if (queuedDirection !== direction) return
        queueMicrotask(() => {
          if (generation !== animationGeneration) return
          if (direction > 0) void nextPage()
          else void previousPage()
        })
      },
      verticalAnimationOptions(),
    )
    if (!started) activeVerticalDirection = 0
    return started
  }

  function targetChapterIndex(index) {
    return Math.max(
      0,
      Math.min(Number(index), Math.max(options.chapters.value.length - 1, 0)),
    )
  }

  function paragraphByChapterPosition(chapterEl, position) {
    if (!chapterEl || !Number.isFinite(position) || position <= 0) return null
    const nodes = [
      ...chapterEl.querySelectorAll('h1[data-pos], [data-reader-block][data-pos]'),
    ]
    return readerParagraphAtPosition(nodes, position)
  }

  function animateContentTo(top) {
    const element = options.contentEl.value
    if (!element) return false
    return scrollAnimator.scrollTo(
      element,
      top,
      options.getAnimateDuration(),
    )
  }

  function jumpToLoadedChapter(index, offset = 0) {
    if (!options.contentEl.value || !options.contentBody.value) return false
    const targetIndex = targetChapterIndex(index)
    const chapterEl = options.contentBody.value
      .querySelector(`.chapter-content[data-index="${targetIndex}"]`)
    if (!chapterEl) return false
    const block = options.chapterBlocks.value.find(item => item.index === targetIndex)
    options.currentIndex.value = targetIndex
    options.chapter.value = options.chapters.value[targetIndex]
      || (block?.id ? { id: block.id, title: block.title, index: targetIndex } : options.chapter.value)
    options.content.value = block?.content || options.content.value

    if (Number(offset) === READER_CHAPTER_END_OFFSET) {
      animateContentTo(
        readerChapterBoundaryScrollTop({
          chapterTop: chapterEl.offsetTop,
          chapterHeight: chapterEl.offsetHeight,
          clientHeight: options.contentEl.value.clientHeight,
          end: true,
        }),
      )
    } else if (offset > 0) {
      const target = paragraphByChapterPosition(chapterEl, offset)
      if (target) {
        options.jumpToParagraph(target, { save: false, flash: false })
      } else {
        animateContentTo(
          readerChapterBoundaryScrollTop({
            chapterTop: chapterEl.offsetTop,
            chapterHeight: chapterEl.offsetHeight,
            clientHeight: options.contentEl.value.clientHeight,
            end: false,
          }),
        )
      }
    } else {
      animateContentTo(
        readerChapterBoundaryScrollTop({
          chapterTop: chapterEl.offsetTop,
          chapterHeight: chapterEl.offsetHeight,
          clientHeight: options.contentEl.value.clientHeight,
          end: false,
        }),
      )
    }
    options.progressVersion.value += 1
    options.scheduleProgressSave(Math.max(300, options.getAnimateDuration() + 80))
    return true
  }

  function jumpWithinCurrentChapter(offset = 0) {
    if (options.getMode() === 'flip') {
      options.page.value = restoredReaderFlipPage({
        offset: Number(offset) === READER_CHAPTER_END_OFFSET
          ? READER_CHAPTER_END_OFFSET
          : 0,
        percent: null,
        pageCount: options.pageCount.value,
      })
      options.progressVersion.value += 1
      options.saveProgress()
      return
    }
    if (jumpToLoadedChapter(options.currentIndex.value, offset)) return
    if (!options.contentEl.value) return
    animateContentTo(
      restoredReaderSingleChapterScrollTop({
        offset: Number(offset) === READER_CHAPTER_END_OFFSET
          ? READER_CHAPTER_END_OFFSET
          : 0,
        percent: null,
        scrollHeight: options.contentEl.value.scrollHeight,
        clientHeight: options.contentEl.value.clientHeight,
      }),
    )
    options.progressVersion.value += 1
    options.saveProgress()
  }

  async function goChapter(index, offset = 0) {
    cancelPageAnimation()
    const targetIndex = targetChapterIndex(index)
    if (targetIndex === options.currentIndex.value) {
      options.closeToc?.()
      jumpWithinCurrentChapter(offset)
      return
    }
    if (unref(options.isContinuousScrollRead)) {
      const loaded = options.contentBody.value
        ?.querySelector(`.chapter-content[data-index="${targetIndex}"]`)
      if (loaded) {
        await options.rebuildContinuousWindow?.(targetIndex)
        if (jumpToLoadedChapter(targetIndex, offset)) {
          options.closeToc?.()
          return
        }
      }
    }
    const query = { chapter: targetIndex }
    if (offset) query.offset = offset
    await options.navigate(query)
  }

  async function previousPage() {
    if (options.getMode() === 'flip' && options.page.value > 0) {
      options.page.value -= 1
      options.progressVersion.value += 1
      options.saveProgress()
      return
    }
    if (unref(options.isVerticalRead) && options.contentEl.value) {
      const el = options.contentEl.value
      if (el.scrollTop > 8) {
        runVerticalPageAnimation(el, -1)
        return
      }
    }
    if (options.currentIndex.value > 0) {
      await goChapter(options.currentIndex.value - 1, READER_CHAPTER_END_OFFSET)
    }
  }

  async function nextPage() {
    if (
      options.getMode() === 'flip'
      && options.page.value < options.pageCount.value - 1
    ) {
      options.page.value += 1
      options.progressVersion.value += 1
      options.saveProgress()
      return
    }
    if (unref(options.isVerticalRead) && options.contentEl.value) {
      const el = options.contentEl.value
      const bottom = el.scrollHeight - el.clientHeight
      if (el.scrollTop < bottom - 8) {
        runVerticalPageAnimation(el, 1)
        return
      }
    }
    if (options.currentIndex.value < options.chapters.value.length - 1) {
      await goChapter(options.currentIndex.value + 1)
    }
  }

  function scrollToTop() {
    cancelPageAnimation()
    if (options.getMode() === 'flip') {
      options.page.value = 0
    } else if (options.contentEl.value) {
      options.contentEl.value.scrollTop = 0
    } else {
      return
    }
    options.progressVersion.value += 1
    options.saveProgress()
  }

  function scrollToBottom() {
    cancelPageAnimation()
    if (options.getMode() === 'flip') {
      options.page.value = Math.max(0, options.pageCount.value - 1)
    } else if (options.contentEl.value) {
      options.contentEl.value.scrollTop = Math.max(
        0,
        options.contentEl.value.scrollHeight - options.contentEl.value.clientHeight,
      )
    } else {
      return
    }
    options.progressVersion.value += 1
    options.saveProgress()
  }

  return {
    cancelPageAnimation,
    goChapter,
    jumpToLoadedChapter,
    jumpWithinCurrentChapter,
    nextPage,
    paragraphByChapterPosition,
    previousPage,
    scrollToBottom,
    scrollToTop,
  }
}
