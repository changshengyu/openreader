import { nextTick, unref } from 'vue'
import {
  adjacentReaderChapterIndex,
  readerChapterWindowExtension,
  readerChapterWindowIndexes,
  readerChapterWindowPrunePlan,
} from '../utils/readerChapterWindow.js'

export function useReaderChapterWindow(options) {
  const previousSize = Math.max(0, Number(options.previousSize) || 0)
  const nextSize = Math.max(0, Number(options.nextSize) || 0)
  let extending = false

  async function compute(computeOptions = {}) {
    const chapterRows = unref(options.chapters)
    if (!chapterRows.length) {
      options.chapterBlocks.value = []
      return
    }
    const anchorIndex = Number.isInteger(computeOptions.anchorIndex)
      ? computeOptions.anchorIndex
      : unref(options.currentIndex)
    const indexes = readerChapterWindowIndexes({
      mode: options.reader.mode,
      anchorIndex,
      totalChapters: chapterRows.length,
      previousSize,
      nextSize: unref(options.isContinuousScrollRead) ? nextSize : 0,
    })
    const rows = await Promise.all(indexes.map(async index => {
      try {
        const data = await options.loadContent(index)
        return options.makeChapterBlock(
          index,
          data.chapter || chapterRows[index],
          data.content || '',
        )
      } catch {
        return null
      }
    }))
    if (unref(options.currentIndex) !== anchorIndex) return
    const blocks = rows.filter(Boolean)
    if (!blocks.some(block => block.index === anchorIndex)) return
    const scrollAnchor = options.captureScrollAnchor()
    options.chapterBlocks.value = blocks
    await options.restoreScrollAnchor(scrollAnchor)
  }

  async function appendNext() {
    if (!unref(options.isContinuousScrollRead) || !options.chapterBlocks.value.length) return
    const chapterRows = unref(options.chapters)
    const nextIndex = adjacentReaderChapterIndex({
      blocks: options.chapterBlocks.value,
      direction: 'next',
      totalChapters: chapterRows.length,
    })
    if (nextIndex === null) return
    if (options.chapterBlocks.value.some(block => block.index === nextIndex)) return
    const data = await options.loadContent(nextIndex)
    options.chapterBlocks.value = [
      ...options.chapterBlocks.value,
      options.makeChapterBlock(
        nextIndex,
        data.chapter || chapterRows[nextIndex],
        data.content || '',
      ),
    ]
  }

  async function prependPrevious() {
    const viewport = unref(options.contentEl)
    if (
      options.reader.mode !== 'scroll2'
      || !options.chapterBlocks.value.length
      || !viewport
    ) return
    const chapterRows = unref(options.chapters)
    const previousIndex = adjacentReaderChapterIndex({
      blocks: options.chapterBlocks.value,
      direction: 'previous',
      totalChapters: chapterRows.length,
    })
    if (previousIndex === null) return
    if (options.chapterBlocks.value.some(block => block.index === previousIndex)) return
    const beforeHeight = viewport.scrollHeight
    const beforeTop = viewport.scrollTop
    const data = await options.loadContent(previousIndex)
    options.chapterBlocks.value = [
      options.makeChapterBlock(
        previousIndex,
        data.chapter || chapterRows[previousIndex],
        data.content || '',
      ),
      ...options.chapterBlocks.value,
    ]
    await nextTick()
    await options.nextFrame()
    const heightDelta = Math.max(0, viewport.scrollHeight - beforeHeight)
    viewport.scrollTop = beforeTop + heightDelta
  }

  function prune() {
    const viewport = unref(options.contentEl)
    if (
      options.reader.mode !== 'scroll2'
      || !viewport
      || !options.chapterBlocks.value.length
    ) return
    const plan = readerChapterWindowPrunePlan({
      blocks: options.chapterBlocks.value,
      currentIndex: unref(options.currentIndex),
      totalChapters: unref(options.chapters).length,
      previousSize,
      nextSize,
    })
    if (!plan.changed) return
    const body = unref(options.contentBody)
    const removedBeforeHeight = plan.removedBeforeIndexes.reduce((sum, index) => {
      const element = body?.querySelector(`.chapter-content[data-index="${index}"]`)
      return sum + (element?.getBoundingClientRect?.().height || 0)
    }, 0)
    const beforeTop = viewport.scrollTop
    options.chapterBlocks.value = plan.blocks
    if (removedBeforeHeight > 0) {
      nextTick(() => {
        const currentViewport = unref(options.contentEl)
        if (!currentViewport) return
        currentViewport.scrollTop = Math.max(0, beforeTop - removedBeforeHeight)
      })
    }
  }

  function syncCurrentChapter() {
    if (!unref(options.isContinuousScrollRead)) return
    const snapshot = options.visibleProgressSnapshot()
    const nextIndex = Number(snapshot?.chapterIndex)
    if (!Number.isInteger(nextIndex) || nextIndex === unref(options.currentIndex)) return
    const block = options.chapterBlocks.value.find(item => item.index === nextIndex)
    options.currentIndex.value = nextIndex
    options.chapter.value = snapshot?.chapter
      || unref(options.chapters)[nextIndex]
      || (block?.id
        ? { id: block.id, title: block.title, index: nextIndex }
        : options.chapter.value)
    options.content.value = block?.content || options.content.value
    prune()
  }

  function maybeExtend() {
    const viewport = unref(options.contentEl)
    if (!unref(options.isContinuousScrollRead) || extending || !viewport) return
    const extension = readerChapterWindowExtension({
      mode: options.reader.mode,
      scrollTop: viewport.scrollTop,
      clientHeight: viewport.clientHeight,
      scrollHeight: viewport.scrollHeight,
    })
    if (!extension.previous && !extension.next) return
    extending = true
    Promise.all([
      extension.previous ? prependPrevious() : Promise.resolve(),
      extension.next ? appendNext() : Promise.resolve(),
    ])
      .catch(() => {})
      .finally(() => {
        extending = false
      })
  }

  return {
    appendNext,
    compute,
    maybeExtend,
    prependPrevious,
    prune,
    syncCurrentChapter,
  }
}
