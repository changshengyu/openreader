import { unref } from 'vue'

export const READER_PRIMARY_PANEL_NAMES = Object.freeze([
  'shelf',
  'source',
  'toc',
  'settings',
])

export function useReaderPrimaryPanels(options) {
  function stateFor(name) {
    if (!READER_PRIMARY_PANEL_NAMES.includes(name)) return null
    return options.panels?.[name] || null
  }

  function close() {
    for (const name of READER_PRIMARY_PANEL_NAMES) {
      const state = stateFor(name)
      if (state) state.value = false
    }
  }

  function isOpen() {
    return READER_PRIMARY_PANEL_NAMES.some(name => Boolean(unref(stateFor(name))))
  }

  function toggle(name, open) {
    const state = stateFor(name)
    if (!state) return false
    const alreadyOpen = Boolean(unref(state))
    close()
    if (alreadyOpen) return false
    open?.()
    return true
  }

  return {
    close,
    isOpen,
    toggle,
  }
}
