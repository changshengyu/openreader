function finite(value, fallback = 0) {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

export function shouldUseDocumentReaderScroll({
  mobile = false,
  mode = 'page',
  format = 'text',
  comic = false,
} = {}) {
  return Boolean(
    mobile
    && !comic
    && format === 'text'
    && ['page', 'scroll', 'scroll2'].includes(mode),
  )
}

export function createDocumentReaderScrollViewport({
  documentTarget = document,
  windowTarget = window,
} = {}) {
  function root() {
    return documentTarget.scrollingElement
      || documentTarget.documentElement
      || documentTarget.body
  }

  function viewportWidth() {
    return Math.max(
      1,
      finite(windowTarget.visualViewport?.width)
        || finite(windowTarget.innerWidth)
        || finite(root()?.clientWidth, 1),
    )
  }

  function viewportHeight() {
    return Math.max(
      1,
      finite(windowTarget.visualViewport?.height)
        || finite(windowTarget.innerHeight)
        || finite(root()?.clientHeight, 1),
    )
  }

  const viewport = {
    get scrollTop() {
      return Math.max(0, finite(root()?.scrollTop))
    },
    set scrollTop(value) {
      const top = Math.max(0, finite(value))
      const element = root()
      if (element) element.scrollTop = top
      if (documentTarget.documentElement && documentTarget.documentElement !== element) {
        documentTarget.documentElement.scrollTop = top
      }
      if (documentTarget.body && documentTarget.body !== element) {
        documentTarget.body.scrollTop = top
      }
    },
    get scrollHeight() {
      return Math.max(
        finite(root()?.scrollHeight),
        finite(documentTarget.documentElement?.scrollHeight),
        finite(documentTarget.body?.scrollHeight),
      )
    },
    get scrollWidth() {
      return Math.max(
        finite(root()?.scrollWidth),
        finite(documentTarget.documentElement?.scrollWidth),
        finite(documentTarget.body?.scrollWidth),
      )
    },
    get clientHeight() {
      return viewportHeight()
    },
    get clientWidth() {
      return viewportWidth()
    },
    getBoundingClientRect() {
      const width = viewportWidth()
      const height = viewportHeight()
      return {
        top: 0,
        right: width,
        bottom: height,
        left: 0,
        width,
        height,
        x: 0,
        y: 0,
      }
    },
    scrollTo(first, second) {
      if (typeof windowTarget.scrollTo === 'function') {
        if (second === undefined) windowTarget.scrollTo(first)
        else windowTarget.scrollTo(first, second)
        return
      }
      viewport.scrollTop = typeof first === 'object' ? first?.top : second
    },
  }

  return viewport
}

export function readerElementScrollTop(viewport, element) {
  if (!viewport || !element) return 0
  const viewportRect = viewport.getBoundingClientRect?.()
  const elementRect = element.getBoundingClientRect?.()
  if (Number.isFinite(elementRect?.top) && Number.isFinite(viewportRect?.top)) {
    return Math.max(
      0,
      finite(viewport.scrollTop) + elementRect.top - viewportRect.top,
    )
  }
  return Math.max(0, finite(element.offsetTop))
}
