import { nextTick, unref } from 'vue'

export function useReaderChapterLoader(options) {
  let loadingTimer = null

  function clearLoadingTimer() {
    clearTimeout(loadingTimer)
    loadingTimer = null
  }

  async function load(index, offset = 0, loadOptions = {}) {
    options.currentIndex.value = Math.max(
      0,
      Math.min(index, Math.max(options.chapters.value.length - 1, 0)),
    )
    if (loadOptions.hideChrome) {
      options.mobileChromeVisible.value = false
    }
    options.restoringPosition.value = true
    options.chapterLoaded.value = false
    options.chapterLoadError.value = ''
    options.cancelProgressSave()
    clearLoadingTimer()

    const cachedBeforeLoad = !loadOptions.refresh
      && options.getMemoryContent(options.currentIndex.value)
    options.chapterLoading.value = !cachedBeforeLoad
    if (!cachedBeforeLoad) {
      loadingTimer = setTimeout(() => {
        options.chapterLoading.value = true
      }, 120)
    }

    try {
      const data = await options.loadContent(
        options.currentIndex.value,
        { refresh: Boolean(loadOptions.refresh) },
      )
      options.chapter.value = data.chapter
      options.content.value = data.content || ''
      const format = data.format === 'epub' && data.resourceUrl ? 'epub' : 'text'
      options.chapterFormat.value = format
      options.epubResource.value = format === 'epub'
        ? {
            url: data.resourceUrl,
            expiresAt: data.resourceExpiresAt || '',
          }
        : null
      options.page.value = 0
      options.chapterBlocks.value = format === 'epub'
        ? []
        : [
            options.makeChapterBlock(
              options.currentIndex.value,
              options.chapter.value,
              options.content.value,
            ),
          ]
      if (format === 'epub') {
        options.onEpubPrepared?.({
          chapterIndex: options.currentIndex.value,
          offset,
          restoreOptions: loadOptions,
        })
      }
      options.chapterLoading.value = false
      await nextTick()
      options.updateLayout()
      await options.restorePosition(offset, loadOptions)
      options.progressVersion.value += 1
      options.preloadNearby(options.currentIndex.value)
      if (loadOptions.saveAfterLoad) {
        await options.saveProgress({ force: true })
      } else {
        options.markProgressSaved(options.getCurrentProgress())
      }
      options.chapterLoaded.value = true
      if (unref(options.isContinuousScrollRead)) {
        options.computeChapterWindow({ anchorIndex: options.currentIndex.value }).catch(() => {})
      }
    } catch (error) {
      options.epubResource.value = null
      options.chapterLoadError.value = options.formatError(error)
    } finally {
      clearLoadingTimer()
      await options.nextFrame()
      options.restoringPosition.value = false
      options.chapterLoading.value = false
    }
  }

  return {
    clearLoadingTimer,
    load,
  }
}
