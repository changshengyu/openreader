import { unref } from 'vue'

export function useReaderChrome(options) {
  function toggle() {
    if (unref(options.isMobileReader)) {
      options.mobileChromeVisible.value = !options.mobileChromeVisible.value
      return
    }
    if (options.tocVisible.value) {
      options.tocVisible.value = false
    } else {
      options.openToc()
    }
    options.settingsVisible.value = false
  }

  return { toggle }
}
