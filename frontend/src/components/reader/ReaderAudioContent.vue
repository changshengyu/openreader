<template>
  <section class="reader-audio-content">
    <div class="reader-audio-card">
      <div class="reader-audio-cover" :class="{ empty: !coverUrl }">
        <img v-if="coverUrl" :src="coverUrl" :alt="title || '音频封面'" @click.stop />
        <span v-else>有声</span>
      </div>
      <p class="reader-audio-kicker">有声阅读</p>
      <h1>{{ title || '音频章节' }}</h1>
      <p class="reader-audio-time">{{ elapsedLabel }} / {{ durationLabel }}</p>
      <audio
        ref="audioEl"
        class="reader-audio-element"
        :src="resource.url"
        preload="metadata"
        :autoplay="autoplay"
        @click.stop
        @play="handlePlay"
        @pause="handlePause"
        @loadedmetadata="handleLoadedMetadata"
        @timeupdate="handleTimeUpdate"
        @ended="handleEnded"
        @error="handleError"
      />
      <div class="reader-audio-progress" @click.stop>
        <input
          type="range"
          min="0"
          :max="durationSliderMax"
          step="1"
          :value="currentTimeSliderValue"
          aria-label="音频播放进度"
          @input="handleSeekInput"
          @change="handleSeekChange"
        />
      </div>
      <div class="reader-audio-actions primary" @click.stop>
        <button type="button" @click="seekBy(-15)">-15s</button>
        <button type="button" :disabled="previousDisabled" @click="handlePrevious">上一章</button>
        <button class="reader-audio-play" type="button" :aria-pressed="playing" @click="togglePlay">
          {{ playing ? '暂停' : '播放' }}
        </button>
        <button type="button" :disabled="nextDisabled" @click="handleNext">下一章</button>
        <button type="button" @click="seekBy(15)">+15s</button>
      </div>
      <div class="reader-audio-volume" @click.stop>
        <button type="button" :aria-pressed="muted" @click="toggleMute">
          {{ muted || volume <= 0 ? '静音' : '音量' }}
        </button>
        <input
          type="range"
          min="0"
          max="100"
          step="1"
          :value="volume"
          aria-label="音频音量"
          @input="handleVolumeInput"
          @change="handleVolumeInput"
        />
        <span>{{ volume }}%</span>
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
  coverUrl: {
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
const playing = ref(false)
const volume = ref(100)
const muted = ref(false)
const previousVolume = ref(100)
let restoredForURL = ''

const elapsedLabel = computed(() => formatAudioTime(currentTime.value))
const durationLabel = computed(() => (
  duration.value > 0 ? formatAudioTime(duration.value) : '--:--'
))
const durationSliderMax = computed(() => Math.max(1, Math.floor(Number(duration.value) || 0)))
const currentTimeSliderValue = computed(() => Math.max(0, Math.floor(Number(currentTime.value) || 0)))

watch(
  () => props.resource.url,
  async () => {
    restoredForURL = ''
    currentTime.value = 0
    duration.value = 0
    playing.value = false
    await nextTick()
    restoreInitialTime()
    syncAudioVolume()
  },
)

watch(
  () => props.initialTime,
  () => restoreInitialTime(),
)

function handleLoadedMetadata() {
  duration.value = Number(audioEl.value?.duration || 0)
  restoreInitialTime()
  syncAudioVolume()
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

function handlePlay() {
  playing.value = true
}

function handlePause() {
  playing.value = false
}

function handleEnded() {
  playing.value = false
  emit('ended')
}

function handleError() {
  playing.value = false
  emit('error')
}

async function togglePlay() {
  const audio = audioEl.value
  if (!audio) return
  if (playing.value) {
    audio.pause()
    return
  }
  try {
    await audio.play()
  } catch {
    playing.value = false
  }
}

function seekBy(seconds) {
  const audio = audioEl.value
  if (!audio) return
  const max = Number.isFinite(audio.duration) ? audio.duration : Infinity
  audio.currentTime = Math.max(0, Math.min(max, Number(audio.currentTime || 0) + seconds))
  handleTimeUpdate()
}

function handleSeekInput(event) {
  currentTime.value = Math.max(0, Number(event?.target?.value) || 0)
}

function handleSeekChange(event) {
  const audio = audioEl.value
  if (!audio) return
  const target = Math.max(0, Number(event?.target?.value) || 0)
  const max = Number.isFinite(audio.duration) ? audio.duration : target
  audio.currentTime = Math.min(target, max)
  handleTimeUpdate()
}

function handlePrevious() {
  playing.value = false
  emit('previous')
}

function handleNext() {
  playing.value = false
  emit('next')
}

function handleVolumeInput(event) {
  setVolume(Number(event?.target?.value))
}

function toggleMute() {
  if (muted.value || volume.value <= 0) {
    setVolume(previousVolume.value > 0 ? previousVolume.value : 100)
    return
  }
  previousVolume.value = volume.value
  setVolume(0)
}

function setVolume(value) {
  const next = Math.max(0, Math.min(100, Math.round(Number(value) || 0)))
  volume.value = next
  muted.value = next <= 0
  if (next > 0) previousVolume.value = next
  syncAudioVolume()
}

function syncAudioVolume() {
  const audio = audioEl.value
  if (!audio) return
  audio.volume = Math.max(0, Math.min(1, volume.value / 100))
  audio.muted = muted.value || volume.value <= 0
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

.reader-audio-cover {
  display: grid;
  width: 148px;
  height: 196px;
  margin: 0 auto 4px;
  overflow: hidden;
  border-radius: 14px;
  background:
    linear-gradient(135deg, rgba(77, 57, 24, 0.22), rgba(255, 255, 255, 0.34)),
    var(--reader-bg);
  box-shadow: 0 12px 32px rgba(32, 22, 8, 0.2);
  color: rgba(46, 38, 24, 0.58);
  font-size: 28px;
  font-weight: 700;
  place-items: center;
}

.reader-audio-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.reader-audio-cover.empty span {
  letter-spacing: 0.18em;
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
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.reader-audio-progress input,
.reader-audio-volume input {
  width: 100%;
  accent-color: #409eff;
}

.reader-audio-actions {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}

.reader-audio-actions button,
.reader-audio-volume button {
  min-height: 38px;
  border: 1px solid rgba(70, 56, 25, 0.18);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.56);
  color: var(--reader-text);
  cursor: pointer;
}

.reader-audio-play {
  border-color: rgba(64, 158, 255, 0.45) !important;
  background: rgba(64, 158, 255, 0.14) !important;
  font-weight: 700;
}

.reader-audio-actions button:disabled,
.reader-audio-volume button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.reader-audio-volume {
  display: grid;
  grid-template-columns: 58px 1fr 48px;
  gap: 10px;
  align-items: center;
  color: rgba(46, 38, 24, 0.62);
  font-variant-numeric: tabular-nums;
}

.reader-audio-volume span {
  text-align: right;
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

  .reader-audio-cover {
    width: 118px;
    height: 156px;
  }

  .reader-audio-actions {
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 7px;
  }

  .reader-audio-actions button,
  .reader-audio-volume button {
    min-height: 34px;
    padding: 0 4px;
    font-size: 12px;
  }

  .reader-audio-volume {
    grid-template-columns: 48px 1fr 42px;
    gap: 8px;
  }
}
</style>
