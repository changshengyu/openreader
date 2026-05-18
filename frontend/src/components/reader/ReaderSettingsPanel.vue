<template>
  <div class="settings-body">
    <div class="setting-row">
      <label class="setting-label">阅读模式</label>
      <el-radio-group v-model="reader.mode" size="small" @change="$emit('modeChange', $event)">
        <el-radio-button value="scroll">滚动</el-radio-button>
        <el-radio-button value="flip">翻页</el-radio-button>
        <el-radio-button value="page">分页</el-radio-button>
      </el-radio-group>
    </div>

    <div class="setting-row">
      <label class="setting-label">主题</label>
      <div class="theme-grid">
        <span
          v-for="(preset, key) in themePresets"
          :key="key"
          class="theme-dot"
          :class="{ active: reader.theme === key }"
          :style="{ background: preset.bg }"
          :title="preset.label"
          @click="$emit('themeChange', key)"
        />
        <span
          class="theme-dot custom-dot"
          :class="{ active: reader.theme === 'custom' }"
          @click="$emit('themeChange', 'custom')"
        >+</span>
      </div>
    </div>

    <template v-if="reader.theme === 'custom'">
      <div class="setting-row">
        <label class="setting-label">背景色</label>
        <el-color-picker v-model="localCustomBg" size="small" @change="reader.setCustomBgColor($event)" />
      </div>
      <div class="setting-row">
        <label class="setting-label">背景图</label>
        <el-upload accept="image/*" :show-file-list="false" :auto-upload="false" @change="$emit('pickBgImage', $event)">
          <el-button size="small">上传</el-button>
        </el-upload>
      </div>
    </template>

    <div class="setting-row">
      <label class="setting-label">亮度</label>
      <el-slider v-model="reader.brightness" :min="50" :max="150" size="small" @input="reader.setBrightness" />
    </div>

    <div class="setting-row">
      <label class="setting-label">自动阅读速度 ({{ reader.autoReadSpeed }}px)</label>
      <el-slider v-model="reader.autoReadSpeed" :min="2" :max="40" :step="1" size="small" @input="reader.setAutoReadSpeed($event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">字体</label>
      <el-select v-model="reader.fontFamily" size="small" @change="reader.setFontFamily">
        <el-option v-for="font in fontOptions" :key="font.value" :label="font.label" :value="font.value" />
      </el-select>
    </div>

    <div class="setting-row">
      <label class="setting-label">字号 ({{ reader.fontSize }}px)</label>
      <div class="font-controls">
        <el-button size="small" :icon="Minus" circle @click="reader.setFontSize(reader.fontSize - 1)" />
        <el-slider v-model="reader.fontSize" :min="8" :max="36" size="small" class="font-slider" @input="reader.setFontSize" />
        <el-button size="small" :icon="Plus" circle @click="reader.setFontSize(reader.fontSize + 1)" />
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">行高 ({{ reader.lineHeight }})</label>
      <el-slider v-model="localLineHeight" :min="1" :max="5" :step="0.2" size="small" @input="reader.setLineHeight($event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">字重 ({{ reader.fontWeight }})</label>
      <el-slider v-model="reader.fontWeight" :min="300" :max="900" :step="100" size="small" @input="reader.setFontWeight($event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">段落间距 ({{ reader.paragraphSpace }}em)</label>
      <el-slider v-model="reader.paragraphSpace" :min="0" :max="3" :step="0.1" size="small" @input="reader.setParagraphSpace($event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">阅读宽度 ({{ reader.columnWidth }}px)</label>
      <el-slider v-model="reader.columnWidth" :min="560" :max="1080" :step="20" size="small" @input="reader.setColumnWidth($event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语速 ({{ reader.ttsRate }})</label>
      <el-slider v-model="reader.ttsRate" :min="0.5" :max="3" :step="0.1" size="small" @input="$emit('ttsRateChange', $event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读音调 ({{ reader.ttsPitch }})</label>
      <el-slider v-model="reader.ttsPitch" :min="0.5" :max="2" :step="0.1" size="small" @input="$emit('ttsPitchChange', $event)" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语音</label>
      <el-select
        v-model="reader.ttsVoiceURI"
        size="small"
        clearable
        :disabled="!tts.state.supported || !ttsVoices.length"
        placeholder="浏览器默认"
        @change="$emit('ttsVoiceChange', $event)"
      >
        <el-option label="浏览器默认" value="" />
        <el-option
          v-for="voice in ttsVoices"
          :key="voice.voiceURI"
          :label="`${voice.name} · ${voice.lang}`"
          :value="voice.voiceURI"
        />
      </el-select>
      <small v-if="!tts.state.supported" class="setting-help">当前浏览器不支持系统朗读。</small>
      <small v-else-if="!ttsVoices.length" class="setting-help">浏览器尚未返回可用语音，稍后再打开设置会自动刷新。</small>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Minus, Plus } from '@element-plus/icons-vue'

const props = defineProps({
  reader: { type: Object, required: true },
  tts: { type: Object, required: true },
  ttsVoices: { type: Array, default: () => [] },
  fontOptions: { type: Array, default: () => [] },
  themePresets: { type: Object, default: () => ({}) },
  customBg: { type: String, default: '' },
  lineHeight: { type: Number, default: 2.12 },
})

const emit = defineEmits([
  'update:customBg',
  'update:lineHeight',
  'modeChange',
  'themeChange',
  'pickBgImage',
  'ttsRateChange',
  'ttsPitchChange',
  'ttsVoiceChange',
])

const localCustomBg = computed({
  get: () => props.customBg,
  set: value => emit('update:customBg', value),
})

const localLineHeight = computed({
  get: () => props.lineHeight,
  set: value => emit('update:lineHeight', value),
})
</script>

<style scoped>
.settings-body {
  display: grid;
  gap: 20px;
}

.setting-row {
  display: grid;
  gap: 8px;
}

.setting-label {
  color: #666;
  font-size: 13px;
}

.setting-help {
  color: #8a8171;
  font-size: 12px;
  line-height: 1.5;
}

.theme-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.theme-dot {
  width: 28px;
  height: 28px;
  border: 2px solid transparent;
  border-radius: 50%;
  cursor: pointer;
}

.theme-dot.active {
  border-color: #409eff;
  box-shadow: 0 0 0 1px #409eff;
}

.theme-dot.custom-dot {
  display: grid;
  color: #fff;
  background: linear-gradient(135deg, #f4e9bd, #2d2d2d);
  font-size: 14px;
  place-items: center;
}

.font-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.font-slider {
  flex: 1;
}
</style>
