<template>
  <div class="settings-body">
    <div class="setting-row">
      <label class="setting-label">阅读模式</label>
      <el-radio-group v-model="reader.mode" size="small" @change="$emit('modeChange', $event)">
        <el-radio-button value="scroll">滚动</el-radio-button>
        <el-radio-button value="flip">左右翻页</el-radio-button>
        <el-radio-button value="page">上下分页</el-radio-button>
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
      <el-slider v-model="brightnessModel" :min="50" :max="150" size="small" @input="setBrightness" @change="setBrightness" />
    </div>

    <div class="setting-row">
      <label class="setting-label">自动阅读速度 ({{ reader.autoReadSpeed }}px)</label>
      <el-slider v-model="autoReadSpeedModel" :min="2" :max="40" :step="1" size="small" @input="setAutoReadSpeed" @change="setAutoReadSpeed" />
    </div>

    <div class="setting-row">
      <label class="setting-label">字体</label>
      <div class="font-family-grid">
        <button
          v-for="font in fontOptions"
          :key="font.value"
          class="font-family-option"
          :class="{ active: fontFamilyModel === font.value }"
          :style="{ fontFamily: font.stack }"
          type="button"
          @click="setFontFamily(font.value)"
        >
          {{ font.label }}
        </button>
      </div>
      <div class="font-preview" :style="fontPreviewStyle">
        夜色沉静，字形、字号与行距会立即应用到正文。
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">字号 ({{ reader.fontSize }}px)</label>
      <div class="font-controls">
        <el-button size="small" :icon="Minus" circle @click="changeFontSize(-1)" />
        <el-slider v-model="fontSizeModel" :min="8" :max="36" size="small" class="font-slider" @input="setFontSize" @change="setFontSize" />
        <el-button size="small" :icon="Plus" circle @click="changeFontSize(1)" />
      </div>
      <div class="font-size-presets">
        <button
          v-for="size in fontSizePresets"
          :key="size"
          class="font-size-preset"
          :class="{ active: reader.fontSize === size }"
          type="button"
          @click="setFontSize(size)"
        >
          {{ size }}
        </button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">行高 ({{ reader.lineHeight }})</label>
      <el-slider v-model="localLineHeight" :min="1" :max="5" :step="0.2" size="small" @input="setLineHeight" @change="setLineHeight" />
    </div>

    <div class="setting-row">
      <label class="setting-label">字重 ({{ reader.fontWeight }})</label>
      <el-slider v-model="fontWeightModel" :min="300" :max="900" :step="100" size="small" @input="setFontWeight" @change="setFontWeight" />
    </div>

    <div class="setting-row">
      <label class="setting-label">段落间距 ({{ reader.paragraphSpace }}em)</label>
      <el-slider v-model="paragraphSpaceModel" :min="0" :max="3" :step="0.1" size="small" @input="setParagraphSpace" @change="setParagraphSpace" />
    </div>

    <div class="setting-row">
      <label class="setting-label">阅读宽度 ({{ reader.columnWidth }}px)</label>
      <el-slider v-model="columnWidthModel" :min="560" :max="1080" :step="20" size="small" @input="setColumnWidth" @change="setColumnWidth" />
    </div>

    <div class="setting-row">
      <label class="setting-label">替换规则</label>
      <el-button size="small" plain @click="$emit('openReplaceRules')">管理全局替换规则</el-button>
      <small class="setting-help">规则会在章节加载时应用，适合清理广告、乱码和固定干扰文本。</small>
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语速 ({{ reader.ttsRate }})</label>
      <el-slider v-model="ttsRateModel" :min="0.5" :max="3" :step="0.1" size="small" @input="setTTSRate" @change="setTTSRate" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读音调 ({{ reader.ttsPitch }})</label>
      <el-slider v-model="ttsPitchModel" :min="0.5" :max="2" :step="0.1" size="small" @input="setTTSPitch" @change="setTTSPitch" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语音</label>
      <el-select
        v-model="ttsVoiceModel"
        size="small"
        clearable
        :disabled="!tts.state.supported || !ttsVoices.length"
        placeholder="浏览器默认"
        @change="setTTSVoice"
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
  'openReplaceRules',
])

const fontSizePresets = [14, 16, 18, 20, 22, 24, 28, 32]

const fontPreviewStyle = computed(() => ({
  fontFamily: props.fontOptions.find(font => font.value === props.reader.fontFamily)?.stack,
  fontSize: `${props.reader.fontSize}px`,
  fontWeight: props.reader.fontWeight,
  lineHeight: props.reader.lineHeight,
}))

