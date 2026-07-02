export function useReaderTools(options) {
  function resolve(action) {
    return options.actions[action]
  }

  function openMobileTool(action) {
    options.mobileChromeVisible.value = false
    action?.()
  }

  function runMobileAction(action) {
    options.mobileMoreVisible.value = false
    options.mobileChromeVisible.value = false
    action?.()
  }

  function handleMobileToolAction(action) {
    runMobileAction(resolve(action))
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
    if (action === 'more') {
      openMobileTool(() => {
        options.mobileMoreVisible.value = true
      })
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
    handleMobileToolAction,
    openMobileTool,
    resolve,
    runMobileAction,
  }
}
