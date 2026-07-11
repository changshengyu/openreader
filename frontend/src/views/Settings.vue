<template>
  <section class="app-page settings-page">
    <header class="settings-head">
      <div>
        <h1 class="app-page-title">设置</h1>
      </div>
      <el-button :icon="Refresh" :loading="checking" @click="checkHealth">检查服务</el-button>
    </header>

    <el-tabs v-model="activeTab" class="settings-tabs">
      <el-tab-pane label="账户" name="account">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><User /></el-icon>
              <h2>账户</h2>
            </div>
            <dl class="info-list">
              <div><dt>用户名</dt><dd>{{ userStore.profile?.username || '-' }}</dd></div>
              <div><dt>角色</dt><dd>{{ userStore.profile?.role || '-' }}</dd></div>
              <div><dt>书籍限制</dt><dd>{{ limitText(userStore.profile?.bookLimit) }}</dd></div>
              <div><dt>书源限制</dt><dd>{{ limitText(userStore.profile?.sourceLimit) }}</dd></div>
            </dl>
            <el-button type="primary" plain :icon="SwitchButton" @click="logout">退出登录</el-button>
          </article>

          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Connection /></el-icon>
              <h2>同步</h2>
            </div>
            <p class="panel-text">阅读进度和书架变更通过 WebSocket 推送。当前连接状态：</p>
            <el-tag :type="syncConnected ? 'success' : 'info'" effect="plain">
              {{ syncConnected ? '同步在线' : '等待连接' }}
            </el-tag>
            <dl v-if="healthInfo" class="info-list service-info">
              <div><dt>构建时间</dt><dd>{{ healthInfo.buildDate || '-' }}</dd></div>
              <div><dt>提交版本</dt><dd>{{ shortCommit(healthInfo.commit) }}</dd></div>
            </dl>
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="缓存" name="cache">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Files /></el-icon>
              <h2>远程章节缓存</h2>
            </div>
            <dl class="info-list">
              <div><dt>缓存范围</dt><dd>当前账号</dd></div>
              <div><dt>缓存文件</dt><dd>{{ cacheStats.files || 0 }}</dd></div>
              <div><dt>缓存大小</dt><dd>{{ formatSize(cacheStats.size || 0) }}</dd></div>
              <div><dt>章节状态</dt><dd>{{ cacheStats.cachedChapters || 0 }} 章已缓存</dd></div>
            </dl>
            <div class="panel-actions">
              <el-button :icon="Refresh" :loading="cacheLoading" @click="loadCacheStats">刷新</el-button>
              <el-button type="danger" plain :icon="Delete" :loading="cacheClearing" @click="clearSystemCache">清理缓存</el-button>
            </div>
          </article>
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Files /></el-icon>
              <h2>浏览器本地缓存</h2>
            </div>
            <dl class="info-list">
              <div><dt>缓存位置</dt><dd>当前浏览器</dd></div>
              <div><dt>缓存文件</dt><dd>{{ localBrowserCacheStats.total?.files || 0 }}</dd></div>
              <div><dt>缓存大小</dt><dd>{{ formatSize(localBrowserCacheStats.total?.size || 0) }}</dd></div>
              <div><dt>书源缓存</dt><dd>{{ cacheGroupText('bookSourceList') }}</dd></div>
              <div><dt>RSS源缓存</dt><dd>{{ cacheGroupText('rssSources') }}</dd></div>
              <div><dt>章节列表缓存</dt><dd>{{ cacheGroupText('chapterList') }}</dd></div>
              <div><dt>章节内容缓存</dt><dd>{{ cacheGroupText('chapterContent') }}</dd></div>
            </dl>
            <div class="panel-actions cache-action-grid">
              <el-button :icon="Refresh" :loading="cacheLoading" @click="loadCacheStats">刷新</el-button>
              <el-button
                type="danger"
                plain
                :icon="Delete"
                :loading="browserCacheClearing === 'bookSourceList'"
                :disabled="!cacheGroupFiles('bookSourceList')"
                @click="clearBrowserLocalCache('bookSourceList')"
              >清空书源缓存</el-button>
              <el-button
                type="danger"
                plain
                :icon="Delete"
                :loading="browserCacheClearing === 'rssSources'"
                :disabled="!cacheGroupFiles('rssSources')"
                @click="clearBrowserLocalCache('rssSources')"
              >清空 RSS 源缓存</el-button>
              <el-button
                type="danger"
                plain
                :icon="Delete"
                :loading="browserCacheClearing === 'chapterList'"
                :disabled="!cacheGroupFiles('chapterList')"
                @click="clearBrowserLocalCache('chapterList')"
              >清空章节列表缓存</el-button>
              <el-button
                type="danger"
                plain
                :icon="Delete"
                :loading="browserCacheClearing === 'chapterContent'"
                :disabled="!cacheGroupFiles('chapterContent')"
                @click="clearBrowserLocalCache('chapterContent')"
              >清空章节内容缓存</el-button>
            </div>
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="阅读" name="reader">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><View /></el-icon>
              <h2>阅读默认值</h2>
              <span class="reader-sync-state" :class="{ error: readerStore.settingsSyncError }">{{ readerSettingsSyncText }}</span>
            </div>
            <div class="reader-setting-list">
              <label>
                <span>特殊模式</span>
                <el-radio-group v-model="readerPageTypeModel" size="small">
                  <el-radio-button value="normal">正常</el-radio-button>
                  <el-radio-button value="kindle">简洁</el-radio-button>
                </el-radio-group>
              </label>
              <label class="reader-config-schemes-row">
                <span>配置方案</span>
                <div class="reader-config-schemes">
                  <button
                    v-for="(config, index) in readerStore.customConfigList"
                    :key="config.name"
                    class="reader-config-scheme"
                    :class="{ active: readerStore.customConfigName === config.name }"
                    type="button"
                    @click="selectReaderCustomConfig(config.name)"
                  >
                    <span>{{ config.name }}</span>
                    <small v-if="config.configDefaultType">{{ config.configDefaultType }}</small>
                    <el-icon v-if="index > 1 && !config.builtin && readerStore.customConfigName !== config.name" @click.stop="deleteReaderCustomConfig(config.name)"><Delete /></el-icon>
                  </button>
                  <button class="reader-config-scheme add" type="button" @click="addReaderCustomConfig">新增方案</button>
                  <button class="reader-config-scheme" :class="{ active: readerStore.autoTheme }" type="button" @click="readerStore.setAutoTheme(!readerStore.autoTheme)">自动切换</button>
                </div>
              </label>
              <label class="reader-config-schemes-row">
                <span>方案类型</span>
                <div class="reader-config-schemes">
                  <button
                    v-for="type in configDefaultTypes"
                    :key="type"
                    class="reader-config-scheme"
                    :class="{ active: currentReaderCustomConfig?.configDefaultType === type }"
                    type="button"
                    @click="setReaderConfigDefaultType(type)"
                  >
                    {{ type }}
                  </button>
                </div>
              </label>
              <div class="reader-theme-row">
                <span>阅读主题</span>
                <div class="theme-list">
                  <button
                    v-for="(theme, key) in themePresets"
                    :key="key"
                    type="button"
                    class="theme-choice"
                    :class="{ active: readerStore.theme === key }"
                    @click="readerStore.setTheme(key)"
                  >
                    <span class="theme-swatch" :style="{ background: theme.bg }" />
                    <span>{{ theme.label }}</span>
                  </button>
                  <button type="button" class="theme-choice" :class="{ active: readerStore.theme === 'custom' }" @click="readerStore.setTheme('custom')">
                    <span class="theme-swatch custom-swatch" :style="{ background: readerStore.customBgColor || '#f4e9bd' }" />
                    <span>自定义</span>
                  </button>
                </div>
              </div>
              <div v-if="readerStore.theme === 'custom'" class="reader-theme-row">
                <span>自定义</span>
                <div class="custom-theme-row">
                  <div class="custom-theme-field">
                    <span>页面背景颜色</span>
                    <el-color-picker v-model="readerCustomBodyColorModel" />
                    <el-button v-if="readerStore.customBodyColor" size="small" text type="danger" @click="readerStore.setCustomBodyColor('')">恢复默认</el-button>
                  </div>
                  <div class="custom-theme-field">
                    <span>浮窗背景颜色</span>
                    <el-color-picker v-model="readerCustomPopupColorModel" />
                    <el-button v-if="readerStore.customPopupColor" size="small" text type="danger" @click="readerStore.setCustomPopupColor('')">恢复默认</el-button>
                  </div>
                  <div class="custom-theme-field">
                    <span>阅读背景颜色</span>
                    <el-color-picker v-model="readerCustomBgColorModel" />
                  </div>
                  <el-upload accept="image/*" :show-file-list="false" :auto-upload="false" @change="pickReaderBgImage">
                    <el-button size="small" :icon="Upload" :loading="readerBgUploading">背景图</el-button>
                  </el-upload>
                  <el-button v-if="readerStore.customBgImage" size="small" text type="danger" @click="readerStore.setCustomBgImage('')">取消背景图</el-button>
                  <div v-if="readerStore.customBgImageList?.length" class="settings-bg-list">
                    <div
                      v-for="image in readerStore.customBgImageList"
                      :key="image"
                      class="settings-bg-choice"
                      :class="{ active: readerStore.customBgImage === image }"
                      :style="{ backgroundImage: `url(${image})` }"
                      role="button"
                      tabindex="0"
                      @click="readerStore.setCustomBgImage(readerStore.customBgImage === image ? '' : image)"
                      @keydown.enter.prevent="readerStore.setCustomBgImage(readerStore.customBgImage === image ? '' : image)"
                      @keydown.space.prevent="readerStore.setCustomBgImage(readerStore.customBgImage === image ? '' : image)"
                    >
                      <span>{{ readerStore.customBgImage === image ? '使用中' : '选择' }}</span>
                      <button type="button" @click.stop="deleteReaderBgImage(image)">删除</button>
                    </div>
                  </div>
                </div>
              </div>
              <label>
                <span>字体</span>
                <el-select v-model="readerFontFamilyModel" size="small">
                  <el-option v-for="font in fontOptions" :key="font.value" :label="font.label" :value="font.value" />
                </el-select>
              </label>
              <label>
                <span>简繁转换</span>
                <el-radio-group v-model="readerChineseFontModel" size="small">
                  <el-radio-button value="简体">简体</el-radio-button>
                  <el-radio-button value="繁体">繁体</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>亮度 {{ readerStore.brightness }}%</span>
                <el-slider v-model="readerBrightnessModel" :min="50" :max="150" />
              </label>
              <label>
                <span>字号 {{ readerStore.fontSize }}px</span>
                <el-slider v-model="readerFontSizeModel" :min="8" :max="36" />
              </label>
              <label>
                <span>字重 {{ readerStore.fontWeight }}</span>
                <el-slider v-model="readerFontWeightModel" :min="100" :max="900" :step="100" />
              </label>
              <label>
                <span>行高 {{ readerStore.lineHeight }}</span>
                <el-slider v-model="readerLineHeightModel" :min="1" :max="5" :step="0.2" />
              </label>
              <label>
                <span>段落间距 {{ readerStore.paragraphSpace }}em</span>
                <el-slider v-model="readerParagraphSpaceModel" :min="0" :max="5" :step="0.2" />
              </label>
              <label>
                <span>阅读宽度 {{ readerStore.columnWidth }}px</span>
                <el-slider v-model="readerColumnWidthModel" :min="480" :max="1120" :step="160" />
              </label>
              <label>
                <span>页面模式（本机）</span>
                <el-radio-group v-model="readerPageModeModel" size="small">
                  <el-radio-button value="auto">自适应</el-radio-button>
                  <el-radio-button value="mobile">手机模式</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>翻页方式</span>
                <el-radio-group v-model="readerModeModel" size="small">
                  <el-radio-button value="page">上下滑动</el-radio-button>
                  <el-radio-button v-if="readerSettingsMiniInterface" value="flip">左右滑动</el-radio-button>
                  <el-radio-button value="scroll">上下滚动</el-radio-button>
                  <el-radio-button value="scroll2">上下滚动2</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>动画时长 {{ readerStore.animateDuration }}ms</span>
                <el-slider v-model="readerAnimateDurationModel" :min="0" :max="500" :step="50" :disabled="readerStore.pageType === 'kindle'" />
              </label>
              <label>
                <span>自动阅读</span>
                <el-radio-group v-model="readerAutoReadingMethodModel" size="small">
                  <el-radio-button value="像素滚动">像素滚动</el-radio-button>
                  <el-radio-button value="段落滚动">段落滚动</el-radio-button>
                </el-radio-group>
              </label>
              <label v-if="readerStore.autoReadingMethod === '像素滚动'">
                <span>滚动像素 {{ readerStore.autoReadingPixel }}px</span>
                <el-slider v-model="readerAutoReadingPixelModel" :min="1" :max="80" :step="1" />
              </label>
              <label>
                <span>翻页速度 {{ readerStore.autoReadingLineTime }}ms</span>
                <el-slider v-model="readerAutoReadingLineTimeModel" :min="10" :max="3000" :step="50" />
              </label>
              <label>
                <span>全屏点击</span>
                <el-radio-group v-model="readerClickMethodModel" size="small">
                  <el-radio-button value="next">下一页</el-radio-button>
                  <el-radio-button value="auto">自动</el-radio-button>
                  <el-radio-button value="none">不翻页</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>选择文字</span>
                <el-radio-group v-model="readerSelectionActionModel" size="small">
                  <el-radio-button value="操作弹窗">操作弹窗</el-radio-button>
                  <el-radio-button value="忽略">忽略</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>朗读语速 {{ readerStore.ttsRate }}</span>
                <el-slider v-model="readerTTSRateModel" :min="0.5" :max="2" :step="0.1" />
              </label>
              <label>
                <span>朗读音调 {{ readerStore.ttsPitch }}</span>
                <el-slider v-model="readerTTSPitchModel" :min="0" :max="2" :step="0.1" />
              </label>
            </div>
          </article>
        </section>
      </el-tab-pane>

    </el-tabs>

  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Connection,
  Delete,
  Files,
  Refresh,
  SwitchButton,
  Upload,
  User,
  View,
} from '@element-plus/icons-vue'
import api from '../api/client'
import { clearCache, getCacheStats } from '../api/cache'
import { deleteAsset, uploadAsset } from '../api/uploads'
import { useSync } from '../composables/useSync'
import { useReaderStore, themePresets } from '../stores/reader'
import { readerFontOptions } from '../utils/readerFonts'
import { useUserStore } from '../stores/user'
import { clearBrowserLocalCacheGroup, currentBrowserLocalCacheStats } from '../utils/localCacheStats'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'

