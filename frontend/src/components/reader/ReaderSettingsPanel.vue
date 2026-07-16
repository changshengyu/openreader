<template>
  <div class="settings-body">
    <div class="settings-title">
      设置
      <button type="button" @click="resetReaderSettings">重置为默认配置</button>
    </div>

    <div class="settings-list">
    <div class="setting-row">
      <label class="setting-label">特殊模式</label>
      <div class="selection-zone">
        <button
          v-for="option in pageTypeOptions"
          :key="option.value"
          class="selection-button"
          :class="{ active: pageTypeModel === option.value }"
          type="button"
          @click="pageTypeModel = option.value"
        >
          {{ option.label }}
        </button>
      </div>
      <small class="setting-help">开启简洁模式会关闭动画以及首页的部分功能。</small>
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
      <label class="setting-label">阅读主题</label>
      <div class="selection-zone theme-grid">
        <span
          v-for="(preset, key) in visibleThemePresets"
          :key="key"
          class="theme-item"
          :class="{ active: reader.theme === key }"
          :style="{ background: preset.bg }"
          :title="preset.label"
          @click="$emit('themeChange', key)"
        >
          <em v-if="key === 'dark'" class="moon-icon">◐</em>
          <em v-else class="theme-check">✓</em>
        </span>
        <button
          class="selection-button theme-custom-button"
          :class="{ active: reader.theme === 'custom' }"
          type="button"
          @click="$emit('themeChange', 'custom')"
        >自定义</button>
      </div>
    </div>

    <template v-if="reader.theme === 'custom'">
      <div class="setting-row">
        <label class="setting-label">自定义</label>
        <div class="custom-theme">
          <span class="custom-theme-title custom-theme-mode">主题模式
            <button
              v-for="option in themeTypeOptions"
              :key="option.value"
              class="selection-button"
              :class="{ active: themeTypeModel === option.value }"
              type="button"
              @click="themeTypeModel = option.value"
            >
              {{ option.label }}
            </button>
          </span>
          <span class="custom-theme-title">页面背景颜色
            <el-color-picker v-model="bodyColorModel" size="small" />
          </span>
          <span class="custom-theme-title">浮窗背景颜色
            <el-color-picker v-model="popupColorModel" size="small" />
          </span>
          <span class="custom-theme-title">阅读背景颜色
            <el-color-picker v-model="localCustomBg" size="small" />
          </span>
          <span class="custom-theme-title bg-image-title">阅读背景图片
            <span v-if="reader.customBgImageList?.length" class="content-bg-preview-list">
              <span
                v-for="image in reader.customBgImageList"
                :key="image"
                class="content-bg-preview"
                :class="{ selected: reader.customBgImage === image }"
                role="button"
                tabindex="0"
                @click="toggleBgImage(image)"
                @keydown.enter.prevent="toggleBgImage(image)"
                @keydown.space.prevent="toggleBgImage(image)"
              >
                <img :src="image" alt="" />
                <button class="delete-bg-icon" type="button" title="删除背景图" @click.stop="$emit('clearBgImage', image)">
                  <el-icon><Close /></el-icon>
                </button>
              </span>
            </span>
            <el-upload class="upload-bg-upload" accept="image/*" :show-file-list="false" :auto-upload="false" @change="$emit('pickBgImage', $event)">
              <span class="upload-bg-btn">上传</span>
            </el-upload>
          </span>
        </div>
      </div>
    </template>

    <div class="setting-row stepper-setting-row">
      <label class="setting-label">亮度</label>
      <ReaderSettingStepper
        v-model="brightnessModel"
        :min="50"
        :max="150"
        :step="5"
        decrease-label="降低亮度"
        increase-label="提高亮度"
      />
    </div>

    <div class="setting-row">
      <label class="setting-label">正文字体</label>
      <div class="selection-zone font-family-grid">
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
          </button>
          <span class="font-family-actions" @click.stop>
            <el-upload
              accept=".ttf,.otf,.woff,.woff2"
              :show-file-list="false"
              :auto-upload="false"
              @change="file => $emit('pickFontFile', { file, font })"
            >
              <button
                class="font-action-btn"
                :class="{ active: hasCustomFont(font.value) }"
                type="button"
                :title="hasCustomFont(font.value) ? '替换字体' : '上传字体'"
              >
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
      <div class="selection-zone">
        <button
          v-for="option in chineseFontOptions"
          :key="option"
          class="selection-button"
          :class="{ active: chineseFontModel === option }"
          type="button"
          @click="chineseFontModel = option"
        >
          {{ option }}
        </button>
      </div>
    </div>

    <div class="setting-row typography-setting-row">
      <label class="setting-label">字体大小</label>
      <ReaderSettingStepper
        v-model="fontSizeModel"
        :min="8"
        :max="36"
        :step="1"
        decrease-label="减小字号"
        increase-label="增大字号"
      />
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

    <div class="setting-row typography-setting-row">
      <label class="setting-label">字体粗细</label>
      <ReaderSettingStepper
        v-model="fontWeightModel"
        :min="100"
        :max="900"
        :step="100"
        decrease-label="减小字重"
        increase-label="增大字重"
      />
    </div>

    <div class="setting-row typography-setting-row">
      <label class="setting-label">段落行高</label>
      <ReaderSettingStepper
        v-model="localLineHeight"
        :min="1"
        :max="5"
        :step="0.2"
        decrease-label="减小行高"
        increase-label="增大行高"
      />
    </div>

    <div class="setting-row typography-setting-row">
      <label class="setting-label">段落间距</label>
      <ReaderSettingStepper
        v-model="paragraphSpaceModel"
        :min="0"
        :max="5"
        :step="0.2"
        decrease-label="减小段落间距"
        increase-label="增大段落间距"
      />
    </div>

    <div class="setting-row">
      <label class="setting-label">字体颜色</label>
      <div class="color-setting">
        <el-color-picker v-model="fontColorModel" size="small" />
        <el-button v-if="reader.fontColor" size="small" text type="danger" @click="reader.setFontColor('')">恢复默认</el-button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">页面模式（本机）</label>
      <div class="selection-zone">
        <button
          v-for="option in pageModeOptions"
          :key="option.value"
          class="selection-button"
          :class="{ active: pageModeModel === option.value }"
          type="button"
          @click="pageModeModel = option.value"
        >
          {{ option.label }}
        </button>
      </div>
    </div>

    <div v-if="!miniInterface" class="setting-row stepper-setting-row">
      <label class="setting-label">页面宽度</label>
      <ReaderSettingStepper
        v-model="columnWidthModel"
        :min="480"
        :max="1120"
        :step="160"
        decrease-label="缩小页面宽度"
        increase-label="增大页面宽度"
      />
    </div>

    <div class="setting-row">
      <label class="setting-label">翻页方式</label>
      <div class="selection-zone">
        <button
          v-for="option in visibleReaderModeOptions"
          :key="option.value"
          class="selection-button"
          :class="{ active: readerModeModel === option.value }"
          type="button"
          @click="readerModeModel = option.value"
        >
          {{ option.label }}
        </button>
      </div>
      <small class="setting-help">上下滚动2会自动隐藏看过的章节，但是可能会抖动。</small>
    </div>

    <div class="setting-row stepper-setting-row">
      <label class="setting-label">动画时长</label>
      <ReaderSettingStepper
        v-model="animateDurationModel"
        :min="0"
        :max="500"
        :step="50"
        :disabled="reader.pageType === 'kindle'"
        decrease-label="缩短动画"
        increase-label="延长动画"
      />
      <small v-if="reader.pageType === 'kindle'" class="setting-help">简洁模式会关闭翻页动画。</small>
    </div>

    <div class="setting-row">
      <label class="setting-label">自动阅读</label>
      <div class="selection-zone">
        <button
          v-for="option in autoReadingMethodOptions"
          :key="option"
          class="selection-button"
          :class="{ active: autoReadingMethodModel === option }"
          type="button"
          @click="autoReadingMethodModel = option"
        >
          {{ option }}
        </button>
      </div>
    </div>

    <div v-if="reader.autoReadingMethod === '像素滚动'" class="setting-row stepper-setting-row">
      <label class="setting-label">滚动像素</label>
      <ReaderSettingStepper
        v-model="autoReadingPixelModel"
        :min="1"
        :max="80"
        :step="5"
        decrease-label="减少滚动像素"
        increase-label="增加滚动像素"
      />
    </div>

    <div class="setting-row stepper-setting-row">
      <label class="setting-label">翻页速度</label>
      <ReaderSettingStepper
        v-model="autoReadingLineTimeModel"
        :min="10"
        :max="3000"
        :step="50"
        decrease-label="加快翻页"
        increase-label="减慢翻页"
      />
    </div>

    <div class="setting-row">
      <label class="setting-label">全屏点击</label>
      <div class="selection-zone">
        <button
          v-for="option in clickMethodOptions"
          :key="option.value"
          class="selection-button"
          :class="{ active: clickMethodModel === option.value }"
          type="button"
          @click="clickMethodModel = option.value"
        >
          {{ option.label }}
        </button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">选择文字</label>
      <div class="selection-zone">
        <button
          v-for="option in selectionActionOptions"
          :key="option"
          class="selection-button"
          :class="{ active: selectionActionModel === option }"
          type="button"
          @click="selectionActionModel = option"
        >
          {{ option }}
        </button>
      </div>
    </div>

    <div class="setting-row">
      <label class="setting-label">替换规则</label>
      <div class="operation-actions">
        <el-button size="small" plain @click="$emit('showClickZone')">显示翻页区域</el-button>
        <el-button size="small" plain @click="$emit('openReplaceRules')">管理全局替换规则</el-button>
      </div>
    </div>

    <div class="setting-row stepper-setting-row">
      <label class="setting-label">朗读语速</label>
      <ReaderSettingStepper
        v-model="ttsRateModel"
        :min="0.5"
        :max="2"
        :step="0.1"
        decrease-label="降低朗读语速"
        increase-label="提高朗读语速"
      />
    </div>

    <div class="setting-row stepper-setting-row">
      <label class="setting-label">朗读音调</label>
      <ReaderSettingStepper
        v-model="ttsPitchModel"
        :min="0"
        :max="2"
        :step="0.1"
        decrease-label="降低朗读音调"
        increase-label="提高朗读音调"
      />
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
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, RefreshLeft, Upload } from '@element-plus/icons-vue'
import ReaderSettingStepper from './ReaderSettingStepper.vue'

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
const themeTypeOptions = [
  { value: 'day', label: '白天' },
  { value: 'night', label: '黑夜' },
]
const pageTypeOptions = [
  { value: 'normal', label: '正常' },
  { value: 'kindle', label: '简洁' },
]
const chineseFontOptions = ['简体', '繁体']
const pageModeOptions = [
  { value: 'auto', label: '自适应' },
  { value: 'mobile', label: '手机模式' },
]
const readerModeOptions = [
  { value: 'page', label: '上下滑动' },
  { value: 'flip', label: '左右滑动', mobileOnly: true },
  { value: 'scroll', label: '上下滚动' },
  { value: 'scroll2', label: '上下滚动2' },
]
const autoReadingMethodOptions = ['像素滚动', '段落滚动']
const clickMethodOptions = [
  { value: 'next', label: '下一页' },
  { value: 'auto', label: '自动' },
  { value: 'none', label: '不翻页' },
]
const selectionActionOptions = ['操作弹窗', '忽略']

