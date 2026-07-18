import { computed, onBeforeUnmount, ref, unref } from 'vue'
import { useTTS } from './useTTS'
import {
  normalizeTTSSleepMinutes,
  readerTTSCurrentParagraphIndex,
  readerTTSParagraphElements,
  readerTTSParagraphText,
  readerTTSProgressLabel,
  readerTTSSleepExpired,
} from '../utils/readerTTS'

export function useReaderTTS(options) {
  const tts = useTTS()
  const sleepMinutes = ref(0)
  const sleepEndAt = ref(0)
  let continueToken = 0
  let chapterWaitController = null

  const voices = computed(() => tts.voices.value)
  const progressLabel = computed(() => readerTTSProgressLabel({
    playing: tts.state.playing,
    currentIndex: tts.currentIndex.value,
    total: tts.total.value,
  }))

  tts.setRate(options.reader.ttsRate)
  tts.setPitch(options.reader.ttsPitch)
  tts.setVoice(options.reader.ttsVoiceURI)

  function setRate(value) {
    options.reader.setTTSRate(value)
    tts.setRate(options.reader.ttsRate)
  }

  function setPitch(value) {
    options.reader.setTTSPitch(value)
    tts.setPitch(options.reader.ttsPitch)
  }

  function setVoice(value) {
    options.reader.setTTSVoice(value)
    tts.setVoice(options.reader.ttsVoiceURI)
  }

  function setSleepMinutes(value) {
    const minutes = normalizeTTSSleepMinutes(value)
    sleepMinutes.value = minutes
    sleepEndAt.value = minutes > 0 ? Date.now() + minutes * 60 * 1000 : 0
  }

  function sleepExpired() {
    return readerTTSSleepExpired(sleepEndAt.value)
  }

  function chapterRoot() {
    const root = unref(options.contentBody)
    const index = Number(unref(options.currentIndex))
    return root?.querySelector?.(`.chapter-content[data-index="${index}"]`) || root || null
  }

  function currentParagraphPayload() {
    const elements = readerTTSParagraphElements(chapterRoot())
    return {
      elements,
      texts: elements.map(readerTTSParagraphText),
    }
  }

  function clearActiveParagraph() {
    const root = unref(options.contentBody)
    const elements = readerTTSParagraphElements(root)
    elements.forEach((paragraph) => {
      paragraph.classList?.remove?.('tts-active')
      paragraph.classList?.remove?.('reading')
    })
  }

  function currentVisibleParagraphIndex() {
    const { elements } = currentParagraphPayload()
    return readerTTSCurrentParagraphIndex(elements, {
      slide: options.isSlideRead?.() === true,
      topOffset: options.topOffset?.() ?? 50,
    })
  }

  function currentParagraphElement() {
    const root = unref(options.contentBody)
    const active = root?.querySelector?.('.tts-active, .reading')
    if (active) return active
    const { elements } = currentParagraphPayload()
    return elements[currentVisibleParagraphIndex()] || null
  }

  function markActiveParagraph(index, scroll = true) {
    const { elements } = currentParagraphPayload()
    clearActiveParagraph()
    const target = elements[index]
    if (!target) return
    target.classList?.add?.('tts-active')
    target.classList?.add?.('reading')
    if (scroll) target.scrollIntoView?.({ behavior: 'smooth', block: 'center' })
  }

  function handleSpeechError(event) {
    const reason = event?.error || event?.message || event?.name || event?.type || event?.toString?.() || ''
    options.notify?.(`朗读错误: ${reason}`.trim(), 2400)
  }

  function handleParagraphStart(index) {
    markActiveParagraph(index)
    if (!sleepExpired()) return
    stop()
    options.notify?.('定时关闭朗读', 1400)
  }

  function selectedVoiceAvailable() {
    return Boolean(tts.hasSelectedVoice.value)
  }

  function canStartSpeech() {
    if (!tts.state.supported) {
      options.notify?.('当前浏览器不支持朗读')
      return false
    }
    if (!selectedVoiceAvailable()) {
      options.notify?.('请先选择语音库')
      return false
    }
    return true
  }

  function cancelChapterWait() {
    chapterWaitController?.abort()
    chapterWaitController = null
  }

  function beginCommand() {
    cancelChapterWait()
    continueToken += 1
    return continueToken
  }

  function toggle() {
    if (!tts.state.supported) {
      options.notify?.('当前浏览器不支持朗读')
      return
    }
    if (tts.state.playing) {
      stop()
      return
    }
    if (!canStartSpeech()) return

    const token = beginCommand()
    if (sleepMinutes.value > 0 && !sleepEndAt.value) setSleepMinutes(sleepMinutes.value)
    speakCurrentContent(token, currentVisibleParagraphIndex())
  }

  function speakCurrentContent(token, startIndex = 0) {
    if (token !== continueToken) return
    const { texts } = currentParagraphPayload()
    if (texts.length > 0) {
      tts.speakList(texts, Math.max(0, startIndex), () => {
        if (token !== continueToken) return
        if (sleepExpired()) {
          handleParagraphStart(tts.currentIndex.value)
          return
        }
        const index = Number(unref(options.currentIndex))
        const chapters = unref(options.chapters) || []
        if (index < chapters.length - 1) void speakChapter(index + 1, token, 'first')
      }, markActiveParagraph, handleSpeechError)
      return
    }
    tts.speak(unref(options.content), () => {
      if (token !== continueToken) return
      if (sleepExpired()) {
        handleParagraphStart()
        return
      }
      const index = Number(unref(options.currentIndex))
      const chapters = unref(options.chapters) || []
      if (index < chapters.length - 1) void speakChapter(index + 1, token, 'first')
    }, handleParagraphStart, handleSpeechError)
  }

  function stop() {
    beginCommand()
    tts.stop()
    clearActiveParagraph()
  }

  async function speakChapter(index, token, position = 'first') {
    cancelChapterWait()
    if (token !== continueToken) return
    const controller = new AbortController()
    chapterWaitController = controller
    tts.stop()
    clearActiveParagraph()
    try {
      await options.goChapter(index)
      if (token !== continueToken) return
      await options.waitForChapterReady(index, { signal: controller.signal })
      if (token !== continueToken || controller.signal.aborted) return
      const { texts } = currentParagraphPayload()
      const startIndex = position === 'last' && texts.length ? texts.length - 1 : 0
      speakCurrentContent(token, startIndex)
    } catch (error) {
      if (error?.name === 'AbortError' || token !== continueToken) return
      const reason = error?.response?.data?.error || error?.message || String(error || '章节加载失败')
      options.notify?.(`朗读章节加载失败: ${reason}`, 2400)
    } finally {
      if (chapterWaitController === controller) chapterWaitController = null
    }
  }

  function speakRelative(delta) {
    if (!canStartSpeech()) return
    const token = beginCommand()
    const { texts } = currentParagraphPayload()
    const baseIndex = tts.currentIndex.value >= 0
      ? tts.currentIndex.value
      : currentVisibleParagraphIndex()
    const targetIndex = baseIndex + delta
    if (texts.length && targetIndex >= 0 && targetIndex < texts.length) {
      speakCurrentContent(token, targetIndex)
      return
    }
    const chapterIndex = Number(unref(options.currentIndex))
    const chapters = unref(options.chapters) || []
    if (targetIndex < 0 && chapterIndex > 0) {
      void speakChapter(chapterIndex - 1, token, 'last')
      return
    }
    if (targetIndex >= texts.length && chapterIndex < chapters.length - 1) {
      void speakChapter(chapterIndex + 1, token, 'first')
    }
  }

  onBeforeUnmount(() => {
    beginCommand()
  })

  return {
    tts,
    voices,
    sleepMinutes,
    progressLabel,
    currentParagraphElement,
    setRate,
    setPitch,
    setVoice,
    setSleepMinutes,
    toggle,
    previous: () => speakRelative(-1),
    next: () => speakRelative(1),
    stop,
  }
}