const props = defineProps({
  panel: {
    type: String,
    default: 'account',
  },
})

const router = useRouter()
const userStore = useUserStore()
const readerStore = useReaderStore()
const { connected: syncConnected } = useSync()

const workspacePanels = new Set(['account', 'cache', 'reader'])
const activeTab = ref(resolveWorkspaceSettingsPanel(props.panel))
const checking = ref(false)
const cacheStats = ref({})
const localBrowserCacheStats = ref({ total: { files: 0, size: 0 }, groups: {} })
const cacheLoading = ref(false)
const cacheClearing = ref(false)
const browserCacheClearing = ref('')
const readerBgUploading = ref(false)
const healthInfo = ref(null)
const windowWidth = ref(currentViewportWidth())

const fontOptions = readerFontOptions
const configDefaultTypes = ['白天默认', '黑夜默认']

const readerModeModel = computed({
  get: () => readerStore.mode,
  set: value => readerStore.setMode(value),
})
const readerPageTypeModel = computed({
  get: () => readerStore.pageType,
  set: value => readerStore.setPageType(value),
})
const readerPageModeModel = computed({
  get: () => readerStore.pageMode,
  set: value => readerStore.setPageMode(value),
})
const readerClickMethodModel = computed({
  get: () => readerStore.clickMethod,
  set: value => readerStore.setClickMethod(value),
})
const readerSelectionActionModel = computed({
  get: () => readerStore.selectionAction,
  set: value => readerStore.setSelectionAction(value),
})
const readerFontFamilyModel = computed({
  get: () => readerStore.fontFamily,
  set: value => readerStore.setFontFamily(value),
})
const readerChineseFontModel = computed({
  get: () => readerStore.chineseFont,
  set: value => readerStore.setChineseFont(value),
})
const readerBrightnessModel = computed({
  get: () => readerStore.brightness,
  set: value => readerStore.setBrightness(value),
})
const readerAutoReadingMethodModel = computed({
  get: () => readerStore.autoReadingMethod,
  set: value => readerStore.setAutoReadingMethod(value),
})
const readerAutoReadingPixelModel = computed({
  get: () => readerStore.autoReadingPixel,
  set: value => readerStore.setAutoReadingPixel(value),
})
const readerAutoReadingLineTimeModel = computed({
  get: () => readerStore.autoReadingLineTime,
  set: value => readerStore.setAutoReadingLineTime(value),
})
const readerAnimateDurationModel = computed({
  get: () => readerStore.animateDuration,
  set: value => readerStore.setAnimateDuration(value),
})
const readerFontSizeModel = computed({
  get: () => readerStore.fontSize,
  set: value => readerStore.setFontSize(value),
})
const readerFontWeightModel = computed({
  get: () => readerStore.fontWeight,
  set: value => readerStore.setFontWeight(value),
})
const readerLineHeightModel = computed({
  get: () => readerStore.lineHeight,
  set: value => readerStore.setLineHeight(value),
})
const readerParagraphSpaceModel = computed({
  get: () => readerStore.paragraphSpace,
  set: value => readerStore.setParagraphSpace(value),
})
const readerColumnWidthModel = computed({
  get: () => readerStore.columnWidth,
  set: value => readerStore.setColumnWidth(value),
})
const readerTTSRateModel = computed({
  get: () => readerStore.ttsRate,
  set: value => readerStore.setTTSRate(value),
})
const readerTTSPitchModel = computed({
  get: () => readerStore.ttsPitch,
  set: value => readerStore.setTTSPitch(value),
})
const readerCustomBodyColorModel = computed({
  get: () => readerStore.customBodyColor,
  set: value => readerStore.setCustomBodyColor(value || ''),
})
const readerCustomPopupColorModel = computed({
  get: () => readerStore.customPopupColor,
  set: value => readerStore.setCustomPopupColor(value || ''),
})
const readerCustomBgColorModel = computed({
  get: () => readerStore.customBgColor,
  set: value => readerStore.setCustomBgColor(value || ''),
})
const readerSettingsSyncText = computed(() => {
  if (readerStore.settingsSyncing) return '同步中'
  if (readerStore.settingsSyncError) return `同步失败：${readerStore.settingsSyncError}`
  if (readerStore.settingsSyncBaseUpdatedAt) return '已同步'
  return '本地设置'
})

