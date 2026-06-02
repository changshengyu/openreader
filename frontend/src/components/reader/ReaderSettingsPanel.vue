<template>
  <div class="settings-body">
    <div class="settings-title-row">
      <strong>设置</strong>
      <button type="button" @click="resetReaderSettings">重置为默认配置</button>
    </div>

    <div class="setting-row">
      <label class="setting-label">特殊模式</label>
      <el-radio-group v-model="pageTypeModel" size="small" class="read-method-group">
        <el-radio-button value="normal">正常</el-radio-button>
        <el-radio-button value="kindle">Kindle</el-radio-button>
      </el-radio-group>
      <small class="setting-help">Kindle 模式会关闭动画、切到手机模式并忽略文字选择。</small>
    </div>

    <div class="setting-row">
      <label class="setting-label">配置方案</label>
      <div class="config-scheme-list">
        <button
          v-for="(config, index) in reader.customConfigList"
          :key="config.name"
          class="config-scheme"
          :class="{ active: reader.customConfigName === config.name }"
          type="button"
          @click="selectCustomConfig(config.name)"
        >
          <span>{{ config.name }}</span>
          <small v-if="config.configDefaultType">{{ config.configDefaultType }}</small>
          <el-icon v-if="index > 1 && !config.builtin && reader.customConfigName !== config.name" @click.stop="deleteCustomConfig(config.name)"><Close /></el-icon>
        </button>
        <button class="config-scheme add" type="button" @click="addCustomConfig">新增方案</button>
        <button class="config-scheme" :class="{ active: reader.autoTheme }" type="button" @click="reader.setAutoTheme(!reader.autoTheme)">自动切换</button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">方案类型</label>
      <div class="config-scheme-list">
        <button
          v-for="type in configDefaultTypes"
          :key="type"
          class="config-scheme"
          :class="{ active: currentCustomConfig?.configDefaultType === type }"
          type="button"
          @click="setConfigDefaultType(type)"
        >
          {{ type }}
        </button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">页面模式（本机）</label>
      <el-radio-group v-model="pageModeModel" size="small" class="read-method-group">
        <el-radio-button value="auto">自适应</el-radio-button>
        <el-radio-button value="mobile">手机模式</el-radio-button>
      </el-radio-group>
    </div>

    <div class="setting-row">
      <label class="setting-label">翻页方式</label>
      <el-radio-group v-model="readerModeModel" size="small" class="read-method-group">
        <el-radio-button value="page">上下滑动</el-radio-button>
        <el-radio-button v-if="miniInterface" value="flip">左右滑动</el-radio-button>
        <el-radio-button value="scroll">上下滚动</el-radio-button>
        <el-radio-button value="scroll2">上下滚动2</el-radio-button>
      </el-radio-group>
    </div>

    <div class="setting-row">
      <label class="setting-label">全屏点击</label>
      <el-radio-group v-model="clickMethodModel" size="small" class="read-method-group">
        <el-radio-button value="next">下一页</el-radio-button>
        <el-radio-button value="auto">自动</el-radio-button>
        <el-radio-button value="none">不翻页</el-radio-button>
      </el-radio-group>
    </div>

    <div class="setting-row">
      <label class="setting-label">选择文字</label>
      <el-radio-group v-model="selectionActionModel" size="small" class="read-method-group">
        <el-radio-button value="操作弹窗">操作弹窗</el-radio-button>
        <el-radio-button value="忽略">忽略</el-radio-button>
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
        <el-color-picker v-model="localCustomBg" size="small" />
      </div>
      <div class="setting-row">
        <label class="setting-label">背景图</label>
        <div v-if="reader.customBgImageList?.length" class="bg-image-grid">
          <div
            v-for="image in reader.customBgImageList"
            :key="image"
            class="bg-image-option"
            :class="{ active: reader.customBgImage === image }"
            :style="{ backgroundImage: `url(${image})` }"
            role="button"
            tabindex="0"
            @click="toggleBgImage(image)"
            @keydown.enter.prevent="toggleBgImage(image)"
            @keydown.space.prevent="toggleBgImage(image)"
          >
            <span>{{ reader.customBgImage === image ? '使用中' : '选择' }}</span>
            <button class="bg-image-delete" type="button" title="删除背景图" @click.stop="$emit('clearBgImage', image)">
              <el-icon><Close /></el-icon>
            </button>
          </div>
        </div>
        <div class="bg-image-actions">
          <el-upload accept="image/*" :show-file-list="false" :auto-upload="false" @change="$emit('pickBgImage', $event)">
            <el-button size="small">上传</el-button>
          </el-upload>
          <el-button v-if="reader.customBgImage" size="small" text type="danger" @click="reader.setCustomBgImage('')">取消背景图</el-button>
        </div>
      </div>
    </template>

    <div class="setting-row">
      <label class="setting-label">亮度</label>
      <el-slider v-model="brightnessModel" :min="50" :max="150" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">自动阅读速度 ({{ reader.autoReadSpeed }}px)</label>
      <el-slider v-model="autoReadSpeedModel" :min="2" :max="40" :step="1" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">动画时长 ({{ reader.animateDuration }}ms)</label>
      <el-slider v-model="animateDurationModel" :min="0" :max="1000" :step="20" size="small" :disabled="reader.pageType === 'kindle'" />
      <small v-if="reader.pageType === 'kindle'" class="setting-help">Kindle 模式会关闭翻页动画。</small>
    </div>

    <div class="setting-row">
      <label class="setting-label">字体</label>
      <div class="font-family-grid">
        <div
          v-for="font in fontOptions"
          :key="font.value"
          class="font-family-option"
          :class="{ active: fontFamilyModel === font.value }"
          :style="{ fontFamily: font.stack }"
          @click="setFontFamily(font.value)"
        >
          <button class="font-family-select" type="button">
            <span>{{ font.label }}</span>
            <small v-if="hasCustomFont(font.value)">已上传</small>
          </button>
          <span class="font-family-actions" @click.stop>
            <el-upload
              accept=".ttf,.otf,.woff,.woff2"
              :show-file-list="false"
              :auto-upload="false"
              @change="file => $emit('pickFontFile', { file, font })"
            >
              <button class="font-action-btn" type="button" :title="hasCustomFont(font.value) ? '替换字体' : '上传字体'">
                <el-icon><Upload /></el-icon>
              </button>
            </el-upload>
            <button
              v-if="hasCustomFont(font.value)"
              class="font-action-btn"
              type="button"
              title="恢复默认字体"
              @click="$emit('clearFontFile', font)"
            >
              <el-icon><RefreshLeft /></el-icon>
            </button>
          </span>
        </div>
      </div>
      <div class="font-preview" :style="fontPreviewStyle">
        春风过处，纸页微明。
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">简繁转换</label>
      <el-radio-group v-model="chineseFontModel" size="small" class="read-method-group">
        <el-radio-button value="简体">简体</el-radio-button>
        <el-radio-button value="繁体">繁体</el-radio-button>
      </el-radio-group>
    </div>

    <div class="setting-row">
      <label class="setting-label">字号 ({{ reader.fontSize }}px)</label>
      <div class="font-controls">
        <el-button size="small" :icon="Minus" circle @click="changeFontSize(-1)" />
        <el-slider v-model="fontSizeModel" :min="8" :max="36" size="small" class="font-slider" />
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
      <el-slider v-model="localLineHeight" :min="1" :max="5" :step="0.2" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">字重 ({{ reader.fontWeight }})</label>
      <el-slider v-model="fontWeightModel" :min="300" :max="900" :step="100" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">段落间距 ({{ reader.paragraphSpace }}em)</label>
      <el-slider v-model="paragraphSpaceModel" :min="0" :max="3" :step="0.1" size="small" />
    </div>

    <div v-if="!miniInterface" class="setting-row">
      <label class="setting-label">阅读宽度 ({{ reader.columnWidth }}px)</label>
      <el-slider v-model="columnWidthModel" :min="560" :max="1080" :step="20" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">替换规则</label>
      <div class="operation-actions">
        <el-button size="small" plain @click="$emit('showClickZone')">显示翻页区域</el-button>
        <el-button size="small" plain @click="$emit('openReplaceRules')">管理全局替换规则</el-button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语速 ({{ reader.ttsRate }})</label>
      <el-slider v-model="ttsRateModel" :min="0.5" :max="3" :step="0.1" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读音调 ({{ reader.ttsPitch }})</label>
      <el-slider v-model="ttsPitchModel" :min="0.5" :max="2" :step="0.1" size="small" />
    </div>

    <div class="setting-row">
      <label class="setting-label">朗读语音</label>
      <el-select
        v-model="ttsVoiceModel"
        size="small"
        clearable
        :disabled="!tts.state.supported || !ttsVoices.length"
        placeholder="浏览器默认"
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, Minus, Plus, RefreshLeft, Upload } from '@element-plus/icons-vue'

