const PRIMARY_TOOLS = new Set(['shelf', 'source', 'toc', 'settings'])

export function useReaderTools(options) {
  function resolve(action) {
    return options.actions[action]
  }

  function openMobileTool(action) {
    action?.()
  }

  // reader-dev Reader.vue#toLastChapter/#toNextChapter: the chapter buttons stay enabled
  // and report the book boundary instead.
  function stepChapter(delta) {
    const target = options.currentIndex.value + delta
    const count = Number(options.chapterCount?.()) || 0
    if (target < 0) {
      options.notify?.('本章是第一章')
      return
    }
    if (count && target >= count) {
      options.notify?.('本章是最后一章')
      return
    }
    options.goChapter(target)
  }

  function run(action) {
    if (action === 'toggle') {
      options.toggleChrome()
      return
    }
    // reader-dev's primary panels are click popovers: a press on any other tool also closes them.
    if (!PRIMARY_TOOLS.has(action)) options.closePrimaryPanels?.()
    if (action === 'previous') {
      stepChapter(-1)
      return
    }
    if (action === 'next') {
      stepChapter(1)
      return
    }
    openMobileTool(resolve(action))
  }

  function handleMobileChromeAction(action) {
    run(action)
  }

  function handleDesktopToolAction(action) {
    run(action)
  }

  return {
    handleDesktopToolAction,
    handleMobileChromeAction,
    openMobileTool,
    resolve,
  }
}
