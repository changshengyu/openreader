<template>
  <section class="reader-audio-content">
    <div class="reader-audio-card">
      <p class="reader-audio-kicker">有声阅读</p>
      <h1>{{ title || '音频章节' }}</h1>
      <p class="reader-audio-time">{{ elapsedLabel }} / {{ durationLabel }}</p>
      <audio
        ref="audioEl"
        class="reader-audio-element"
        :src="resource.url"
        controls
        preload="metadata"
        :autoplay="autoplay"
        @click.stop
        @loadedmetadata="handleLoadedMetadata"
        @timeupdate="handleTimeUpdate"
        @ended="emit('ended')"
        @error="emit('error')"
      />
      <div class="reader-audio-actions" @click.stop>
        <button type="button" :disabled="previousDisabled" @click="emit('previous')">上一章</button>
        <button type="button" @click="seekBy(-15)">-15s</button>
        <button type="button" @click="seekBy(15)">+15s</button>
        <button type="button" :disabled="nextDisabled" @click="emit('next')">下一章</button>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'

const props = defineProps({
  resource: {
    type: Object,
    required: true,
  },
  initialTime: {
    type: Number,
    default: 0,
  },
  title: {
    type: String,
    default: '',
  },
  previousDisabled: {
    type: Boolean,
    default: false,
  },
  nextDisabled: {
    type: Boolean,
    default: false,
  },
  autoplay: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits([
  'ended',
  'error',
  'loaded',
  'next',
  'previous',
  'progress',
])

const audioEl = ref(null)
const currentTime = ref(0)
const duration = ref(0)
let restoredForURL = ''

const elapsedLabel = computed(() => formatAudioTime(currentTime.value))
const durationLabel = computed(() => (
  duration.value > 0 ? formatAudioTime(duration.value) : '--:--'
))

watch(
  () => props.resource.url,
  async () => {
    restoredForURL = ''
    currentTime.value = 0
    duration.value = 0
    await nextTick()
    restoreInitialTime()
  },
)

watch(
  () => props.initialTime,
  () => restoreInitialTime(),
)

function handleLoadedMetadata() {
  duration.value = Number(audioEl.value?.duration || 0)
  restoreInitialTime()
  emitProgress()
  emit('loaded', {
    currentTime: currentTime.value,
    duration: duration.value,
  })
}

function handleTimeUpdate() {
  currentTime.value = Number(audioEl.value?.currentTime || 0)
  duration.value = Number(audioEl.value?.duration || duration.value || 0)
  emitProgress()
}

function seekBy(seconds) {
  const audio = audioEl.value
  if (!audio) return
  const max = Number.isFinite(audio.duration) ? audio.duration : Infinity
  audio.currentTime = Math.max(0, Math.min(max, Number(audio.currentTime || 0) + seconds))
  handleTimeUpdate()
}

function restoreInitialTime() {
  const audio = audioEl.value
  if (!audio || restoredForURL === props.resource.url) return
  const target = Math.max(0, Number(props.initialTime) || 0)
  if (!target) {
    restoredForURL = props.resource.url
    return
  }
  const max = Number.isFinite(audio.duration) ? audio.duration : target
  try {
    audio.currentTime = Math.min(target, max)
    currentTime.value = Number(audio.currentTime || 0)
    restoredForURL = props.resource.url
  } catch {
    // Some browsers reject seeking until enough metadata is available.
  }
}

function emitProgress() {
  emit('progress', {
    currentTime: currentTime.value,
    duration: duration.value,
  })
}

function formatAudioTime(value) {
  const total = Math.max(0, Math.floor(Number(value) || 0))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  }
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}
</script>

<style scoped>
.reader-audio-content {
  display: grid;
  min-height: min(680px, calc(100vh - 168px));
  padding: 40px 0;
  place-items: center;
}

.reader-audio-card {
  display: grid;
  width: min(100%, 520px);
  gap: 18px;
  padding: 34px 32px;
  border: 1px solid rgba(70, 56, 25, 0.16);
  border-radius: 20px;
  background: rgba(255, 252, 239, 0.46);
  box-shadow: 0 18px 50px rgba(44, 32, 12, 0.12);
  text-align: center;
}

.reader-audio-kicker {
  margin: 0;
  color: rgba(46, 38, 24, 0.58);
  font-size: 14px;
  letter-spacing: 0.18em;
  text-indent: 0;
}

h1 {
  margin: 0;
  color: var(--reader-text);
  font-size: var(--reader-heading-size);
  line-height: 1.35;
}

.reader-audio-time {
  margin: 0;
  color: rgba(46, 38, 24, 0.62);
  font-variant-numeric: tabular-nums;
  text-indent: 0;
}

.reader-audio-element {
  width: 100%;
}

.reader-audio-actions {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.reader-audio-actions button {
  min-height: 38px;
  border: 1px solid rgba(70, 56, 25, 0.18);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.56);
  color: var(--reader-text);
  cursor: pointer;
}

.reader-audio-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

@media (max-width: 640px) {
  .reader-audio-content {
    min-height: calc(100vh - 150px);
    padding: 24px 0;
  }

  .reader-audio-card {
    gap: 14px;
    padding: 26px 18px;
    border-radius: 16px;
  }

  .reader-audio-actions {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
