<template>
  <div class="tts-bar" @click.stop @touchstart.stop @touchmove.stop @touchend.stop>
    <div class="tts-main">
      <button class="tts-btn tts-close" type="button" title="关闭朗读栏" @click="$emit('close')">×</button>
      <button class="tts-text-btn" type="button" @click="$emit('backward')">上一段</button>
      <button
        class="tts-btn tts-play"
        type="button"
        :aria-label="playing && !paused ? '暂停朗读' : '开始朗读'"
        @click="$emit(playing ? (paused ? 'resume' : 'pause') : 'play')"
      >
        {{ playing && !paused ? '⏸' : '▶' }}
      </button>
      <button class="tts-text-btn" type="button" @click="$emit('forward')">下一段</button>
      <span class="tts-progress">{{ progressText }}</span>
      <button class="tts-btn" type="button" title="展开/收起朗读设置" @click="$emit('toggle-config')">
        {{ configExpanded ? '⌄' : '⌃' }}
      </button>
    </div>

    <div v-if="configExpanded" class="tts-config">
      <div class="tts-row tts-voice-row">
        <span class="tts-label">语音库</span>
        <div v-if="voices.length" class="tts-voice-list" role="radiogroup" aria-label="朗读语音库">
          <button
            v-for="voice in voices"
            :key="voice.voiceURI || voice.name"
            class="tts-voice-option"
            :class="{ active: voiceUri === voice.voiceURI }"
            type="button"
            role="radio"
            :aria-checked="voiceUri === voice.voiceURI"
            @click="$emit('voice-change', voice.voiceURI)"
          >
            {{ voice.name }} · {{ voice.lang }}
          </button>
        </div>
        <span v-else class="tts-empty-voice">暂无可用语音</span>
      </div>

      <div class="tts-row">
        <span class="tts-label">语速</span>
        <ReaderSettingStepper
          :model-value="rate"
          :min="0.5"
          :max="2"
          :step="0.1"
          decrease-label="降低朗读语速"
          increase-label="提高朗读语速"
          edit-label="自定义朗读语速"
          @update:model-value="$emit('rate-change', $event)"
        />
      </div>

      <div class="tts-row">
        <span class="tts-label">音调</span>
        <ReaderSettingStepper
          :model-value="pitch"
          :min="0"
          :max="2"
          :step="0.1"
          decrease-label="降低朗读音调"
          increase-label="提高朗读音调"
          edit-label="自定义朗读音调"
          @update:model-value="$emit('pitch-change', $event)"
        />
      </div>

      <div class="tts-row">
        <span class="tts-label">定时</span>
        <ReaderSettingStepper
          :model-value="sleepMinutes"
          :min="0"
          :max="180"
          :step="1"
          decrease-label="减少朗读定时"
          increase-label="增加朗读定时"
          edit-label="自定义朗读定时分钟数"
          @update:model-value="$emit('sleep-change', $event)"
        />
        <span class="tts-unit">分钟</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import ReaderSettingStepper from './ReaderSettingStepper.vue'

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
  right: calc(50vw - var(--reader-frame-width) / 2);
  bottom: 0;
  z-index: 220;
  display: flex;
  width: min(500px, 100vw);
  max-height: min(440px, 70vh);
  flex-direction: column;
  gap: 8px;
  padding: 10px 14px max(10px, env(safe-area-inset-bottom));
  overflow: hidden;
  color: #fff;
  background: rgba(64, 158, 255, 0.94);
  border-radius: 8px 8px 0 0;
  box-shadow: 0 -4px 18px rgba(0, 0, 0, 0.16);
}

.tts-main,
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
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
}

.tts-row {
  width: 100%;
  gap: 8px;
}

.tts-row :deep(.reader-setting-stepper) {
  flex: 1;
}

.tts-voice-row {
  align-items: flex-start;
}

.tts-voice-list {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 6px;
  padding-bottom: 3px;
  overflow-x: auto;
  overscroll-behavior-x: contain;
}

.tts-voice-option {
  min-height: 32px;
  flex: 0 0 auto;
  padding: 5px 9px;
  color: #fff;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
}

.tts-voice-option.active {
  color: #1f5f9f;
  background: #fff;
  border-color: #fff;
}

.tts-btn,
.tts-text-btn {
  min-height: 34px;
  padding: 4px 8px;
  color: #fff;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.tts-btn {
  font-size: 18px;
}

.tts-close {
  font-size: 22px;
}

.tts-play {
  font-size: 20px;
}

.tts-label,
.tts-unit,
.tts-empty-voice,
.tts-progress {
  font-size: 12px;
  white-space: nowrap;
}

.tts-label,
.tts-unit,
.tts-empty-voice {
  padding-top: 9px;
  color: rgba(255, 255, 255, 0.76);
}

.tts-progress {
  color: #fff;
}

@media (max-width: 750px) {
  .tts-bar {
    right: 0;
    bottom: 0;
    left: 0;
    width: 100vw;
    max-height: min(440px, 72vh);
    min-width: 0;
    padding-right: 10px;
    padding-left: 10px;
    border-radius: 0;
    transform: none;
  }

  .tts-main {
    justify-content: space-between;
    gap: 2px;
  }

  .tts-main button {
    padding-right: 5px;
    padding-left: 5px;
  }

  .tts-progress {
    max-width: 82px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
