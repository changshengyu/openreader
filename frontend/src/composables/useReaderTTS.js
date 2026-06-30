import { computed, onBeforeUnmount, ref, unref, watch } from 'vue'
import { useTTS } from './useTTS'
import {
  normalizeTTSSleepMinutes,
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

  function handleParagraphStart() {
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
    speakCurrentContent(token)
  }

  function speakCurrentContent(token) {
    tts.speak(unref(options.content), () => {
      if (sleepExpired()) {
        handleParagraphStart()
        return
      }
      const index = Number(unref(options.currentIndex))
      const chapters = unref(options.chapters) || []
      if (index < chapters.length - 1) speakNextChapter(index + 1, token)
    }, handleParagraphStart)
  }

  function stop() {
    continueToken += 1
    tts.stop()
  }

  async function speakNextChapter(index, token) {
    await options.goChapter(index)
    for (let attempt = 0; attempt < 30; attempt += 1) {
      if (token !== continueToken) return
      await new Promise(resolve => setTimeout(resolve, 120))
      const content = String(unref(options.content) || '')
      if (Number(unref(options.currentIndex)) === index && content.trim()) {
        speakCurrentContent(token)
        return
      }
    }
  }

  watch(() => tts.currentIndex.value, (index) => {
    const contentBody = unref(options.contentBody)
    if (index < 0 || !contentBody) return
    const paragraphs = contentBody.querySelectorAll('p')
    paragraphs.forEach(paragraph => paragraph.classList.remove('tts-active'))
    const target = paragraphs[index]
    if (!target) return
    target.classList.add('tts-active')
    target.scrollIntoView({ behavior: 'smooth', block: 'center' })
  })

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
    stop,
  }
}
