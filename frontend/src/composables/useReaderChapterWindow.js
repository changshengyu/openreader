import { ref, unref } from 'vue'
import {
  adjacentReaderChapterIndex,
  readerChapterWindowExtension,
  readerChapterWindowIndexes,
  readerChapterWindowPrunePlan,
} from '../utils/readerChapterWindow.js'

export function useReaderChapterWindow(options) {
  const configuredNextSize = Number(options.nextSize)
  const nextSize = Math.max(0, Number.isFinite(configuredNextSize) ? configuredNextSize : 1)
  let scrollStartIndex = Math.max(0, Number(unref(options.currentIndex)) || 0)
  let extending = false
  let computeVersion = 0
  let generation = 0
  const busy = options.busy || ref(false)

  function scopeKey() {
    return [
      options.getScopeKey?.() || '',
      options.reader.mode || '',
      unref(options.isContinuousScrollRead) ? 'continuous' : 'single',
    ].join('|')
  }

  function beginTransaction() {
    generation += 1
    const transaction = {
      generation,
      scopeKey: scopeKey(),
    }
    busy.value = true
    return transaction
  }

  function isCurrentTransaction(transaction) {
    return Boolean(
      transaction
      && transaction.generation === generation
      && transaction.scopeKey === scopeKey(),
    )
  }

  function finishTransaction(transaction) {
    if (transaction?.generation === generation) busy.value = false
  }

  function invalidate() {
    generation += 1
    extending = false
    busy.value = false
  }

  function blockFor(index, data) {
    const chapterRows = unref(options.chapters)
    return options.makeChapterBlock(
      index,
      data?.chapter || chapterRows[index],
      data?.content || '',
    )
  }

  function errorBlockFor(index, error) {
    const message = options.formatError?.(error)
      || error?.message
      || String(error || '章节加载失败')
    return {
      ...blockFor(index, {
        chapter: unref(options.chapters)[index],
        content: `获取章节内容失败！\n${message}`,
      }),
      error: message,
    }
  }

  async function replaceBlocks(blocks, preserveAnchor = true, transaction = null) {
    if (transaction && !isCurrentTransaction(transaction)) return false
    const anchor = preserveAnchor ? options.captureScrollAnchor() : null
    if (transaction && !isCurrentTransaction(transaction)) return false
    options.chapterBlocks.value = blocks
    await options.restoreScrollAnchor(anchor)
    return !transaction || isCurrentTransaction(transaction)
  }

  async function compute(computeOptions = {}) {
    const transaction = beginTransaction()
    try {
      const chapterRows = unref(options.chapters)
      if (!chapterRows.length) {
        if (isCurrentTransaction(transaction)) options.chapterBlocks.value = []
        return
      }
      const anchorIndex = Number.isInteger(computeOptions.anchorIndex)
        ? computeOptions.anchorIndex
        : unref(options.currentIndex)
      scrollStartIndex = anchorIndex
      const version = ++computeVersion
      const activatedBlock = options.chapterBlocks.value.find(
        block => Number(block.index) === anchorIndex,
      )
      if (computeOptions.activate && isCurrentTransaction(transaction)) {
        options.currentIndex.value = anchorIndex
        options.chapter.value = chapterRows[anchorIndex]
          || (activatedBlock?.id
            ? { id: activatedBlock.id, title: activatedBlock.title, index: anchorIndex }
            : unref(options.chapter))
        options.content.value = activatedBlock?.content || unref(options.content)
      }
      const indexes = readerChapterWindowIndexes({
        mode: options.reader.mode,
        anchorIndex,
        startIndex: scrollStartIndex,
        totalChapters: chapterRows.length,
        nextSize: unref(options.isContinuousScrollRead) ? nextSize : 0,
      })
      const existing = new Map(
        options.chapterBlocks.value.map(block => [Number(block.index), block]),
      )
      if (!existing.has(anchorIndex) && anchorIndex === unref(options.currentIndex)) {
        existing.set(
          anchorIndex,
          blockFor(anchorIndex, {
            chapter: unref(options.chapter),
            content: unref(options.content),
          }),
        )
      }
      const immediate = indexes.map(index => existing.get(index)).filter(Boolean)
      if (
        isCurrentTransaction(transaction)
        && options.chapterBlocks.value.length <= 1
        && immediate.some(block => Number(block.index) === anchorIndex)
      ) {
        options.chapterBlocks.value = immediate
      }

      const loaded = await Promise.all(indexes.map(async index => {
        if (existing.has(index)) return existing.get(index)
        try {
          const data = await options.loadContent(index)
          return blockFor(index, data)
        } catch (error) {
          return errorBlockFor(index, error)
        }
      }))
      if (
        !isCurrentTransaction(transaction)
        || version !== computeVersion
        || unref(options.currentIndex) !== anchorIndex
      ) return
      const blocks = loaded.filter(Boolean)
      if (!blocks.some(block => block.index === anchorIndex)) return
      await replaceBlocks(blocks, !computeOptions.activate, transaction)
    } finally {
      finishTransaction(transaction)
    }
  }

  async function appendNext() {
    if (!unref(options.isContinuousScrollRead) || !options.chapterBlocks.value.length) return
    const transaction = beginTransaction()
    let stable = false
    try {
      const chapterRows = unref(options.chapters)
      const nextIndex = adjacentReaderChapterIndex({
        blocks: options.chapterBlocks.value,
        direction: 'next',
        totalChapters: chapterRows.length,
      })
      if (nextIndex === null) return
      if (options.chapterBlocks.value.some(block => block.index === nextIndex)) return
      let block
      try {
        const data = await options.loadContent(nextIndex)
        if (!isCurrentTransaction(transaction)) return
        block = blockFor(nextIndex, data)
      } catch (error) {
        if (!isCurrentTransaction(transaction)) return
        block = errorBlockFor(nextIndex, error)
      }
      const appended = [
        ...options.chapterBlocks.value,
        block,
      ]
      const plan = readerChapterWindowPrunePlan({
        blocks: appended,
        mode: options.reader.mode,
        currentIndex: unref(options.currentIndex),
        totalChapters: chapterRows.length,
      })
      stable = await replaceBlocks(plan.blocks, true, transaction)
    } finally {
      finishTransaction(transaction)
      if (stable && isCurrentTransaction(transaction)) options.onStable?.()
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
  }

  async function retry(index) {
    const targetIndex = Number(index)
    const blockIndex = options.chapterBlocks.value.findIndex(
      block => Number(block.index) === targetIndex,
    )
    if (blockIndex < 0) return
    const transaction = beginTransaction()
    try {
      const data = await options.loadContent(targetIndex, { refresh: true })
      if (!isCurrentTransaction(transaction)) return
      const blocks = [...options.chapterBlocks.value]
      blocks[blockIndex] = blockFor(targetIndex, data)
      await replaceBlocks(blocks, true, transaction)
    } catch (error) {
      if (!isCurrentTransaction(transaction)) return
      const blocks = [...options.chapterBlocks.value]
      blocks[blockIndex] = errorBlockFor(targetIndex, error)
      await replaceBlocks(blocks, true, transaction)
    } finally {
      finishTransaction(transaction)
    }
  }

  function maybeExtend() {
    const viewport = unref(options.contentEl)
    if (!unref(options.isContinuousScrollRead) || extending || !viewport) return
    const lastBlock = options.chapterBlocks.value[options.chapterBlocks.value.length - 1]
    if (lastBlock?.error) return
    const extension = readerChapterWindowExtension({
      mode: options.reader.mode,
      scrollTop: viewport.scrollTop,
      clientHeight: viewport.clientHeight,
      scrollHeight: viewport.scrollHeight,
    })
    if (!extension.next) return
    const nextIndex = adjacentReaderChapterIndex({
      blocks: options.chapterBlocks.value,
      direction: 'next',
      totalChapters: unref(options.chapters).length,
    })
    if (nextIndex === null) return
    extending = true
    appendNext()
      .catch(() => {})
      .finally(() => {
        extending = false
      })
  }

  return {
    appendNext,
    busy,
    compute,
    invalidate,
    maybeExtend,
    retry,
    syncCurrentChapter,
  }
}
