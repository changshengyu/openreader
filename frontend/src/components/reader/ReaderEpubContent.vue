<template>
  <iframe
    ref="frame"
    class="epub-iframe"
    :src="resource.url"
    :style="{ height: `${height}px` }"
    sandbox="allow-same-origin allow-scripts"
    title="EPUB 正文"
    @load="handleNativeLoad"
    @error="handleNativeError"
  />
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useReaderEpubFrame } from '../../composables/useReaderEpubFrame.js'

const props = defineProps({
  resource: {
    type: Object,
    required: true,
  },
  styleText: {
    type: String,
    default: '',
  },
  viewportHeight: {
    type: Number,
    default: 0,
  },
})

const emit = defineEmits([
  'ready',
  'load',
  'height',
  'click-point',
  'hash',
  'navigate',
  'keydown',
  'preview',
  'error',
])

const frame = ref(null)
const height = ref(Math.max(1, props.viewportHeight * 0.8))
let ready = false
let readinessTimer = null

function clearReadinessTimer() {
  window.clearTimeout(readinessTimer)
  readinessTimer = null
}

const bridge = useReaderEpubFrame({
  frame,
  resourceUrl: () => props.resource.url,
  expectedOrigin: () => window.location.origin,
  viewportHeight: () => props.viewportHeight || window.innerHeight,
  styleText: () => props.styleText,
  onReady: () => {
    ready = true
    clearReadinessTimer()
    emit('ready')
  },
  onLoad: data => emit('load', data),
  onHeight: nextHeight => {
    height.value = nextHeight
    emit('height', nextHeight)
  },
  onClick: point => emit('click-point', point),
  onHash: rect => emit('hash', rect),
  onNavigate: location => emit('navigate', location),
  onKeydown: event => emit('keydown', event),
  onPreview: data => emit('preview', data),
})

function handleWindowMessage(event) {
  bridge.handleMessage(event)
}

function handleNativeLoad() {
  clearReadinessTimer()
  if (ready) return
  readinessTimer = window.setTimeout(() => {
    if (!ready) emit('error', new Error('EPUB 正文加载失败，请重试'))
  }, 1500)
}

function handleNativeError() {
  clearReadinessTimer()
  emit('error', new Error('EPUB 正文资源无法加载'))
}

watch(
  () => props.styleText,
  () => bridge.syncStyle(),
)

watch(
  () => props.resource.url,
  () => {
    ready = false
    height.value = Math.max(1, (props.viewportHeight || window.innerHeight) * 0.8)
    clearReadinessTimer()
  },
)

onMounted(() => {
  window.addEventListener('message', handleWindowMessage)
})

onBeforeUnmount(() => {
  clearReadinessTimer()
  window.removeEventListener('message', handleWindowMessage)
})
</script>

<style scoped>
.epub-iframe {
  display: block;
  width: 100%;
  min-height: 50vh;
  border: 0;
  background: transparent;
}
</style>