const localCustomBg = computed({
  get: () => props.customBg,
  set: value => emit('update:customBg', value),
})

const localLineHeight = computed({
  get: () => props.lineHeight,
  set: value => emit('update:lineHeight', value),
})

const fontFamilyModel = computed({
  get: () => props.reader.fontFamily,
  set: value => props.reader.setFontFamily(value),
})

const fontSizeModel = computed({
  get: () => props.reader.fontSize,
  set: value => props.reader.setFontSize(value),
})

const fontWeightModel = computed({
  get: () => props.reader.fontWeight,
  set: value => props.reader.setFontWeight(value),
})

const paragraphSpaceModel = computed({
  get: () => props.reader.paragraphSpace,
  set: value => props.reader.setParagraphSpace(value),
})

const brightnessModel = computed({
  get: () => props.reader.brightness,
  set: value => props.reader.setBrightness(value),
})

const autoReadSpeedModel = computed({
  get: () => props.reader.autoReadSpeed,
  set: value => props.reader.setAutoReadSpeed(value),
})

const columnWidthModel = computed({
  get: () => props.reader.columnWidth,
  set: value => props.reader.setColumnWidth(value),
})

const ttsRateModel = computed({
  get: () => props.reader.ttsRate,
  set: value => emit('ttsRateChange', value),
})

const ttsPitchModel = computed({
  get: () => props.reader.ttsPitch,
  set: value => emit('ttsPitchChange', value),
})

const ttsVoiceModel = computed({
  get: () => props.reader.ttsVoiceURI,
  set: value => emit('ttsVoiceChange', value),
})

function setFontFamily(value) {
  props.reader.setFontFamily(value)
}

function setFontSize(value) {
  props.reader.setFontSize(value)
}

function setLineHeight(value) {
  props.reader.setLineHeight(value)
  emit('update:lineHeight', props.reader.lineHeight)
}

function setFontWeight(value) {
  props.reader.setFontWeight(value)
}

function setParagraphSpace(value) {
  props.reader.setParagraphSpace(value)
}

function changeFontSize(delta) {
  props.reader.setFontSize(props.reader.fontSize + delta)
}

function setBrightness(value) {
  props.reader.setBrightness(value)
}

function setAutoReadSpeed(value) {
  props.reader.setAutoReadSpeed(value)
}

function setColumnWidth(value) {
  props.reader.setColumnWidth(value)
}

function setTTSRate(value) {
  emit('ttsRateChange', value)
}

function setTTSPitch(value) {
  emit('ttsPitchChange', value)
}

function setTTSVoice(value) {
  emit('ttsVoiceChange', value)
}
</script>

<style scoped>
.settings-body {
  display: grid;
  gap: 20px;
  min-width: 0;
}

.setting-row {
  display: grid;
  gap: 8px;
  min-width: 0;
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
  min-width: 0;
}

.font-slider {
  min-width: 0;
  flex: 1;
}

.font-family-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.font-family-option {
  min-width: 0;
  min-height: 34px;
  padding: 0 10px;
  color: #5f564a;
  background: #fffaf0;
  border: 1px solid #eadfca;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.font-family-option.active {
  color: #0f5451;
  background: #e6f2ee;
  border-color: #2f6f6d;
  font-weight: 700;
}

.font-preview {
  min-width: 0;
  padding: 10px 12px;
  color: #2c2a24;
  background: #fffaf0;
  border: 1px solid #eadfca;
  border-radius: 6px;
  overflow-wrap: anywhere;
}

.font-size-presets {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.font-size-preset {
  min-width: 0;
  min-height: 32px;
  color: #5f564a;
  background: #fffaf0;
  border: 1px solid #eadfca;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.font-size-preset.active {
  color: #0f5451;
  background: #e6f2ee;
  border-color: #2f6f6d;
  font-weight: 700;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .settings-body {
    gap: 16px;
    padding-bottom: max(10px, env(safe-area-inset-bottom));
  }

  .setting-row {
    gap: 10px;
  }

  .theme-dot {
    width: 34px;
    height: 34px;
  }

  .font-family-option,
  .font-size-preset {
    min-height: 42px;
    font-size: 14px;
  }

  .font-controls :deep(.el-button.is-circle) {
    width: 36px;
    height: 36px;
    flex: 0 0 36px;
  }

  .font-controls :deep(.el-slider) {
    min-width: 0;
  }
}
</style>
