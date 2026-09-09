import { nextTick, unref } from 'vue'

export function useReaderChapterLoader(options) {
  let loadingTimer = null
  let loadGeneration = 0
  let activeController = null

  function clearLoadingTimer() {
    clearTimeout(loadingTimer)
    loadingTimer = null
  }

  async function load(index, offset = 0, loadOptions = {}) {
    const generation = ++loadGeneration
    activeController?.abort()
    const controller = new AbortController()
    activeController = controller
    options.invalidateChapterWindow?.()
    const anchor = options.captureScrollAnchor?.() || null
    const targetIndex = Math.max(
      0,
      Math.min(index, Math.max(options.chapters.value.length - 1, 0)),
    )
    options.currentIndex.value = targetIndex
    const scopeKey = options.getScopeKey?.()
    const transaction = { generation, targetIndex, offset, anchor, scopeKey }
    options.onStart?.(transaction)
    const isCurrent = () => generation === loadGeneration
      && options.currentIndex.value === targetIndex
      && (options.getScopeKey?.() ?? scopeKey) === scopeKey
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
        if (isCurrent()) options.chapterLoading.value = true
      }, 120)
    }

    try {
      const data = await options.loadContent(
        targetIndex,
        {
          refresh: Boolean(loadOptions.refresh),
          signal: controller.signal,
        },
      )
      if (!isCurrent()) return false
      options.chapter.value = data.chapter
      options.content.value = data.content || ''
      const format = data.format === 'epub' && data.resourceUrl
        ? 'epub'
        : data.format === 'audio' && data.resourceUrl
          ? 'audio'
          : 'text'
      options.chapterFormat.value = format
      const cachedImages = format === 'text' && data.cachedImages && typeof data.cachedImages === 'object'
        ? { ...data.cachedImages }
        : {}
      if (options.cachedImages) options.cachedImages.value = cachedImages
      options.epubResource.value = format === 'epub'
        ? {
            url: data.resourceUrl,
            expiresAt: data.resourceExpiresAt || '',
          }
        : null
      if (options.audioResource) {
        options.audioResource.value = format === 'audio'
          ? {
              url: data.resourceUrl,
              expiresAt: data.resourceExpiresAt || '',
              title: data.chapter?.title || '',
            }
          : null
      }
      options.page.value = 0
      options.chapterBlocks.value = format === 'epub' || format === 'audio'
        ? []
        : [
            options.makeChapterBlock(
              targetIndex,
              options.chapter.value,
              options.content.value,
              cachedImages,
            ),
          ]
      if (format === 'epub') {
        options.onEpubPrepared?.({
          chapterIndex: targetIndex,
          offset,
          restoreOptions: loadOptions,
        })
      }
      if (format === 'audio') {
        options.onAudioPrepared?.({
          chapterIndex: targetIndex,
          offset,
          restoreOptions: loadOptions,
        })
      }
      options.chapterLoading.value = false
      await nextTick()
      if (!isCurrent()) return false
      options.updateLayout()
      await options.restorePosition(offset, loadOptions)
      if (!isCurrent()) return false
      options.progressVersion.value += 1
      options.preloadNearby(targetIndex)
      if (loadOptions.saveAfterLoad) {
        await options.saveProgress({ force: true })
      } else {
        options.markProgressSaved(options.getCurrentProgress())
      }
      options.chapterLoaded.value = true
      if (unref(options.isContinuousScrollRead)) {
        options.computeChapterWindow({ anchorIndex: targetIndex }).catch(() => {})
      }
      return true
    } catch (error) {
      if (!isCurrent() || error?.name === 'AbortError') return false
      options.epubResource.value = null
      if (options.audioResource) options.audioResource.value = null
      if (options.cachedImages) options.cachedImages.value = {}
      options.chapterLoadError.value = options.formatError(error)
      return false
    } finally {
      if (isCurrent()) {
        clearLoadingTimer()
        await options.nextFrame()
        if (isCurrent()) {
          options.restoringPosition.value = false
          options.chapterLoading.value = false
          if (activeController === controller) activeController = null
        }
      }
      options.onSettled?.(transaction)
    }
  }

  function invalidate() {
    loadGeneration += 1
    activeController?.abort()
    activeController = null
    clearLoadingTimer()
  }

  return {
    clearLoadingTimer: invalidate,
    invalidate,
    load,
  }
}