const readerSettingsMiniInterface = computed(() => shouldUseMiniInterface(readerStore.pageMode, windowWidth.value))
const isMobileDialog = computed(() => readerSettingsMiniInterface.value)
const currentReaderCustomConfig = computed(() => {
  return (Array.isArray(readerStore.customConfigList) ? readerStore.customConfigList : []).find(config => config.name === readerStore.customConfigName) || null
})

function selectReaderCustomConfig(name) {
  readerStore.setCustomConfig(name)
}

async function addReaderCustomConfig() {
  const res = await ElMessageBox.prompt('请输入方案名称', '新增配置方案', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '方案名不能为空',
  }).catch(() => null)
  if (!res) return
  const result = readerStore.createCustomConfig(res.value)
  if (!result.ok) {
    ElMessage.error(result.message || '新增方案失败')
    return
  }
  ElMessage.success('已保存当前配置为新方案')
}

async function deleteReaderCustomConfig(name) {
  const confirmed = await ElMessageBox.confirm(`确定删除「${name}」方案吗？`, '删除配置方案', { type: 'warning' }).catch(() => false)
  if (!confirmed) return
  const result = readerStore.deleteCustomConfig(name)
  if (!result.ok) {
    ElMessage.error(result.message || '删除方案失败')
    return
  }
  ElMessage.success('已删除配置方案')
}

