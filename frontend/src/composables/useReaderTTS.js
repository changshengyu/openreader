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

  function currentParagraphPayload() {
    const elements = readerTTSParagraphElements(unref(options.contentBody))
    return {
      elements,
      texts: elements.map(readerTTSParagraphText),
    }
  }

  function clearActiveParagraph(elements = currentParagraphPayload().elements) {
    elements.forEach((paragraph) => {
      paragraph.classList?.remove?.('tts-active')
      paragraph.classList?.remove?.('reading')
    })
  }

  function markActiveParagraph(index, scroll = true) {
    const { elements } = currentParagraphPayload()
    clearActiveParagraph(elements)
    const target = elements[index]
    if (!target) return
    target.classList?.add?.('tts-active')
    target.classList?.add?.('reading')
    if (scroll) target.scrollIntoView?.({ behavior: 'smooth', block: 'center' })
  }

  function handleSpeechError(event) {
    const reason = event?.error || event?.name || event?.type || event?.toString?.() || ''
    options.notify?.(`朗读错误: ${reason}`.trim(), 2400)
  }

  function handleParagraphStart(index) {
    markActiveParagraph(index)
    if (!sleepExpired()) return
    continueToken += 1
    tts.stop()
    options.notify?.('定时关闭朗读', 1400)
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

    const token = ++continueToken
    if (sleepMinutes.value > 0 && !sleepEndAt.value) setSleepMinutes(sleepMinutes.value)
    speakCurrentContent(token, currentVisibleParagraphIndex())
  }

  function currentVisibleParagraphIndex() {
    const { elements } = currentParagraphPayload()
    return readerTTSCurrentParagraphIndex(elements, {
      slide: options.isSlideRead?.() === true,
      topOffset: options.topOffset?.() ?? 50,
    })
  }

  function speakCurrentContent(token, startIndex = 0) {
    const { texts } = currentParagraphPayload()
    if (texts.length > 0) {
      tts.speakList(texts, Math.max(0, startIndex), () => {
        if (sleepExpired()) {
          handleParagraphStart(tts.currentIndex.value)
          return
        }
        const index = Number(unref(options.currentIndex))
        const chapters = unref(options.chapters) || []
        if (index < chapters.length - 1) speakChapter(index + 1, token, 'first')
      }, markActiveParagraph, handleSpeechError)
      return
    }
    tts.speak(unref(options.content), () => {
      if (sleepExpired()) {
        handleParagraphStart()
        return
      }
      const index = Number(unref(options.currentIndex))
      const chapters = unref(options.chapters) || []
      if (index < chapters.length - 1) speakChapter(index + 1, token, 'first')
    }, handleParagraphStart, handleSpeechError)
  }

  function stop() {
    continueToken += 1
    tts.stop()
    clearActiveParagraph()
  }

  async function speakChapter(index, token, position = 'first') {
    await options.goChapter(index)
    for (let attempt = 0; attempt < 30; attempt += 1) {
      if (token !== continueToken) return
      await new Promise(resolve => setTimeout(resolve, 120))
      const { texts } = currentParagraphPayload()
      const content = String(unref(options.content) || '')
      if (Number(unref(options.currentIndex)) === index && (texts.length || content.trim())) {
        const startIndex = position === 'last' && texts.length ? texts.length - 1 : 0
        speakCurrentContent(token, startIndex)
        return
      }
    }
  }

  function speakRelative(delta) {
    if (!tts.state.supported) {
      options.notify?.('当前浏览器不支持朗读')
      return
    }
    const token = ++continueToken
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
      speakChapter(chapterIndex - 1, token, 'last')
      return
    }
    if (targetIndex >= texts.length && chapterIndex < chapters.length - 1) {
      speakChapter(chapterIndex + 1, token, 'first')
    }
  }

  onBeforeUnmount(() => {
    continueToken += 1
  })

  return {
    tts,
    voices,
    sleepMinutes,
    progressLabel,
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