const props = defineProps({
  reader: { type: Object, required: true },
  tts: { type: Object, required: true },
  ttsVoices: { type: Array, default: () => [] },
  fontOptions: { type: Array, default: () => [] },
  themePresets: { type: Object, default: () => ({}) },
  customBg: { type: String, default: '' },
  lineHeight: { type: Number, default: 2.12 },
  miniInterface: { type: Boolean, default: false },
})

const emit = defineEmits([
  'update:customBg',
  'update:lineHeight',
  'modeChange',
  'themeChange',
  'pickBgImage',
  'clearBgImage',
  'pickFontFile',
  'clearFontFile',
  'ttsRateChange',
  'ttsPitchChange',
  'ttsVoiceChange',
  'openReplaceRules',
  'showClickZone',
])

const fontSizePresets = [14, 16, 18, 20, 22, 24, 28, 32]
const configDefaultTypes = ['白天默认', '黑夜默认']

const fontPreviewStyle = computed(() => ({
  fontFamily: props.fontOptions.find(font => font.value === props.reader.fontFamily)?.stack,
  fontSize: `${props.reader.fontSize}px`,
  fontWeight: props.reader.fontWeight,
  lineHeight: props.reader.lineHeight,
}))

const currentCustomConfig = computed(() => {
  return (Array.isArray(props.reader.customConfigList) ? props.reader.customConfigList : []).find(config => config.name === props.reader.customConfigName) || null
})

