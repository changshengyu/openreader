import { onBeforeUnmount, reactive, ref, watch } from 'vue'

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

  function loadVoices() {
    if (!synth) return
    const availableVoices = synth.getVoices()
    voices.value = availableVoices.filter(v => v.lang.startsWith('zh') || v.lang.startsWith('en'))
    if (voices.value.length === 0) {
      voices.value = availableVoices
    }
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

  function speak(text, onEnd, onStart) {
    if (!synth) return
    stop()
    paragraphs = text.split('\n').map(l => l.trim()).filter(Boolean)
    if (paragraphs.length === 0) return

    state.playing = true
    state.paused = false
    currentIndex.value = 0
    total.value = paragraphs.length
    pending = false
    activeOnEnd = onEnd
    activeOnStart = onStart
    speakCurrent(onEnd, onStart)
  }

  function speakCurrent(onEnd, onStart) {
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
        speakCurrent(onEnd, onStart)
      } else {
        stop()
        onEnd?.()
      }
    })

    utterance.addEventListener('error', () => {
      pending = false
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
        speakCurrent(activeOnEnd, activeOnStart)
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
        speakCurrent(activeOnEnd, activeOnStart)
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
      speakCurrent(activeOnEnd, activeOnStart)
      if (wasPaused) setTimeout(() => pause(), 0)
    }, 50)
  }

  function setRate(rate) {
    state.rate = Math.max(0.5, Math.min(3, Number(rate) || 1))
    restartCurrent()
  }

  function setPitch(pitch) {
    state.pitch = Math.max(0.5, Math.min(2, Number(pitch) || 1))
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
