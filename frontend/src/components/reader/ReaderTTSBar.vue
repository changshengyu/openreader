<template>
  <div class="tts-bar">
    <div class="tts-main">
      <el-button text class="tts-btn tts-close" title="关闭朗读栏" @click="$emit('close')">×</el-button>
      <el-button text class="tts-text-btn" @click="$emit('backward')">上一段</el-button>
      <el-button text class="tts-btn tts-play" @click="$emit(playing ? (paused ? 'resume' : 'pause') : 'play')">
        {{ playing && !paused ? '⏸' : '▶' }}
      </el-button>
      <el-button text class="tts-text-btn" @click="$emit('forward')">下一段</el-button>
      <span class="tts-progress">{{ progressText }}</span>
      <el-button text class="tts-btn" title="展开/收起朗读设置" @click="$emit('toggle-config')">
        {{ configExpanded ? '⌄' : '⌃' }}
      </el-button>
    </div>
    <div v-if="configExpanded" class="tts-config">
      <label class="tts-row tts-voice-row">
        <span class="tts-label">语音库</span>
        <select
          class="tts-select"
          :value="voiceUri"
          :disabled="!voices.length"
          @change="$emit('voice-change', $event.target.value)"
        >
          <option value="">浏览器默认</option>
          <option
            v-for="voice in voices"
            :key="voice.voiceURI || voice.name"
            :value="voice.voiceURI"
          >
            {{ voice.name }} · {{ voice.lang }}
          </option>
        </select>
      </label>
      <label class="tts-row">
        <span class="tts-label">语速</span>
        <input :value="rate" max="2" min="0.5" step="0.1" type="range" class="tts-slider" @input="$emit('rate-change', $event.target.value)" />
        <button class="tts-reset" type="button" @click="$emit('rate-change', 1)">重置</button>
      </label>
      <label class="tts-row">
        <span class="tts-label">音调</span>
        <input :value="pitch" max="2" min="0" step="0.1" type="range" class="tts-slider" @input="$emit('pitch-change', $event.target.value)" />
        <button class="tts-reset" type="button" @click="$emit('pitch-change', 1)">重置</button>
      </label>
      <label class="tts-row">
        <span class="tts-label">定时</span>
        <input :value="sleepMinutes" max="180" min="0" step="1" type="range" class="tts-slider" @input="$emit('sleep-change', $event.target.value)" />
        <span class="tts-label">{{ sleepMinutes }}分钟</span>
      </label>
    </div>
  </div>
</template>

<script setup>
defineProps({
  playing: {
    type: Boolean,
    default: false,
  },
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
  voices: {
    type: Array,
    default: () => [],
  },
  voiceUri: {
    type: String,
    default: '',
  },
  configExpanded: {
    type: Boolean,
    default: true,
  },
})

defineEmits([
  'backward',
  'play',
  'pause',
  'resume',
  'forward',
  'close',
  'toggle-config',
  'voice-change',
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
  z-index: 220;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  min-width: min(500px, calc(100vw - 20px));
  padding: 10px 14px;
  color: #fff;
  background: rgba(64, 158, 255, 0.9);
  border-radius: 10px;
  transform: translateX(-50%);
}

.tts-main,
.tts-config,
.tts-row {
  display: flex;
  align-items: center;
}

.tts-main {
  width: 100%;
  justify-content: center;
  gap: 8px;
}

.tts-config {
  width: 100%;
  flex-direction: column;
  gap: 8px;
}

.tts-row {
  width: 100%;
  justify-content: center;
  gap: 8px;
}

.tts-voice-row {
  align-items: stretch;
}

.tts-btn {
  color: #fff !important;
  font-size: 18px;
}

.tts-text-btn {
  color: #fff !important;
}

.tts-close {
  font-size: 22px;
}

.tts-play {
  font-size: 20px;
}

.tts-label {
  color: rgba(255, 255, 255, 0.7);
  font-size: 12px;
  white-space: nowrap;
}

.tts-progress {
  color: #fff;
  font-size: 12px;
  white-space: nowrap;
}

.tts-select {
  min-width: 180px;
  max-width: 320px;
  height: 28px;
  color: #1f2937;
  background: #fff;
  border: 0;
  border-radius: 4px;
}

.tts-slider {
  width: 180px;
  accent-color: #fff;
}

.tts-reset {
  padding: 4px 8px;
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
  border: 0;
  border-radius: 4px;
}

@media (max-width: 750px) {
  .tts-bar {
    right: 10px;
    bottom: max(74px, calc(env(safe-area-inset-bottom) + 74px));
    left: 10px;
    min-width: 0;
    justify-content: center;
    overflow-x: auto;
    transform: none;
  }

  .tts-main,
  .tts-row {
    justify-content: flex-start;
  }

  .tts-slider,
  .tts-select {
    flex: 1;
    min-width: 0;
  }
}
</style>
