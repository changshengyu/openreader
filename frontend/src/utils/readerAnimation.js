function finiteNumber(value, fallback = 0) {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

function easeInOutCubic(progress) {
  return progress < 0.5
    ? 4 * progress * progress * progress
    : 1 - ((-2 * progress + 2) ** 3) / 2
}

function clampProgress(value) {
  return Math.max(0, Math.min(1, finiteNumber(value)))
}

function compositeKeyframes(distance, duration) {
  const sampleCount = Math.max(2, Math.min(60, Math.ceil(duration / 16)))
  return Array.from({ length: sampleCount + 1 }, (_, index) => {
    const offset = index / sampleCount
    const translated = -distance * easeInOutCubic(offset)
    return {
      offset,
      transform: `translate3d(0, ${translated}px, 0)`,
    }
  })
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
  let compositeRun = null

  function restoreCompositeStyle(run) {
    if (!run?.visualElement?.style) return
    run.visualElement.style.willChange = run.previousWillChange
  }

  function stopComposite(run, targetTop, completed) {
    if (!run || compositeRun !== run) return
    run.animation.onfinish = null
    run.element.scrollTop = targetTop
    run.animation.cancel()
    restoreCompositeStyle(run)
    compositeRun = null
    running = false
    if (completed) run.onFinish?.()
  }

  function cancelComposite(run) {
    const currentTime = run.animation.currentTime == null
      ? Math.max(0, now() - run.startedAt)
      : finiteNumber(run.animation.currentTime)
    const progress = easeInOutCubic(clampProgress(currentTime / run.duration))
    stopComposite(
      run,
      Math.max(0, Math.min(run.bottom, run.startTop + run.distance * progress)),
      false,
    )
  }

  function cancel() {
    if (compositeRun) {
      cancelComposite(compositeRun)
      return
    }
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

    const visualElement = animationOptions?.visualElement
    if (typeof visualElement?.animate === 'function' && visualElement.style) {
      const previousWillChange = visualElement.style.willChange
      visualElement.style.willChange = 'transform'
      const distance = targetTop - startTop
      const animation = visualElement.animate(
        compositeKeyframes(distance, duration),
        {
          duration,
          easing: 'linear',
          fill: 'both',
        },
      )
      const run = {
        animation,
        bottom,
        distance,
        duration,
        element,
        onFinish,
        previousWillChange,
        startedAt: now(),
        startTop,
        targetTop,
        visualElement,
      }
      compositeRun = run
      running = true
      animation.onfinish = () => stopComposite(run, targetTop, true)
      return true
    }

    running = true
    const startedAt = now()
    const distance = targetTop - startTop
    const draw = (timestamp) => {
      if (!running) return
      const progress = Math.max(0, Math.min(1, (timestamp - startedAt) / duration))
      element.scrollTop = startTop + distance * easeInOutCubic(progress)
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