const fontPreviewStyle = computed(() => ({
  fontFamily: props.fontOptions.find(font => font.value === props.reader.fontFamily)?.stack,
  fontSize: `${props.reader.fontSize}px`,
  fontWeight: props.reader.fontWeight,
  lineHeight: props.reader.lineHeight,
}))

const currentCustomConfig = computed(() => {
  return (Array.isArray(props.reader.customConfigList) ? props.reader.customConfigList : []).find(config => config.name === props.reader.customConfigName) || null
})
const visibleThemePresets = computed(() => Object.fromEntries(
  Object.entries(props.themePresets).filter(([key]) => key !== 'black'),
))
const visibleReaderModeOptions = computed(() => (
  readerModeOptions.filter(option => props.miniInterface || !option.mobileOnly)
))

const pageModeModel = computed({
  get: () => props.reader.pageMode,
  set: value => props.reader.setPageMode(value),
})

const pageTypeModel = computed({
  get: () => props.reader.pageType,
  set: value => props.reader.setPageType(value),
})

const themeTypeModel = computed({
  get: () => props.reader.themeType,
  set: value => props.reader.setThemeType(value),
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

const fontColorModel = computed({
  get: () => props.reader.fontColor,
  set: value => props.reader.setFontColor(value || ''),
})

const bodyColorModel = computed({
  get: () => props.reader.customBodyColor,
  set: value => props.reader.setCustomBodyColor(value || ''),
})

const popupColorModel = computed({
  get: () => props.reader.customPopupColor,
  set: value => props.reader.setCustomPopupColor(value || ''),
})

const paragraphSpaceModel = computed({
  get: () => props.reader.paragraphSpace,
  set: value => props.reader.setParagraphSpace(value),
})

const brightnessModel = computed({
  get: () => props.reader.brightness,
  set: value => props.reader.setBrightness(value),
})

const autoReadingMethodModel = computed({
  get: () => props.reader.autoReadingMethod,
  set: value => props.reader.setAutoReadingMethod(value),
})

const autoReadingPixelModel = computed({
  get: () => props.reader.autoReadingPixel,
  set: value => props.reader.setAutoReadingPixel(value),
})

const autoReadingLineTimeModel = computed({
  get: () => props.reader.autoReadingLineTime,
  set: value => props.reader.setAutoReadingLineTime(value),
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
  min-width: 0;
  text-align: left;
  user-select: none;
}

.settings-title {
  min-width: 0;
  font-size: 18px;
  line-height: 22px;
  margin-bottom: 28px;
  font-family: -apple-system, "Noto Sans", "Helvetica Neue", Helvetica, "Nimbus Sans L", Arial, "Liberation Sans", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", "Source Han Sans SC", "Source Han Sans CN", "Microsoft YaHei", "Wenquanyi Micro Hei", "WenQuanYi Zen Hei", "ST Heiti", SimHei, sans-serif;
  font-weight: 400;
}

.settings-title button {
  float: right;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}

.settings-list {
  display: grid;
  gap: 20px;
  min-width: 0;
  max-height: 45vh;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
}

.settings-list::-webkit-scrollbar {
  display: none;
}

.selection-zone {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.selection-button {
  min-width: 78px;
  height: 34px;
  padding: 0 12px;
  color: inherit;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 2px;
  cursor: pointer;
  font: 14px / 34px PingFangSC-Regular, HelveticaNeue-Light, "Helvetica Neue Light", "Microsoft YaHei", sans-serif;
  text-align: center;
  white-space: nowrap;
}

.selection-button.active {
  color: #ed4259;
  border-color: #ed4259;
}

@media (hover: hover) {
  .selection-button:hover {
    color: #ed4259;
    border-color: #ed4259;
  }
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

.color-setting {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
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
  gap: 5px 16px;
}

.theme-item {
  width: 34px;
  height: 34px;
  color: #ed4259;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 100%;
  cursor: pointer;
  display: inline-block;
  line-height: 32px;
  text-align: center;
  vertical-align: middle;
}

.theme-item.active {
  border-color: #ed4259;
}

.theme-check {
  display: none;
  font-style: normal;
}

.theme-item.active .theme-check {
  display: inline;
}

.moon-icon {
  color: rgba(255, 255, 255, 0.2);
  font-style: normal;
}

.theme-item.active .moon-icon {
  color: #ed4259;
}

.theme-custom-button {
  min-width: 78px;
}

.custom-theme {
  display: inline-block;
  min-width: 0;
  width: 100%;
  word-wrap: break-word;
}

.custom-theme-title {
  display: inline-block;
  margin-right: 28px;
  margin-bottom: 5px;
}

.custom-theme-mode .selection-button {
  margin-left: 8px;
}

.bg-image-title {
  display: inline-flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 0;
}

.content-bg-preview-list {
  display: inline;
}

.content-bg-preview {
  position: relative;
  width: 36px;
  height: 36px;
  margin-left: 10px;
  margin-bottom: 8px;
  border: 1px solid transparent;
  box-sizing: border-box;
  cursor: pointer;
  display: inline-block;
  vertical-align: middle;
}

.content-bg-preview img {
  width: 100%;
  height: 100%;
  display: inline-block;
  object-fit: cover;
  vertical-align: middle;
}

.content-bg-preview.selected {
  color: #ed4259;
  border-color: #ed4259;
}

.delete-bg-icon {
  position: absolute;
  top: -6px;
  right: -6px;
  z-index: 10;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  display: inline-flex;
  font-size: 18px;
  line-height: 1;
}

.upload-bg-upload {
  display: inline-block;
}

.upload-bg-btn {
  display: inline-block;
  margin-left: 10px;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.upload-bg-upload :deep(.upload-bg-btn) {
  display: inline-block;
  margin-left: 10px;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
}

:global(.upload-bg-btn) {
  display: inline-block !important;
}

.font-family-grid {
  gap: 10px 16px;
}

.font-family-option {
  position: relative;
  width: 78px;
  height: 34px;
  padding: 0;
  color: #5f564a;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 2px;
  cursor: pointer;
  font: 14px / 34px PingFangSC-Regular, HelveticaNeue-Light, "Helvetica Neue Light", "Microsoft YaHei", sans-serif;
  display: inline-block;
  text-align: center;
  vertical-align: middle;
}

.font-family-option.active {
  color: #ed4259;
  border-color: #ed4259;
}

.font-family-select {
  width: 100%;
  height: 100%;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  display: block;
  padding: 0;
  font: inherit;
  line-height: inherit;
  text-align: center;
}

.font-family-select span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.font-family-actions {
  position: absolute;
  top: -10px;
  right: -10px;
  z-index: 10;
  display: inline-flex;
  align-items: center;
  gap: 0;
}

.font-action-btn {
  width: 20px;
  height: 20px;
  padding: 0;
  color: #606266;
  background: transparent;
  border: 0;
  cursor: pointer;
  display: grid;
  place-items: center;
  font-size: 20px;
}

.font-action-btn.active,
.font-action-btn:hover {
  color: #ed4259;
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

.typography-setting-row,
.stepper-setting-row {
  grid-template-columns: 62px minmax(0, 220px);
  align-items: center;
  column-gap: 10px;
}

.typography-setting-row .font-size-presets {
  grid-column: 2;
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
  color: #ed4259;
  background: rgba(237, 66, 89, 0.08);
  border-color: #ed4259;
  font-weight: 700;
}

@media (min-width: 751px) {
  .settings-list {
    gap: 18px;
  }

  .setting-row {
    grid-template-columns: 62px minmax(0, 1fr);
    align-items: start;
    column-gap: 12px;
  }

  .setting-row > .setting-label {
    grid-column: 1;
    line-height: 36px;
  }

  .setting-row > :not(.setting-label) {
    grid-column: 2;
  }

  .typography-setting-row,
  .stepper-setting-row {
    grid-template-columns: 62px minmax(0, 220px);
    align-items: center;
    column-gap: 12px;
  }

  .font-family-grid {
    max-width: 480px;
  }

  .font-preview {
    max-width: 456px;
  }

  .font-size-presets {
    max-width: 456px;
  }
}

@media (max-width: 750px) {
  .settings-list {
    gap: 20px;
  }

  .setting-row {
    grid-template-columns: 72px minmax(0, 1fr);
    align-items: start;
    gap: 8px 0;
  }

  .setting-row > .setting-label {
    grid-column: 1;
    line-height: 36px;
  }

  .setting-row > :not(.setting-label) {
    grid-column: 2;
  }

  .typography-setting-row,
  .stepper-setting-row {
    grid-template-columns: 72px minmax(0, 1fr);
  }
}
</style>
