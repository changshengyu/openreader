import { unref } from 'vue'
import { normalizedReaderWheelDelta } from '../utils/readerInteraction.js'

export function useReaderWheel(options) {
  const windowTarget = options.windowTarget ?? window
  const now = options.now ?? Date.now
  let lastPageAt = 0

  function handle(event) {
    if (event._openReaderWheelHandled) return
    event._openReaderWheelHandled = true
    if (unref(options.isOverlayOpen)) return
    if (!unref(options.shellEl)?.contains(event.target)) return
    const target = event.target
    if (target?.closest?.('a, input, textarea, select, .el-drawer, .el-dialog')) return

    const viewport = unref(options.contentEl)
    const delta = normalizedReaderWheelDelta({
      deltaX: event.deltaX,
      deltaY: event.deltaY,
      deltaMode: event.deltaMode,
      fontSize: options.reader.fontSize,
      lineHeight: options.reader.lineHeight,
      pageHeight: viewport?.clientHeight || windowTarget.innerHeight || 800,
    })
    if (unref(options.isVerticalRead)) {
      if (!viewport || Math.abs(delta) < 0.01) return
      options.cancelPageAnimation?.()
      return
    }

    if (Math.abs(delta) < 4) return

    event.preventDefault()
    const timestamp = now()
    if (
      timestamp - lastPageAt
      < Math.max(140, options.reader.animateDuration + 40)
    ) return
    lastPageAt = timestamp
    if (delta > 0) options.nextPage()
    else options.previousPage()
  }

  return {
    handle,
  }
}