async function setReaderConfigDefaultType(type) {
  const confirmed = await ElMessageBox.confirm(`确认把「${readerStore.customConfigName}」设为${type}吗？`, '设置方案类型', { type: 'warning' }).catch(() => false)
  if (!confirmed) return
  const result = readerStore.setCustomConfigDefaultType(type)
  if (!result.ok) {
    ElMessage.error(result.message || '设置方案类型失败')
    return
  }
  ElMessage.success(`已设为${type}`)
}

onMounted(() => {
  readerStore.normalizeSettings()
  readerStore.loadReaderSettings().catch(() => {})
  window.addEventListener('resize', updateWindowWidth, { passive: true })
  loadCacheStats()
  loadHealthInfo().catch(() => {})
})

onBeforeUnmount(() => window.removeEventListener('resize', updateWindowWidth))

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

watch(
  () => props.panel,
  (panel) => {
    activeTab.value = resolveWorkspaceSettingsPanel(panel)
  },
)

function resolveWorkspaceSettingsPanel(panel) {
  const value = String(panel || '')
  return workspacePanels.has(value) ? value : 'account'
}

async function checkHealth() {
  checking.value = true
  try {
    const data = await loadHealthInfo()
    const buildText = data.buildDate && data.buildDate !== 'unknown' ? `，构建 ${data.buildDate}` : ''
    ElMessage.success(`服务连接正常${buildText}`)
  } catch (err) {
    ElMessage.error(readError(err, '服务检查失败'))
  } finally {
    checking.value = false
  }
}

