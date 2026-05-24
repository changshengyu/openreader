import { onBeforeUnmount } from 'vue'

export function useKeyboard(handlers) {
  const {
    onPageUp,
    onPageDown,
    onHome,
    onEnd,
    onEscape,
    onSpace,
    onArrowLeft,
    onArrowRight,
    onArrowUp,
    onArrowDown,
  } = handlers

  function run(handler) {
    handler?.()
  }

  function handleKey(event) {
    const tag = document.activeElement?.tagName?.toLowerCase()
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return

    switch (event.key) {
      case 'ArrowLeft':
        event.preventDefault()
        run(onArrowLeft || onPageUp)
        break
      case 'ArrowRight':
        event.preventDefault()
        run(onArrowRight || onPageDown)
        break
      case 'ArrowUp':
        event.preventDefault()
        run(onArrowUp || onPageUp)
        break
      case 'ArrowDown':
        event.preventDefault()
        run(onArrowDown || onPageDown)
        break
      case 'PageUp':
        event.preventDefault()
        onPageUp?.()
        break
      case 'PageDown':
        event.preventDefault()
        onPageDown?.()
        break
      case 'Home':
        event.preventDefault()
        onHome?.()
        break
      case 'End':
        event.preventDefault()
        onEnd?.()
        break
      case ' ':
        event.preventDefault()
        onSpace?.()
        break
      case 'Escape':
        event.preventDefault()
        onEscape?.()
        break
    }
  }

  window.addEventListener('keydown', handleKey)
  onBeforeUnmount(() => window.removeEventListener('keydown', handleKey))
}
