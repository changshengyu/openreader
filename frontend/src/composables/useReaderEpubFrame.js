import { unref } from 'vue'

function bridgeMessage(data) {
  if (typeof data === 'string') {
    try {
      return JSON.parse(data)
    } catch {
      return null
    }
  }
  return data && typeof data === 'object' ? data : null
}

function expectedValue(value, fallback = '') {
  return typeof value === 'function' ? value() : unref(value) ?? fallback
}

export function useReaderEpubFrame(options) {
  function send(event, payload = {}) {
    const target = unref(options.frame)?.contentWindow
    const origin = expectedValue(options.expectedOrigin)
    if (!target || !origin) return false
    target.postMessage(JSON.stringify({ event, ...payload }), origin)
    return true
  }

  function syncStyle() {
    return send('setStyle', { style: String(expectedValue(options.styleText) || '') })
  }

  function requestHeight() {
    return send('requestHeight')
  }

  function handleMessage(event) {
    const frameWindow = unref(options.frame)?.contentWindow
    const origin = expectedValue(options.expectedOrigin)
    if (!frameWindow || event?.source !== frameWindow || event?.origin !== origin) return false
    const message = bridgeMessage(event.data)
    if (!message || typeof message.event !== 'string') return false

    if (message.event === 'inited') {
      syncStyle()
      requestHeight()
      options.onReady?.()
      return true
    }
    if (message.event === 'load') {
      options.onLoad?.(message.data)
      return true
    }
    if (message.event === 'setHeight') {
      const reported = Math.max(0, Number(message.data) || 0)
      const minimum = Math.max(0, Number(expectedValue(options.viewportHeight)) || 0) * 0.8
      options.onHeight?.(Math.max(reported, minimum))
      return true
    }
    if (message.event === 'click') {
      options.onClick?.(message.data)
      return true
    }
    if (message.event === 'clickHash') {
      options.onHash?.(message.data)
      return true
    }
    if (message.event === 'navigate') {
      options.onNavigate?.(message.data)
      return true
    }
    if (message.event === 'keydown') {
      options.onKeydown?.(message.data)
      return true
    }
    if (message.event === 'previewImageList') {
      options.onPreview?.(message.data)
      return true
    }
    return false
  }

  return {
    handleMessage,
    requestHeight,
    send,
    syncStyle,
  }
}

export function epubResourceLocationFromURL(resourceURL) {
  const raw = String(resourceURL || '')
  if (!raw) return { resourcePath: '', fragment: '' }
  let pathname = raw
  let hash = ''
  try {
    const parsed = new URL(raw, 'https://openreader.local')
    pathname = parsed.pathname
    hash = parsed.hash.slice(1)
  } catch {
    pathname = raw.split(/[?#]/, 1)[0]
    hash = raw.includes('#') ? raw.slice(raw.indexOf('#') + 1) : ''
  }
  const match = pathname.match(/^\/api\/epub-resource\/[^/]+\/(.*)$/)
  if (!match) return { resourcePath: '', fragment: '' }
  try {
    return {
      resourcePath: decodeURIComponent(match[1]),
      fragment: hash ? decodeURIComponent(hash) : '',
    }
  } catch {
    return { resourcePath: match[1], fragment: hash }
  }
}

export function epubResourcePathFromURL(resourceURL) {
  return epubResourceLocationFromURL(resourceURL).resourcePath
}

export function epubChapterIndexForResourceURL(resourceURL, chapters = []) {
  const { resourcePath, fragment } = epubResourceLocationFromURL(resourceURL)
  if (!resourcePath) return -1
  if (fragment) {
    const exactIndex = chapters.findIndex(chapter => (
      String(chapter?.resourcePath || '') === resourcePath &&
      String(chapter?.resourceFragment || '') === fragment
    ))
    if (exactIndex >= 0) return exactIndex
  }
  return chapters.findIndex(chapter => String(chapter?.resourcePath || '') === resourcePath)
}