async function loadHealthInfo() {
  const { data } = await api.get('/health')
  healthInfo.value = data
  return data
}

function shortCommit(value) {
  if (!value || value === 'unknown') return '-'
  return String(value).slice(0, 12)
}

async function loadCacheStats() {
  cacheLoading.value = true
  const [serverResult, browserResult] = await Promise.allSettled([
    getCacheStats(),
    currentBrowserLocalCacheStats(),
  ])
  if (serverResult.status === 'fulfilled') {
    cacheStats.value = serverResult.value?.data || {}
  } else {
    cacheStats.value = {}
    ElMessage.error(readError(serverResult.reason, '加载服务器缓存统计失败'))
  }
  if (browserResult.status === 'fulfilled') {
    localBrowserCacheStats.value = browserResult.value || { total: { files: 0, size: 0 }, groups: {} }
  } else {
    localBrowserCacheStats.value = { total: { files: 0, size: 0 }, groups: {} }
    ElMessage.error(readError(browserResult.reason, '加载浏览器缓存统计失败'))
  }
  cacheLoading.value = false
}

async function clearSystemCache() {
  try {
    await ElMessageBox.confirm('确定清理服务器章节缓存吗？清理后阅读时会重新加载远程章节内容。', '清理缓存', { type: 'warning' })
    cacheClearing.value = true
    const { data } = await clearCache()
    ElMessage.success(`已清理 ${data.clearedFiles || 0} 个文件，释放 ${formatSize(data.clearedSize || 0)}`)
    await loadCacheStats()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cacheClearing.value = false
  }
}

