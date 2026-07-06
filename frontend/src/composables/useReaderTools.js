export function useReaderTools(options) {
  function resolve(action) {
    return options.actions[action]
  }

  function openMobileTool(action) {
    action?.()
  }

  function handleMobileChromeAction(action) {
    if (action === 'previous') {
      options.goChapter(options.currentIndex.value - 1)
      return
    }
    if (action === 'next') {
      options.goChapter(options.currentIndex.value + 1)
      return
    }
    if (action === 'toggle') {
      options.toggleChrome()
      return
    }
    openMobileTool(resolve(action))
  }

  function handleDesktopToolAction(action) {
    resolve(action)?.()
  }

  return {
    handleDesktopToolAction,
    handleMobileChromeAction,
    openMobileTool,
    resolve,
  }
}
