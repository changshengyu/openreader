import { unref } from 'vue'
import {
  invalidateReaderDataCache,
  readerDataCacheKey,
  writeReaderDataCache,
} from '../utils/readerDataCache.js'

export function useReaderBookState(options) {
  const makeCacheKey = options.makeCacheKey || readerDataCacheKey
  const invalidateCache = options.invalidateCache || invalidateReaderDataCache
  const writeCache = options.writeCache || writeReaderDataCache

  function mergeLoadedBook(incoming) {
    if (!incoming?.id) return incoming
    const current = options.bookshelf.books.find(
      item => Number(item.id) === Number(incoming.id),
    ) || (
      Number(unref(options.book)?.id) === Number(incoming.id)
        ? unref(options.book)
        : null
    )
    return options.mergeBook(current, incoming)
  }

  function cacheKey(key) {
    const [type, targetBookId] = String(key || '').split(':')
    return makeCacheKey(
      targetBookId || unref(options.bookId),
      type || key,
    )
  }

  async function invalidate(optionsOverride = {}) {
    const targetBookId = optionsOverride.bookId || unref(options.bookId)
    await invalidateCache(targetBookId, optionsOverride)
  }

  async function write(optionsOverride = {}) {
    const targetBookId = optionsOverride.bookId || unref(options.bookId)
    await writeCache(targetBookId, optionsOverride)
  }

  return {
    cacheKey,
    invalidate,
    mergeLoadedBook,
    write,
  }
}
