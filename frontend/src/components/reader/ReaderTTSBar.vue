<template>
  <div class="tts-bar">
    <el-button text class="tts-btn" @click="$emit('backward')">‹</el-button>
    <el-button text class="tts-btn" @click="$emit(paused ? 'resume' : 'pause')">
      {{ paused ? '▶' : '⏸' }}
    </el-button>
    <el-button text class="tts-btn" @click="$emit('forward')">›</el-button>
    <el-button text class="tts-btn" @click="$emit('stop')">⏹</el-button>
    <span class="tts-progress">{{ progressText }}</span>
    <span class="tts-label">语速</span>
    <input :value="rate" max="3" min="0.5" step="0.1" type="range" class="tts-slider" @input="$emit('rate-change', $event.target.value)" />
    <span class="tts-label">音调</span>
    <input :value="pitch" max="2" min="0.5" step="0.1" type="range" class="tts-slider" @input="$emit('pitch-change', $event.target.value)" />
    <span class="tts-label">定时</span>
    <input :value="sleepMinutes" max="180" min="0" step="1" type="range" class="tts-slider" @input="$emit('sleep-change', $event.target.value)" />
    <span class="tts-label">{{ sleepMinutes }}分钟</span>
  </div>
</template>

<script setup>
defineProps({
  paused: {
    type: Boolean,
    default: false,
  },
  rate: {
    type: Number,
    default: 1,
  },
  pitch: {
    type: Number,
    default: 1,
  },
  sleepMinutes: {
    type: Number,
    default: 0,
  },
  progressText: {
    type: String,
    default: '段落 - / -',
  },
})

defineEmits([
  'backward',
  'pause',
  'resume',
  'forward',
  'stop',
  'rate-change',
  'pitch-change',
  'sleep-change',
])
</script>

<style scoped>
.tts-bar {
  position: fixed;
  bottom: 16px;
  left: 50%;
  z-index: 6;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 18px;
  color: #fff;
  background: rgba(64, 158, 255, 0.9);
  border-radius: 10px;
  transform: translateX(-50%);
}

.tts-btn {
  color: #fff !important;
  font-size: 18px;
}

.tts-label {
  color: rgba(255, 255, 255, 0.7);
  font-size: 12px;
}

.tts-progress {
  color: #fff;
  font-size: 12px;
  white-space: nowrap;
}

.tts-slider {
  width: 60px;
  accent-color: #fff;
}

@media (max-width: 750px) {
  .tts-bar {
    right: 10px;
    bottom: max(74px, calc(env(safe-area-inset-bottom) + 74px));
    left: 10px;
    justify-content: center;
    overflow-x: auto;
    transform: none;
  }
}
</style>
