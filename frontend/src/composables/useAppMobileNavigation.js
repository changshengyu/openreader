import { computed, ref } from 'vue'

export function useAppMobileNavigation(options) {
  const windowWidth = ref(options.currentViewportWidth())
  const visible = ref(false)
  const touchStart = ref(null)
  const touchMoveX = ref(0)
  const touchAxis = ref('')
  const navigationWidth = computed(() => 260)
  let ignoreWorkspaceClickUntil = 0

  const isMobile = computed(() => (
    options.shouldUseMiniInterface(
      options.getPageMode(),
      windowWidth.value,
    )
  ))
  const navigationStyle = computed(() => {
    const width = navigationWidth.value
    const base = { '--mobile-nav-width': `${width}px` }
    if (!isMobile.value || !touchMoveX.value) return base
    if (
      !visible.value &&
      touchMoveX.value > 0 &&
      touchMoveX.value <= width
    ) {
      const offset = touchMoveX.value - width
      return {
        ...base,
        '--mobile-nav-drag-offset': `${offset}px`,
        marginLeft: `${offset}px`,
        transition: 'none',
      }
    }
    if (
      visible.value &&
      touchMoveX.value < 0 &&
      touchMoveX.value >= -width
    ) {
      const offset = touchMoveX.value
      return {
        ...base,
        '--mobile-nav-drag-offset': `${offset}px`,
        marginLeft: `${offset}px`,
        transition: 'none',
      }
    }
    return base
  })

  function updateViewport() {
    windowWidth.value = options.currentViewportWidth()
  }

  function handleTouchStart(event) {
    if (!isMobile.value || event.touches?.length !== 1) return
    const touch = event.touches[0]
    const viewportHeight = options.getViewportHeight()
    const viewportWidth = options.getViewportWidth()
    if (
      touch.clientY <= 20 ||
      touch.clientY >= viewportHeight - 20 ||
      touch.clientX <= 20 ||
      touch.clientX >= viewportWidth - 20
    ) {
      touchStart.value = null
      return
    }
    touchStart.value = { x: touch.clientX, y: touch.clientY }
    touchMoveX.value = 0
    touchAxis.value = ''
  }

  function handleTouchMove(event) {
    if (
      !isMobile.value ||
      !touchStart.value ||
      event.touches?.length !== 1
    ) {
      return
    }
    const touch = event.touches[0]
    const moveX = touch.clientX - touchStart.value.x
    const moveY = touch.clientY - touchStart.value.y
    if (
      !touchAxis.value &&
      Math.max(Math.abs(moveX), Math.abs(moveY)) >= 8
    ) {
      touchAxis.value = Math.abs(moveX) > Math.abs(moveY) ? 'x' : 'y'
    }
    if (touchAxis.value === 'y') {
      touchMoveX.value = 0
      return
    }
    if (touchAxis.value !== 'x') return
    const width = navigationWidth.value
    if (
      (!visible.value && moveX > 0 && moveX <= width) ||
      (visible.value && moveX < 0 && moveX >= -width)
    ) {
      event.preventDefault()
      event.stopPropagation()
      touchMoveX.value = moveX
    }
  }

  function handleTouchEnd() {
    if (!isMobile.value) return
    if (touchAxis.value === 'x' && touchMoveX.value > 0) {
      visible.value = true
    }
    if (touchAxis.value === 'x' && touchMoveX.value < 0) {
      visible.value = false
    }
    if (touchAxis.value === 'x' && touchMoveX.value !== 0) {
      ignoreWorkspaceClickUntil = options.now() + 350
    }
    resetTouch()
  }

  function handleTouchCancel() {
    resetTouch()
  }

  function resetTouch() {
    touchStart.value = null
    touchMoveX.value = 0
    touchAxis.value = ''
  }

  function close() {
    if (options.now() < ignoreWorkspaceClickUntil) return
    if (isMobile.value && visible.value) visible.value = false
  }

  function toggle() {
    if (isMobile.value) visible.value = !visible.value
  }

  return {
    windowWidth,
    visible,
    touchStart,
    touchMoveX,
    touchAxis,
    navigationWidth,
    isMobile,
    navigationStyle,
    updateViewport,
    handleTouchStart,
    handleTouchMove,
    handleTouchEnd,
    handleTouchCancel,
    close,
    toggle,
  }
}