async function clearBrowserLocalCache(group) {
  const label = cacheGroupLabel(group)
  try {
    await ElMessageBox.confirm(`确定清理当前浏览器的${label}吗？清理后会在需要时重新加载。`, '清理浏览器缓存', { type: 'warning' })
    browserCacheClearing.value = group
    const removed = await clearBrowserLocalCacheGroup(group)
    ElMessage.success(`已清理${label} ${removed} 项`)
    await loadCacheStats()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理浏览器缓存失败'))
  } finally {
    browserCacheClearing.value = ''
  }
}

function cacheGroup(group) {
  return localBrowserCacheStats.value?.groups?.[group] || { files: 0, size: 0 }
}

function cacheGroupFiles(group) {
  return Number(cacheGroup(group).files || 0)
}

function cacheGroupText(group) {
  const row = cacheGroup(group)
  return `${row.files || 0} 项 · ${formatSize(row.size || 0)}`
}

function cacheGroupLabel(group) {
  const labels = {
    bookSourceList: '书源缓存',
    rssSources: 'RSS源缓存',
    chapterList: '章节列表缓存',
    chapterContent: '章节内容缓存',
  }
  return labels[group] || '缓存'
}

async function pickReaderBgImage(data) {
  const file = data.raw || data.file
  if (!file) return
  readerBgUploading.value = true
  try {
    const { data: result } = await uploadAsset({ file, type: 'background' })
    readerStore.addCustomBgImage(result.url)
    ElMessage.success('阅读背景图已上传')
  } catch (err) {
    ElMessage.error(readError(err, '上传背景图失败'))
  } finally {
    readerBgUploading.value = false
  }
}

async function deleteReaderBgImage(image) {
  if (!image) return
  try {
    await deleteAsset(image)
    readerStore.removeCustomBgImage(image)
    ElMessage.success('已删除阅读背景图')
  } catch (err) {
    ElMessage.error(readError(err, '删除背景图失败'))
  }
}

