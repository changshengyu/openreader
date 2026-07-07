import { onBeforeUnmount, reactive, ref, watch } from 'vue'
import { normalizeTTSPitch, normalizeTTSRate, sortTTSVoices } from '../utils/readerTTS'

export function useTTS() {
  const synth = typeof window !== 'undefined' ? window.speechSynthesis : null
  const state = reactive({
    supported: !!synth,
    playing: false,
    paused: false,
    rate: 1,
    pitch: 1,
    voiceIndex: 0,
    voiceURI: '',
  })
  const currentIndex = ref(-1)
  const total = ref(0)
  let paragraphs = []
  let pending = false

  const voices = ref([])
  let activeOnEnd = null
  let activeOnStart = null
  let activeOnError = null

  function loadVoices() {
    if (!synth) return
    const availableVoices = synth.getVoices()
    voices.value = sortTTSVoices(availableVoices)
    if (availableVoices.length > 0 && state.voiceURI && !voices.value.some(v => v.voiceURI === state.voiceURI)) {
      state.voiceURI = ''
    }
  }
  loadVoices()
  synth?.addEventListener('voiceschanged', loadVoices)

  function stop() {
    if (!synth) return
    synth.cancel()
    state.playing = false
    state.paused = false
    currentIndex.value = -1
    total.value = 0
    pending = false
    activeOnEnd = null
    activeOnStart = null
    activeOnError = null
  }

  function pause() {
    if (!synth) return
    if (state.playing && !state.paused) {
      synth.pause()
      state.paused = true
    }
  }

  function resume() {
    if (!synth) return
    if (state.paused) {
      synth.resume()
      state.paused = false
    }
  }

  function speak(text, onEnd, onStart, onError) {
    speakList(String(text || '').split('\n'), 0, onEnd, onStart, onError)
  }

  function speakList(list, startIndex = 0, onEnd, onStart, onError) {
    if (!synth) return
    stop()
    paragraphs = (Array.isArray(list) ? list : [])
      .map(l => String(l || '').trim())
      .filter(Boolean)
    if (paragraphs.length === 0) return

    state.playing = true
    state.paused = false
    currentIndex.value = Math.max(0, Math.min(paragraphs.length - 1, Number(startIndex) || 0))
    total.value = paragraphs.length
    pending = false
    activeOnEnd = onEnd
    activeOnStart = onStart
    activeOnError = onError
    speakCurrent(onEnd, onStart, onError)
  }

  function speakCurrent(onEnd, onStart, onError) {
    if (currentIndex.value >= paragraphs.length) {
      stop()
      onEnd?.()
      return
    }

    const utterance = new SpeechSynthesisUtterance(paragraphs[currentIndex.value])
    utterance.rate = state.rate
    utterance.pitch = state.pitch
    if (voices.value.length > 0) {
      utterance.voice = state.voiceURI
        ? voices.value.find(voice => voice.voiceURI === state.voiceURI) || voices.value[0]
        : voices.value[Math.min(state.voiceIndex, voices.value.length - 1)]
    }

    utterance.addEventListener('start', () => {
      onStart?.(currentIndex.value)
    })

    utterance.addEventListener('end', () => {
      if (pending) return
      currentIndex.value++
      if (currentIndex.value < paragraphs.length) {
        speakCurrent(onEnd, onStart, onError)
      } else {
        stop()
        onEnd?.()
      }
    })

    utterance.addEventListener('error', (event) => {
      pending = false
      state.playing = synth.speaking || false
      onError?.(event)
    })

    synth.speak(utterance)
  }

  function skipForward() {
    if (!synth) return
    if (currentIndex.value < paragraphs.length - 1) {
      synth.cancel()
      pending = true
      currentIndex.value++
      setTimeout(() => {
        pending = false
        speakCurrent(activeOnEnd, activeOnStart, activeOnError)
      }, 50)
    }
  }

  function skipBackward() {
    if (!synth) return
    if (currentIndex.value > 0) {
      synth.cancel()
      pending = true
      currentIndex.value = Math.max(0, currentIndex.value - 1)
      setTimeout(() => {
        pending = false
        speakCurrent(activeOnEnd, activeOnStart, activeOnError)
      }, 50)
    }
  }

  function restartCurrent() {
    if (!synth || !state.playing || currentIndex.value < 0 || currentIndex.value >= paragraphs.length) return
    const wasPaused = state.paused
    synth.cancel()
    pending = true
    setTimeout(() => {
      pending = false
      speakCurrent(activeOnEnd, activeOnStart, activeOnError)
      if (wasPaused) setTimeout(() => pause(), 0)
    }, 50)
  }

  function setRate(rate) {
    state.rate = normalizeTTSRate(rate)
    restartCurrent()
  }

  function setPitch(pitch) {
    state.pitch = normalizeTTSPitch(pitch)
    restartCurrent()
  }

  function setVoice(uri) {
    state.voiceURI = uri || ''
    const index = voices.value.findIndex(voice => voice.voiceURI === state.voiceURI)
    state.voiceIndex = index >= 0 ? index : 0
    restartCurrent()
  }

  onBeforeUnmount(() => {
    stop()
    synth?.removeEventListener('voiceschanged', loadVoices)
  })

  return {
    state,
    voices,
    currentIndex,
    total,
    speak,
    speakList,
    stop,
    pause,
    resume,
    skipForward,
    skipBackward,
    setRate,
    setPitch,
    setVoice,
  }
}
