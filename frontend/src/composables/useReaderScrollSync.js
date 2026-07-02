import { unref } from 'vue'

export function useReaderScrollSync(options) {
  function handle() {
    if (!unref(options.isVerticalRead)) return
    if (unref(options.restoringPosition) || unref(options.chapterLoading)) return
    options.syncCurrentChapter()
    options.maybeExtendChapterWindow()
    options.updateLayout()
    options.progressVersion.value += 1
    options.applyLocalProgress()
    options.scheduleProgressSave(500)
  }

  return {
    handle,
  }
}
