import { computed, getCurrentInstance, onBeforeUnmount, reactive, ref } from 'vue'
import {
  normalizeTTSPitch,
  normalizeTTSRate,
  readerSpeechSynthesisSupported,
  sortTTSVoices,
} from '../utils/readerTTS'

export function useTTS() {
  const synth = typeof window !== 'undefined' ? window.speechSynthesis : null
  const state = reactive({
    supported: readerSpeechSynthesisSupported(synth),
    playing: false,
    paused: false,
    rate: 1,
    pitch: 1,
    voiceIndex: -1,
    voiceURI: '',
  })
  const currentIndex = ref(-1)
  const total = ref(0)
  const voices = ref([])
  const hasSelectedVoice = computed(() => Boolean(selectedVoice()))
  let paragraphs = []
  let pending = false
  let activeOnEnd = null
  let activeOnStart = null
  let activeOnError = null
  let previousVoicesChanged = null
  let usesVoicesChangedProperty = false

  function selectedVoice() {
    if (!state.voiceURI) return null
    return voices.value.find(voice => voice.voiceURI === state.voiceURI) || null
  }

  function loadVoices() {
    if (!state.supported) return
    try {
      voices.value = sortTTSVoices(synth.getVoices())
      const index = voices.value.findIndex(voice => voice.voiceURI === state.voiceURI)
      state.voiceIndex = index
      if (state.voiceURI && index < 0) state.voiceURI = ''
    } catch {
      voices.value = []
      state.voiceURI = ''
      state.voiceIndex = -1
      state.supported = false
    }
  }

  loadVoices()
  if (state.supported && typeof synth.addEventListener === 'function') {
    synth.addEventListener('voiceschanged', loadVoices)
  } else if (state.supported && 'onvoiceschanged' in synth) {
    previousVoicesChanged = synth.onvoiceschanged
    synth.onvoiceschanged = loadVoices
    usesVoicesChangedProperty = true
  }

  function stop() {
    if (state.supported && typeof synth.cancel === 'function') synth.cancel()
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
    if (!state.supported || typeof synth.pause !== 'function') return
    if (state.playing && !state.paused) {
      synth.pause()
      state.paused = true
    }
  }

  function resume() {
    if (!state.supported || typeof synth.resume !== 'function') return
    if (state.paused) {
      synth.resume()
      state.paused = false
    }
  }

  function speak(text, onEnd, onStart, onError) {
    return speakList(String(text || '').split('\n'), 0, onEnd, onStart, onError)
  }

  function speakList(list, startIndex = 0, onEnd, onStart, onError) {
    if (!state.supported || !hasSelectedVoice.value || typeof synth.speak !== 'function') return false
    stop()
    paragraphs = (Array.isArray(list) ? list : [])
      .map(line => String(line || '').trim())
      .filter(Boolean)
    if (paragraphs.length === 0) return false

    state.playing = true
    state.paused = false
    currentIndex.value = Math.max(0, Math.min(paragraphs.length - 1, Number(startIndex) || 0))
    total.value = paragraphs.length
    pending = false
    activeOnEnd = onEnd
    activeOnStart = onStart
    activeOnError = onError
    speakCurrent(onEnd, onStart, onError)
    return true
  }

  function utteranceConstructor() {
    if (typeof window !== 'undefined' && typeof window.SpeechSynthesisUtterance === 'function') {
      return window.SpeechSynthesisUtterance
    }
    if (typeof globalThis.SpeechSynthesisUtterance === 'function') {
      return globalThis.SpeechSynthesisUtterance
    }
    return null
  }

  function speakCurrent(onEnd, onStart, onError) {
    if (currentIndex.value >= paragraphs.length) {
      stop()
      onEnd?.()
      return
    }

    const Utterance = utteranceConstructor()
    const voice = selectedVoice()
    if (!Utterance || !voice) {
      stop()
      onError?.(new Error(!voice ? 'voice-not-selected' : 'speech-utterance-unavailable'))
      return
    }

    const utterance = new Utterance(paragraphs[currentIndex.value])
    utterance.rate = state.rate
    utterance.pitch = state.pitch
    utterance.voice = voice

    utterance.addEventListener('start', () => {
      onStart?.(currentIndex.value)
    })

    utterance.addEventListener('end', () => {
      if (pending) return
      currentIndex.value += 1
      if (currentIndex.value < paragraphs.length) {
        speakCurrent(onEnd, onStart, onError)
      } else {
        stop()
        onEnd?.()
      }
    })

    utterance.addEventListener('error', (event) => {
      pending = false
      state.playing = Boolean(synth.speaking)
      onError?.(event)
    })

    synth.speak(utterance)
  }

  function skipForward() {
    if (!state.supported || typeof synth.cancel !== 'function') return
    if (currentIndex.value < paragraphs.length - 1) {
      synth.cancel()
      pending = true
      currentIndex.value += 1
      setTimeout(() => {
        pending = false
        speakCurrent(activeOnEnd, activeOnStart, activeOnError)
      }, 50)
    }
  }

  function skipBackward() {
    if (!state.supported || typeof synth.cancel !== 'function') return
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
    if (
      !state.supported
      || typeof synth.cancel !== 'function'
      || !state.playing
      || currentIndex.value < 0
      || currentIndex.value >= paragraphs.length
      || !hasSelectedVoice.value
    ) return
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
    state.voiceURI = String(uri || '')
    state.voiceIndex = voices.value.findIndex(voice => voice.voiceURI === state.voiceURI)
    restartCurrent()
  }

  function dispose() {
    stop()
    if (state.supported && typeof synth.removeEventListener === 'function') {
      synth.removeEventListener('voiceschanged', loadVoices)
    }
    if (usesVoicesChangedProperty && synth.onvoiceschanged === loadVoices) {
      synth.onvoiceschanged = previousVoicesChanged
    }
  }

  if (getCurrentInstance()) onBeforeUnmount(dispose)

  return {
    state,
    voices,
    hasSelectedVoice,
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