function logout() {
  userStore.logout()
  router.push({ name: 'login' })
}

function limitText(value) {
  return value ? value : '不限制'
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.settings-page {
  display: grid;
  gap: 16px;
}

.settings-head,
.card-head,
.panel-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.settings-head {
  justify-content: space-between;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.settings-card {
  display: grid;
  align-content: start;
  gap: 14px;
  padding: 18px;
}

.card-head {
  color: var(--app-primary);
}

.card-head h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 17px;
}

.reader-sync-state {
  margin-left: auto;
  color: var(--app-text-muted);
  font-size: 12px;
}

.reader-sync-state.error {
  color: #c45656;
}

.panel-text {
  margin: 0;
  color: var(--app-text-muted);
  line-height: 1.7;
}

.panel-actions {
  flex-wrap: wrap;
}

.cache-action-grid :deep(.el-button) {
  margin-left: 0;
}

.info-list {
  display: grid;
  gap: 8px;
  margin: 0;
}

.info-list div {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr);
  gap: 12px;
}

.info-list dt {
  color: var(--app-text-muted);
}

.info-list dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-info {
  width: 100%;
  margin-top: 8px;
}

.reader-setting-list {
  display: grid;
  gap: 14px;
}

.reader-setting-list label,
.reader-theme-row {
  display: grid;
  gap: 6px;
}

.reader-setting-list span {
  color: var(--app-text-muted);
  font-size: 13px;
}

.reader-config-schemes-row > span {
  align-self: start;
}

.reader-config-schemes {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.reader-config-scheme {
  display: inline-flex;
  min-width: 0;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--app-border);
  border-radius: 6px;
  padding: 6px 10px;
  background: var(--app-surface);
  color: var(--app-text);
  cursor: pointer;
}

.reader-config-scheme span {
  min-width: 0;
  overflow: hidden;
  color: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reader-config-scheme small {
  color: var(--app-text-muted);
  white-space: nowrap;
}

.reader-config-scheme.active {
  border-color: var(--app-primary);
  color: var(--app-primary);
  background: rgba(64, 158, 255, 0.08);
}

.reader-config-scheme.add {
  color: var(--app-primary);
}

.theme-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 10px;
}

.theme-choice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px;
  color: var(--app-text);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.theme-choice.active,
.theme-choice:hover {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.theme-swatch {
  width: 22px;
  height: 22px;
  border: 1px solid var(--app-border);
  border-radius: 50%;
}

.custom-swatch {
  background-image: linear-gradient(135deg, rgba(255,255,255,0.55), rgba(0,0,0,0.08));
}

.custom-theme-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  color: var(--app-text-muted);
  font-size: 13px;
}

.custom-theme-field {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.custom-theme-field > span {
  color: var(--app-text-muted);
  white-space: nowrap;
}

.settings-bg-list {
  flex: 1 1 100%;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(86px, 1fr));
  gap: 8px;
  min-width: 0;
}

.settings-bg-choice {
  position: relative;
  min-width: 0;
  aspect-ratio: 4 / 3;
  color: #fff;
  background-color: var(--app-bg-soft);
  background-position: center;
  background-size: cover;
  border: 2px solid transparent;
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  overflow: hidden;
}

.settings-bg-choice::before {
  position: absolute;
  inset: 0;
  content: "";
  background: linear-gradient(to top, rgba(0,0,0,0.55), rgba(0,0,0,0.04));
}

.settings-bg-choice.active {
  border-color: var(--app-primary);
}

.settings-bg-choice span,
.settings-bg-choice button {
  position: relative;
  z-index: 1;
}

.settings-bg-choice span {
  position: absolute;
  left: 8px;
  bottom: 6px;
  font-size: 12px;
  font-weight: 700;
}

.settings-bg-choice button {
  position: absolute;
  top: 4px;
  right: 4px;
  color: #fff;
  background: rgba(0,0,0,0.42);
  border: 0;
  border-radius: 999px;
  cursor: pointer;
  font-size: 12px;
  min-height: 24px;
  padding: 0 8px;
}

@media (max-width: 750px) {
  .settings-head,
  .settings-grid {
    display: grid;
    grid-template-columns: 1fr;
  }

}
</style>