const pageModeModel = computed({
  get: () => props.reader.pageMode,
  set: value => props.reader.setPageMode(value),
})

const pageTypeModel = computed({
  get: () => props.reader.pageType,
  set: value => props.reader.setPageType(value),
})

function selectCustomConfig(name) {
  if (!props.reader.setCustomConfig(name)) return
  emit('update:customBg', props.reader.customBgColor)
  emit('update:lineHeight', props.reader.lineHeight)
}

async function addCustomConfig() {
  const res = await ElMessageBox.prompt('请输入方案名称', '新增配置方案', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '方案名不能为空',
  }).catch(() => null)
  if (!res) return
  const result = props.reader.createCustomConfig(res.value)
  if (!result.ok) {
    ElMessage.error(result.message || '新增方案失败')
    return
  }
  ElMessage.success('已保存当前配置为新方案')
}

async function deleteCustomConfig(name) {
  const confirmed = await ElMessageBox.confirm(`确定删除「${name}」方案吗？`, '删除配置方案', { type: 'warning' }).catch(() => false)
  if (!confirmed) return
  const result = props.reader.deleteCustomConfig(name)
  if (!result.ok) {
    ElMessage.error(result.message || '删除方案失败')
    return
  }
  ElMessage.success('已删除配置方案')
}

async function setConfigDefaultType(type) {
  const confirmed = await ElMessageBox.confirm(`确认把「${props.reader.customConfigName}」设为${type}吗？`, '设置方案类型', { type: 'warning' }).catch(() => false)
  if (!confirmed) return
  const result = props.reader.setCustomConfigDefaultType(type)
  if (!result.ok) {
    ElMessage.error(result.message || '设置方案类型失败')
    return
  }
  ElMessage.success(`已设为${type}`)
}

const readerModeModel = computed({
  get: () => props.reader.mode,
  set: value => emit('modeChange', value),
})

const clickMethodModel = computed({
  get: () => props.reader.clickMethod,
  set: value => props.reader.setClickMethod(value),
})

const selectionActionModel = computed({
  get: () => props.reader.selectionAction,
  set: value => props.reader.setSelectionAction(value),
})

