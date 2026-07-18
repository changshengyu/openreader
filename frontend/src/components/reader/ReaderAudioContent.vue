<template>
  <section class="reader-audio-content content-audio">
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

    <div class="reader-audio-cover primary">
      <img v-if="coverUrl" :src="coverUrl" :alt="bookTitle || title || '音频封面'" @click.stop />
      <span v-else>有声</span>
    </div>

    <div class="reader-audio-progress book-progress" @click.stop>
      <span class="reader-audio-time progress-tip">{{ elapsedLabel }}</span>
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
      <span class="reader-audio-time progress-tip total-time">{{ durationLabel }}</span>
    </div>

    <div class="reader-audio-actions primary book-operation" @click.stop>
      <button type="button" @click="seekBy(-15)">-15s</button>
      <button type="button" @click="handlePrevious">上一章</button>
      <button class="reader-audio-play" type="button" :aria-pressed="playing" @click="togglePlay">
        {{ playing ? '暂停' : '播放' }}
      </button>
      <button type="button" @click="handleNext">下一章</button>
      <button type="button" @click="seekBy(15)">+15s</button>
    </div>

    <div class="reader-audio-volume book-operation" @click.stop>
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

    <div class="reader-audio-info book-info">
      <div class="reader-audio-info-cover book-cover">
        <img v-if="coverUrl" :src="coverUrl" :alt="bookTitle || title || '音频封面'" @click.stop />
        <span v-else>有声</span>
      </div>
      <div class="reader-audio-intro book-intro">
        <div class="reader-audio-book-title title">{{ title || '音频章节' }}</div>
        <div class="reader-audio-author subtitle">
          {{ bookTitle || '有声阅读' }}<template v-if="author"> · {{ author }}</template>
        </div>
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
  bookTitle: {
    type: String,
    default: '',
  },
  author: {
    type: String,
    default: '',
  },
  coverUrl: {
    type: String,
    default: '',
  },
  autoplay: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits([
  'autoplay-blocked',
  'ended',
  'error',
  'loaded',
  'next',
  'play',
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
let metadataReadyForURL = ''
let autoplayAttemptedForURL = ''

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
    metadataReadyForURL = ''
    autoplayAttemptedForURL = ''
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

watch(
  () => props.autoplay,
  (requested) => {
    if (requested) void attemptAutoplay()
  },
)

async function handleLoadedMetadata() {
  duration.value = Number(audioEl.value?.duration || 0)
  metadataReadyForURL = props.resource.url
  restoreInitialTime()
  syncAudioVolume()
  emitProgress()
  emit('loaded', {
    currentTime: currentTime.value,
    duration: duration.value,
  })
  await attemptAutoplay()
}

function handleTimeUpdate() {
  currentTime.value = Number(audioEl.value?.currentTime || 0)
  duration.value = Number(audioEl.value?.duration || duration.value || 0)
  emitProgress()
}

function handlePlay() {
  playing.value = true
  emit('play')
}

function handlePause() {
  playing.value = false
}

function handleEnded() {
  playing.value = false
  emit('ended')
}

function handleError(event) {
  playing.value = false
  emit('error', event)
}

async function attemptAutoplay() {
  const audio = audioEl.value
  const url = props.resource.url
  if (
    !audio
    || !props.autoplay
    || !url
    || metadataReadyForURL !== url
    || autoplayAttemptedForURL === url
  ) return
  autoplayAttemptedForURL = url
  try {
    await audio.play()
  } catch (error) {
    playing.value = false
    emit('autoplay-blocked', error)
  }
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
  } catch (error) {
    playing.value = false
    emit('error', error)
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
  emit('previous')
}

function handleNext() {
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
  width: 100%;
  min-height: min(680px, calc(100vh - 168px));
  margin: 0 auto;
  padding: 24px 0;
  color: var(--reader-text);
}

.reader-audio-cover.primary {
  display: grid;
  width: min(200px, 58vw);
  aspect-ratio: 3 / 4;
  margin: 0 auto;
  overflow: hidden;
  background: rgba(70, 56, 25, 0.1);
  color: color-mix(in srgb, var(--reader-text) 58%, transparent);
  font-size: 28px;
  font-weight: 700;
  place-items: center;
}

.reader-audio-cover.primary img,
.reader-audio-info-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.reader-audio-element {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.reader-audio-progress {
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr) 48px;
  gap: 10px;
  align-items: center;
  padding: 25px 15px;
}

.reader-audio-progress input,
.reader-audio-volume input {
  width: 100%;
  accent-color: #409eff;
}

.reader-audio-time {
  font-size: 14px;
  font-variant-numeric: tabular-nums;
  text-indent: 0;
}

.reader-audio-time.total-time {
  text-align: right;
}

.reader-audio-actions,
.reader-audio-volume {
  display: grid;
  gap: 10px;
  align-items: center;
  padding: 0 15px 25px;
}

.reader-audio-actions {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.reader-audio-volume {
  grid-template-columns: 58px minmax(0, 180px) 48px;
  justify-content: center;
  font-variant-numeric: tabular-nums;
}

.reader-audio-actions button,
.reader-audio-volume button {
  min-height: 38px;
  border: 0;
  background: transparent;
  color: var(--reader-text);
  cursor: pointer;
}

.reader-audio-play {
  color: #409eff !important;
  font-weight: 700;
}

.reader-audio-volume span {
  text-align: right;
}

.reader-audio-info {
  display: flex;
  align-items: center;
  padding: 10px 15px;
  background: color-mix(in srgb, var(--reader-popup-bg) 82%, transparent);
}

.reader-audio-info-cover {
  display: grid;
  width: 50px;
  height: 66px;
  flex: 0 0 auto;
  overflow: hidden;
  background: rgba(70, 56, 25, 0.1);
  font-size: 12px;
  place-items: center;
}

.reader-audio-intro {
  min-width: 0;
  padding-left: 15px;
}

.reader-audio-book-title {
  overflow: hidden;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reader-audio-author {
  margin-top: 5px;
  overflow: hidden;
  color: color-mix(in srgb, var(--reader-text) 62%, transparent);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .reader-audio-content {
    min-height: calc(100vh - 150px);
    padding-top: 16px;
  }

  .reader-audio-progress {
    padding: 20px 8px;
  }

  .reader-audio-actions,
  .reader-audio-volume {
    gap: 6px;
    padding-right: 8px;
    padding-left: 8px;
  }

  .reader-audio-actions button,
  .reader-audio-volume button {
    min-height: 34px;
    padding: 0 2px;
    font-size: 12px;
  }

  .reader-audio-volume {
    grid-template-columns: 48px minmax(0, 180px) 42px;
  }
}
</style>
