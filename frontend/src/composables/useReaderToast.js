import { onScopeDispose, ref } from 'vue'

export function useReaderToast(options = {}) {
  const message = ref('')
  const setTimer = options.setTimeout || setTimeout
  const clearTimer = options.clearTimeout || clearTimeout
  let timer = null

  function clear() {
    if (timer !== null) {
      clearTimer(timer)
      timer = null
    }
    message.value = ''
  }

  function show(nextMessage, duration = 1600) {
    if (timer !== null) {
      clearTimer(timer)
      timer = null
    }
    message.value = String(nextMessage || '')
    const delay = Number(duration) || 0
    if (!message.value || delay <= 0) return
    timer = setTimer(() => {
      timer = null
      message.value = ''
    }, delay)
  }

  onScopeDispose(clear)

  return {
    message,
    clear,
    show,
  }
}