const localCustomBg = computed({
  get: () => props.customBg,
  set: value => {
    props.reader.setCustomBgColor(value)
    emit('update:customBg', props.reader.customBgColor)
  },
})

const localLineHeight = computed({
  get: () => props.lineHeight,
  set: value => {
    props.reader.setLineHeight(value)
    emit('update:lineHeight', props.reader.lineHeight)
  },
})

const fontFamilyModel = computed({
  get: () => props.reader.fontFamily,
  set: value => props.reader.setFontFamily(value),
})

const chineseFontModel = computed({
  get: () => props.reader.chineseFont,
  set: value => props.reader.setChineseFont(value),
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

const animateDurationModel = computed({
  get: () => props.reader.animateDuration,
  set: value => props.reader.setAnimateDuration(value),
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

function changeFontSize(delta) {
  props.reader.setFontSize(props.reader.fontSize + delta)
}

function toggleBgImage(image) {
  props.reader.setCustomBgImage(props.reader.customBgImage === image ? '' : image)
}

function hasCustomFont(value) {
  return Boolean(props.reader.customFontsMap?.[value])
}

function resetReaderSettings() {
  props.reader.resetReaderSettings()
  emit('update:customBg', props.reader.customBgColor)
  emit('update:lineHeight', props.reader.lineHeight)
}

</script>

<style scoped>
.settings-body {
  display: grid;
  gap: 20px;
  min-width: 0;
}

.settings-title-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.settings-title-row strong {
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
  font-weight: 600;
}

.settings-title-row button {
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 13px;
}

.read-method-group {
  display: flex;
  flex-wrap: wrap;
}

.config-scheme-list {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.config-scheme {
  display: inline-flex;
  min-width: 0;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(111, 94, 54, 0.2);
  border-radius: 6px;
  padding: 6px 10px;
  background: rgba(255, 255, 255, 0.5);
  color: inherit;
  cursor: pointer;
}

.config-scheme span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.config-scheme small {
  color: rgba(31, 41, 55, 0.55);
  white-space: nowrap;
}

.config-scheme.active {
  border-color: #ed4259;
  color: #ed4259;
  background: rgba(237, 66, 89, 0.08);
}

.config-scheme.add {
  color: #ed4259;
}

.operation-actions {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
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

.bg-image-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.bg-image-option {
  position: relative;
  min-width: 0;
  aspect-ratio: 4 / 3;
  color: #fff;
  background-color: #eadfca;
  background-position: center;
  background-size: cover;
  border: 2px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  overflow: hidden;
}

.bg-image-option::before {
  position: absolute;
  inset: 0;
  content: "";
  background: linear-gradient(to top, rgba(0,0,0,0.52), rgba(0,0,0,0.05));
}

.bg-image-option.active {
  border-color: #409eff;
}

.bg-image-option > span {
  position: absolute;
  left: 8px;
  bottom: 6px;
  z-index: 1;
  font-size: 12px;
  font-weight: 700;
}

.bg-image-delete {
  position: absolute;
  top: 4px;
  right: 4px;
  z-index: 1;
  width: 24px;
  height: 24px;
  color: #fff;
  background: rgba(0,0,0,0.42);
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  display: grid;
  place-items: center;
}

.bg-image-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
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
  min-height: 40px;
  padding: 0 8px 0 10px;
  color: #5f564a;
  background: #fffaf0;
  border: 1px solid #eadfca;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 6px;
  justify-content: space-between;
}

.font-family-option.active {
  color: #0f5451;
  background: #e6f2ee;
  border-color: #2f6f6d;
  font-weight: 700;
}

.font-family-select {
  min-width: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  display: grid;
  gap: 1px;
  padding: 0;
  text-align: left;
}

.font-family-select span,
.font-family-select small {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.font-family-select small {
  color: #409eff;
  font-size: 11px;
  font-weight: 500;
}

.font-family-actions {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.font-action-btn {
  width: 24px;
  height: 24px;
  padding: 0;
  color: #7b705f;
  background: transparent;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  display: grid;
  place-items: center;
}

.font-action-btn:hover {
  color: #0f5451;
  background: rgba(47, 111, 109, 0.1);
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

@media (max-width: 750px) {
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
