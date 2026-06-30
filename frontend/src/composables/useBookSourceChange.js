import { ref, unref } from 'vue'
import { changeBookSource } from '../api/books'
import {
  sourceCandidateChangePayload,
  sourceCandidateSourceId,
} from '../utils/sourceCandidate'

export function useBookSourceChange(options) {
  const changingSource = ref(null)

  async function change(source) {
    const currentBook = unref(options.book)
    const targetBookId = Number(unref(options.bookId) || currentBook?.id)
    if (!targetBookId || !currentBook || source?.current || changingSource.value !== null) return null

    changingSource.value = sourceCandidateSourceId(source)
    options.onStart?.(source, currentBook)
    try {
      const { data } = await changeBookSource(
        targetBookId,
        sourceCandidateChangePayload(source, currentBook.title),
      )
      if (String(unref(options.bookId) || unref(options.book)?.id) !== String(targetBookId)) {
        return data
      }
      await options.onChanged?.({
        book: data,
        source,
        previousBook: currentBook,
      })
      options.onSuccess?.(data, source)
      return data
    } catch (error) {
      options.onError?.(error)
      return null
    } finally {
      changingSource.value = null
    }
  }

  return {
    changingSource,
    change,
  }
}
