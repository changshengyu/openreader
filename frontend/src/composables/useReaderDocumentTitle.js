import { onScopeDispose, unref, watch } from 'vue'
import {
  SHELL_DOCUMENT_TITLE,
  formatReaderDocumentTitle,
  setDocumentTitle,
} from '../utils/documentTitle.js'
import { readerCatalogRow } from '../utils/readerCatalogRow.js'

// Upstream Reader titles the tab `书名 - 章节名` and switches to the target catalog title as soon as
// navigation starts, so the chapter name comes from the catalog row at currentIndex rather than the
// committed chapter. A book switch replaces book before its catalog arrives; a row is only used while it
// belongs to the current book, so the new name is never paired with the previous book's chapter.
export function useReaderDocumentTitle(options) {
  const setTitle = options.setTitle || setDocumentTitle
  const shellTitle = options.shellTitle ?? SHELL_DOCUMENT_TITLE

  watch(
    () => readerDocumentTitle(
      unref(options.book),
      unref(options.chapters),
      unref(options.currentIndex),
    ),
    (title) => {
      if (title) setTitle(title)
    },
    { immediate: true },
  )

  onScopeDispose(() => setTitle(shellTitle))
}

// The same catalog row as the running title and the error page (running title contract R1).
function readerDocumentTitle(book, chapters, currentIndex) {
  return formatReaderDocumentTitle(book?.title, readerCatalogRow(book, chapters, currentIndex)?.title || '')
}
