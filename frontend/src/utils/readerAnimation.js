function finiteNumber(value, fallback = 0) {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

function easeInOutCubic(progress) {
  return progress < 0.5
    ? 4 * progress * progress * progress
    : 1 - ((-2 * progress + 2) ** 3) / 2
}

function easeInOutFast(progress) {
  return progress < 0.5
    ? ((2 * progress) ** 1.5) / 2
    : 1 - (((2 * (1 - progress)) ** 1.5) / 2)
}

export function createReaderScrollAnimator(options = {}) {
  const requestFrame = options.requestFrame
    || globalThis.requestAnimationFrame?.bind(globalThis)
    || (callback => setTimeout(() => callback(Date.now()), 16))
  const cancelFrame = options.cancelFrame
    || globalThis.cancelAnimationFrame?.bind(globalThis)
    || clearTimeout
  const now = options.now
    || globalThis.performance?.now?.bind(globalThis.performance)
    || Date.now

  let frameId = null
  let running = false

  function cancel() {
    if (frameId !== null) cancelFrame(frameId)
    frameId = null
    running = false
  }

  function scrollTo(element, requestedTop, requestedDuration, onFinish, animationOptions = {}) {
    if (!element || running) return false
    const startTop = Math.max(0, finiteNumber(element.scrollTop))
    const bottom = Math.max(
      0,
      finiteNumber(element.scrollHeight) - finiteNumber(element.clientHeight),
    )
    const targetTop = Math.max(0, Math.min(bottom, finiteNumber(requestedTop, startTop)))
    const duration = Math.max(0, finiteNumber(requestedDuration))

    if (duration === 0 || targetTop === startTop) {
      element.scrollTop = targetTop
      onFinish?.()
      return true
    }

    running = true
    const startedAt = now()
    const distance = targetTop - startTop
    const easing = animationOptions?.easing === 'fast'
      ? easeInOutFast
      : easeInOutCubic
    const draw = (timestamp) => {
      if (!running) return
      const progress = Math.max(0, Math.min(1, (timestamp - startedAt) / duration))
      element.scrollTop = startTop + distance * easing(progress)
      if (progress < 1) {
        frameId = requestFrame(draw)
        return
      }
      element.scrollTop = targetTop
      frameId = null
      running = false
      onFinish?.()
    }
    frameId = requestFrame(draw)
    return true
  }

  return {
    cancel,
    isActive: () => running,
    scrollBy(element, delta, duration, onFinish, animationOptions) {
      return scrollTo(
        element,
        finiteNumber(element?.scrollTop) + finiteNumber(delta),
        duration,
        onFinish,
        animationOptions,
      )
    },
    scrollTo,
  }
}
