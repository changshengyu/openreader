import { defineStore } from 'pinia'
import api from '../api/client'
import { currentUserScope } from '../utils/authScope'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import {
  newestProgress as pickNewestProgress,
  progressUpdatedAt,
  reconcileAuthoritativeShelfProgress,
} from '../utils/bookOrder'
import { normalizeReaderThemeType, themeTypeForTheme } from '../utils/readerThemeType'
import { normalizeTTSPitch, normalizeTTSRate } from '../utils/readerTTS'

let readerSettingsSyncTimer
let readerSettingsPendingRemoteUpdatedAt = ''
let readerSettingsPendingRemoteUnknown = false
const READER_CLIENT_ID = readerClientId()
const readerSettingsOperations = createAuthenticatedOperationGuard()
const readerProgressOperations = createAuthenticatedOperationGuard()
const READER_FONT_FAMILIES = ['system', 'hei', 'kai', 'serif', 'fangsong', 'mono']
const READER_SETTINGS_VERSION = 14
const READER_SCHEME_FIELDS = Object.freeze([
  'mode',
  'pageMode',
  'clickMethod',
  'selectionAction',
  'fontFamily',
  'chineseFont',
  'fontSize',
  'fontWeight',
  'fontColor',
  'theme',
  'themeType',
  'customBodyColor',
  'customPopupColor',
  'customBgColor',
  'customBgImage',
  'brightness',
  'autoReadingMethod',
  'autoReadingPixel',
  'autoReadingLineTime',
  'animateDuration',
  'lineHeight',
  'paragraphSpace',
  'columnWidth',
])
const READER_PAGE_CONFIG_FIELDS = Object.freeze([
  ...READER_SCHEME_FIELDS,
  'customConfigName',
  'autoTheme',
])

export const themePresets = {
  parchment: { label: '羊皮纸', swatch: 'rgba(250, 245, 235, 0.8)', bg: '#ffffff', text: '#262626', body: '#eadfca', popup: '#ede7da', bodyImage: '/themes/body_0.png', pageImage: '/themes/content_0.png', popupImage: '/themes/popup_0.png' },
  cream:    { label: '米黄',   swatch: 'rgba(245, 234, 204, 0.8)', bg: '#f5eacc', text: '#262626', bodyImage: '/themes/body_1.png', pageImage: '/themes/content_1.png', popupImage: '/themes/popup_1.png' },
  green:    { label: '护眼绿', swatch: 'rgba(230, 242, 230, 0.8)', bg: '#e6f2e6', text: '#262626', bodyImage: '/themes/body_2.png', pageImage: '/themes/content_2.png', popupImage: '/themes/popup_2.png' },
  blue:     { label: '浅蓝',   swatch: 'rgba(228, 241, 245, 0.8)', bg: '#e4f1f5', text: '#262626', bodyImage: '/themes/body_3.png', pageImage: '/themes/content_3.png', popupImage: '/themes/popup_3.png' },
  pink:     { label: '浅粉',   swatch: 'rgba(245, 228, 228, 0.8)', bg: '#f5e4e4', text: '#262626', body: '#ebcece', popup: '#faeceb', bodyImage: 'none', pageImage: 'none', popupImage: 'none' },
  gray:     { label: '浅灰',   swatch: 'rgba(224, 224, 224, 0.8)', bg: '#e0e0e0', text: '#262626', bodyImage: '/themes/body_5.png', pageImage: '/themes/content_5.png', popupImage: '/themes/popup_5.png' },
  dark:     { label: '深色',   swatch: 'rgba(0, 0, 0, 0.5)', bg: '#000000', text: '#ffffff', body: '#000000', popup: '#171717', bodyImage: '/themes/body_6.png', pageImage: '/themes/content_6.png' },
  white:    { label: '纯白',   swatch: 'rgba(255, 255, 255, 0.8)', bg: '#ffffff', text: '#262626', body: '#f7f7f7', popup: '#f7f7f7', bodyImage: 'none', pageImage: 'none', popupImage: 'none' },
  black:    { label: '纯黑',   bg: '#000000', text: '#ffffff', body: '#000000', popup: '#121212', bodyImage: 'none', pageImage: 'none', popupImage: 'none' },
}

export const readerBuiltInBackgrounds = Object.freeze([
  '/bg/山水画.jpg',
  '/bg/山水墨影.jpg',
  '/bg/羊皮纸1.jpg',
  '/bg/护眼漫绿.jpg',
  '/bg/羊皮纸2.jpg',
  '/bg/新羊皮纸.jpg',
  '/bg/羊皮纸3.jpg',
  '/bg/明媚倾城.jpg',
  '/bg/羊皮纸4.jpg',
  '/bg/深宫魅影.jpg',
  '/bg/午后沙滩.jpg',
  '/bg/清新时光.jpg',
  '/bg/宁静夜色.jpg',
  '/bg/边彩画布.jpg',
])

export const useReaderStore = defineStore('reader', {
  state: () => ({
    mode: 'page',
    pageType: 'normal',
    pageMode: 'auto',
    clickMethod: 'auto',
    selectionAction: '操作弹窗',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    fontColor: '#262626',
    theme: 'parchment',
    themeType: 'day',
    customBodyColor: '#eadfca',
    customPopupColor: '#ede7da',
    customBgColor: '#ffffff',
    customBgImage: '',
    customBgImageList: [],
    brightness: 100,
    autoReadSpeed: 1,
    autoReadingMethod: '像素滚动',
    autoReadingPixel: 1,
    autoReadingLineTime: 1000,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: READER_SETTINGS_VERSION,
    settingsUpdatedAt: '',
    settingsSyncBaseUpdatedAt: '',
    settingsSyncing: false,
    settingsSyncError: '',
    settingsScope: currentUserScope(),
    progressScope: currentUserScope(),
    progressByBook: {},
    clientId: READER_CLIENT_ID,
    normalModeSnapshot: null,
    normalPageConfig: null,
    kindlePageConfig: null,
    customConfigName: '内置白天',
    customConfigList: defaultCustomConfigList(),
    autoTheme: true,
  }),
  persist: true,
  getters: {
    currentTheme(state) {
      if (state.theme === 'custom') {
        return {
          label: '自定义',
          bg: state.customBgColor || '#ffffff',
          text: state.fontColor || '#262626',
        }
      }
      return themePresets[state.theme] || themePresets.parchment
    },
  },
  actions: {
    ensureClientId() {
      if (this.clientId !== READER_CLIENT_ID) {
        this.clientId = READER_CLIENT_ID
      }
      return this.clientId
    },
    ensureProgressScope() {
      const scope = currentUserScope()
      if (!this.progressScope) {
        this.progressScope = scope
        return scope
      }
      if (this.progressScope !== scope) {
        readerProgressOperations.reset()
        this.progressByBook = {}
        this.progressScope = scope
      }
      return scope
    },
    ensureReaderSettingsScope() {
      const scope = currentUserScope()
      if (!this.settingsScope) {
        this.settingsScope = scope
        return scope
      }
      if (this.settingsScope !== scope) {
        this.resetReaderSettingsState(scope)
      }
      return scope
    },
    resetReaderSettingsState(scope = currentUserScope()) {
      clearTimeout(readerSettingsSyncTimer)
      readerSettingsPendingRemoteUpdatedAt = ''
      readerSettingsPendingRemoteUnknown = false
      readerSettingsOperations.reset()
      Object.assign(this, defaultReaderSettings(), {
        settingsScope: scope,
        settingsUpdatedAt: '',
        settingsSyncBaseUpdatedAt: '',
        settingsSyncing: false,
        settingsSyncError: '',
        normalModeSnapshot: null,
      })
      this.ensureClientId()
    },
    setMode(mode) {
      this.mode = ['scroll', 'scroll2', 'flip', 'page'].includes(mode) ? mode : 'page'
      this.markSettingsDirty()
    },
    setCustomConfig(name) {
      const config = (Array.isArray(this.customConfigList) ? this.customConfigList : []).find(item => item?.name === name)
      if (!config) return false
      const next = sanitizeReaderScheme(config)
      Object.assign(this, next)
      this.customConfigName = config.name
      this.normalizeSettings()
      this.markSettingsDirty({ skipCustomConfigSync: true })
      return true
    },
    createCustomConfig(name) {
      const cleanName = String(name || '').trim()
      if (!cleanName) return { ok: false, message: '方案名不能为空' }
      const current = Array.isArray(this.customConfigList) ? this.customConfigList : []
      if (current.some(item => item?.name === cleanName)) return { ok: false, message: '方案名不能重复' }
      const dayConfig = current[0] || defaultCustomConfigList()[0]
      const config = readerConfigSnapshot(dayConfig, cleanName, '')
      this.customConfigList = [...current, config]
      this.markSettingsDirty({ skipCustomConfigSync: true })
      return { ok: true }
    },
    deleteCustomConfig(name) {
      const current = Array.isArray(this.customConfigList) ? this.customConfigList : []
      const index = current.findIndex(item => item?.name === name)
      if (index < 0) return { ok: false, message: '方案不存在' }
      if (index <= 1 || current[index]?.builtin) return { ok: false, message: '内置方案不能删除' }
      if (this.customConfigName === name) return { ok: false, message: '方案正在使用，无法删除' }
      this.customConfigList = current.filter(item => item?.name !== name)
      this.markSettingsDirty({ skipCustomConfigSync: true })
      return { ok: true }
    },
    setCustomConfigDefaultType(configDefaultType) {
      if (!this.customConfigName) return { ok: false, message: '请先选择配置方案' }
      if (!['白天默认', '黑夜默认'].includes(configDefaultType)) return { ok: false, message: '方案类型无效' }
      const current = Array.isArray(this.customConfigList) ? this.customConfigList : []
      if (!current.some(item => item?.name === this.customConfigName)) return { ok: false, message: '当前方案不存在' }
      this.customConfigList = current.map(item => {
        if (item.name === this.customConfigName) return { ...item, configDefaultType }
        if (item.configDefaultType === configDefaultType) return { ...item, configDefaultType: '' }
        return item
      })
      this.markSettingsDirty({ skipCustomConfigSync: true })
      return { ok: true }
    },
    setAutoTheme(autoTheme) {
      this.autoTheme = Boolean(autoTheme)
      this.markSettingsDirty({ skipCustomConfigSync: true })
    },
    applyAutoTheme(isNight) {
      if (!this.autoTheme) return false
      return this.setNightTheme(isNight)
    },
    setNightTheme(isNight) {
      const type = isNight ? '黑夜默认' : '白天默认'
      const fallback = defaultCustomConfigList()[isNight ? 1 : 0]
      const current = Array.isArray(this.customConfigList) ? this.customConfigList : []
      const configured = current.find(item => (
        item?.configDefaultType === type
        && typeof item?.name === 'string'
        && item.name.trim()
        && typeof item?.theme === 'string'
        && item.theme.trim()
      ))
      const config = configured || fallback
      if (!configured && !current.some(item => item?.name === fallback.name)) {
        this.customConfigList = [...current, fallback]
      }
      const next = sanitizeReaderScheme(config)
      Object.assign(this, next)
      this.customConfigName = config.name
      this.normalizeSettings()
      this.markSettingsDirty({ skipCustomConfigSync: true })
      return true
    },
    setPageType(pageType) {
      const nextType = ['kindle', 'simple', 'Kindle'].includes(pageType) ? 'kindle' : 'normal'
      if (nextType === this.pageType) return
      if (nextType === 'kindle') {
        this.normalPageConfig = readerPageConfigSnapshot(this)
        const target = this.kindlePageConfig || {
          ...readerPageConfigSnapshot(this),
          animateDuration: 0,
          fontSize: Math.min(this.fontSize, 20),
          theme: 'white',
          themeType: 'day',
          fontColor: '#262626',
          mode: 'flip',
          selectionAction: '忽略',
          pageMode: 'mobile',
        }
        applyReaderPageConfig(this, target)
        this.pageType = 'kindle'
      } else {
        this.kindlePageConfig = readerPageConfigSnapshot(this)
        const snapshot = this.normalPageConfig
          || sanitizePageConfigSnapshot(this.normalModeSnapshot)
          || readerPageConfigSnapshot(defaultReaderSettings())
        applyReaderPageConfig(this, snapshot)
        this.pageType = 'normal'
      }
      this.markSettingsDirty()
    },
    setPageMode(pageMode) {
      const next = pageMode === 'mobile' ? 'mobile' : 'auto'
      if (this.pageMode === next) return
      this.pageMode = next
      this.markSettingsDirty()
    },
    setClickMethod(method) {
      this.clickMethod = ['next', 'auto', 'none'].includes(method) ? method : 'auto'
      this.markSettingsDirty()
    },
    setSelectionAction(action) {
      this.selectionAction = ['操作弹窗', '忽略'].includes(action) ? action : '操作弹窗'
      this.markSettingsDirty()
    },
    setFontFamily(fontFamily) {
      this.fontFamily = READER_FONT_FAMILIES.includes(fontFamily) ? fontFamily : 'system'
      this.markSettingsDirty()
    },
    setChineseFont(chineseFont) {
      this.chineseFont = chineseFont === '繁体' ? '繁体' : '简体'
      this.markSettingsDirty()
    },
    setCustomFont(fontFamily, url) {
      if (!READER_FONT_FAMILIES.includes(fontFamily) || !url) return
      this.customFontsMap = {
        ...(this.customFontsMap || {}),
        [fontFamily]: url,
      }
      this.markSettingsDirty()
    },
    clearCustomFont(fontFamily) {
      if (!this.customFontsMap?.[fontFamily]) return
      const next = { ...this.customFontsMap }
      delete next[fontFamily]
      this.customFontsMap = next
      this.markSettingsDirty()
    },
    setFontSize(fontSize) {
      this.fontSize = numberAtLeast(fontSize, 8, 18)
      this.markSettingsDirty()
    },
    setFontWeight(fontWeight) {
      this.fontWeight = clampNumber(fontWeight, 100, 900, 400)
      this.markSettingsDirty()
    },
    setFontColor(fontColor) {
      this.fontColor = typeof fontColor === 'string' ? fontColor : ''
      this.markSettingsDirty()
    },
    setTheme(theme) {
      this.theme = theme
      this.themeType = themeTypeForTheme(theme, this.themeType)
      this.markSettingsDirty()
    },
    setThemeType(themeType) {
      this.themeType = normalizeReaderThemeType(themeType, this.theme)
      this.markSettingsDirty()
    },
    setCustomBodyColor(color) {
      this.customBodyColor = typeof color === 'string' ? color : ''
      this.markSettingsDirty()
    },
    setCustomPopupColor(color) {
      this.customPopupColor = typeof color === 'string' ? color : ''
      this.markSettingsDirty()
    },
    setCustomBgColor(color) {
      this.customBgColor = color
      this.markSettingsDirty()
    },
    setCustomBgImage(image) {
      this.customBgImage = image
      this.markSettingsDirty()
    },
    addCustomBgImage(image) {
      if (!image) return
      const current = Array.isArray(this.customBgImageList) ? this.customBgImageList : []
      this.customBgImageList = current.includes(image) ? current : [...current, image]
      this.customBgImage = image
      this.markSettingsDirty()
    },
    removeCustomBgImage(image) {
      if (!image) return
      this.customBgImageList = (Array.isArray(this.customBgImageList) ? this.customBgImageList : []).filter(item => item !== image)
      if (this.customBgImage === image) this.customBgImage = readerBuiltInBackgrounds[0]
      this.markSettingsDirty()
    },
    setBrightness(brightness) {
      this.brightness = clampNumber(brightness, 50, 150, 100)
      this.markSettingsDirty()
    },
    setAutoReadSpeed(speed) {
      this.setAutoReadingPixel(speed)
    },
    setAutoReadingMethod(method) {
      this.autoReadingMethod = method === '段落滚动' ? '段落滚动' : '像素滚动'
      this.markSettingsDirty()
    },
    setAutoReadingPixel(pixel) {
      this.autoReadingPixel = numberAtLeast(pixel, 1, 1)
      this.autoReadSpeed = this.autoReadingPixel
      this.markSettingsDirty()
    },
    setAutoReadingLineTime(lineTime) {
      this.autoReadingLineTime = numberAtLeast(lineTime, 10, 1000)
      this.markSettingsDirty()
    },
    setAnimateDuration(duration) {
      this.animateDuration = clampNumber(duration, 0, 500, 300)
      this.markSettingsDirty()
    },
    setTTSRate(rate) {
      this.ttsRate = normalizeTTSRate(rate)
      this.markSettingsDirty()
    },
    setTTSPitch(pitch) {
      this.ttsPitch = normalizeTTSPitch(pitch)
      this.markSettingsDirty()
    },
    setTTSVoice(uri) {
      this.ttsVoiceURI = uri || ''
      this.markSettingsDirty()
    },
    setLineHeight(lineHeight) {
      this.lineHeight = clampNumber(lineHeight, 1, 5, 1.8)
      this.markSettingsDirty()
    },
    setParagraphSpace(paragraphSpace) {
      this.paragraphSpace = clampNumber(paragraphSpace, 0, 5, 0)
      this.markSettingsDirty()
    },
    setColumnWidth(columnWidth) {
      this.columnWidth = numberAtLeast(columnWidth, 160, 800)
      this.markSettingsDirty()
    },
    resetReaderSettings() {
      const customConfigList = sanitizeCustomConfigList(this.customConfigList)
      const customFontsMap = { ...(this.customFontsMap || {}) }
      const customBgImageList = sanitizeStringList(this.customBgImageList)
      const ttsRate = this.ttsRate
      const ttsPitch = this.ttsPitch
      const ttsVoiceURI = this.ttsVoiceURI
      Object.assign(this, defaultReaderSettings(), {
        customConfigList,
        customFontsMap,
        customBgImageList,
        ttsRate,
        ttsPitch,
        ttsVoiceURI,
      })
      this.markSettingsDirty()
    },
    normalizeSettings() {
      this.ensureProgressScope()
      if (!['scroll', 'scroll2', 'flip', 'page'].includes(this.mode)) this.mode = 'page'
      if (this.pageType === 'simple' || this.pageType === 'Kindle') this.pageType = 'kindle'
      if (!['normal', 'kindle'].includes(this.pageType)) this.pageType = 'normal'
      if (!['auto', 'mobile'].includes(this.pageMode)) this.pageMode = 'auto'
      if (!['next', 'auto', 'none'].includes(this.clickMethod)) this.clickMethod = 'auto'
      if (!['操作弹窗', '忽略'].includes(this.selectionAction)) this.selectionAction = '操作弹窗'
      if (!READER_FONT_FAMILIES.includes(this.fontFamily)) this.fontFamily = 'system'
      if (!['简体', '繁体'].includes(this.chineseFont)) this.chineseFont = '简体'
      if (!this.customFontsMap || typeof this.customFontsMap !== 'object' || Array.isArray(this.customFontsMap)) this.customFontsMap = {}
      if (!Array.isArray(this.customBgImageList)) this.customBgImageList = []
      if (!Array.isArray(this.customConfigList) || !this.customConfigList.length) this.customConfigList = defaultCustomConfigList()
      this.customConfigList = sanitizeCustomConfigList(this.customConfigList)
      if (!this.customConfigList.some(item => item.name === this.customConfigName)) {
        this.customConfigName = this.customConfigList[0]?.name || '内置白天'
      }
      this.autoTheme = this.autoTheme === true
      this.fontSize = numberAtLeast(this.fontSize, 8, 18)
      this.fontWeight = clampNumber(this.fontWeight, 100, 900, 400)
      if (typeof this.fontColor !== 'string') this.fontColor = ''
      this.themeType = normalizeReaderThemeType(this.themeType, this.theme)
      if (typeof this.customBodyColor !== 'string') this.customBodyColor = ''
      if (typeof this.customPopupColor !== 'string') this.customPopupColor = ''
      this.lineHeight = clampNumber(this.lineHeight, 1, 5, 1.8)
      this.paragraphSpace = clampNumber(this.paragraphSpace, 0, 5, 0)
      this.columnWidth = numberAtLeast(this.columnWidth, 160, 800)
      this.brightness = clampNumber(this.brightness, 50, 150, 100)
      if (!['像素滚动', '段落滚动'].includes(this.autoReadingMethod)) this.autoReadingMethod = '像素滚动'
      this.autoReadingPixel = numberAtLeast(this.autoReadingPixel ?? this.autoReadSpeed, 1, 1)
      this.autoReadSpeed = this.autoReadingPixel
      this.autoReadingLineTime = numberAtLeast(this.autoReadingLineTime, 10, 1000)
      this.animateDuration = clampNumber(this.animateDuration, 0, 500, 300)
      this.normalPageConfig = sanitizePageConfigSnapshot(this.normalPageConfig || this.normalModeSnapshot)
      this.kindlePageConfig = sanitizePageConfigSnapshot(this.kindlePageConfig)
      this.ttsRate = normalizeTTSRate(this.ttsRate)
      this.ttsPitch = normalizeTTSPitch(this.ttsPitch)
      if ((this.settingsVersion || 0) < 4) {
        this.fontSize = 18
        this.fontWeight = 400
        this.lineHeight = 1.8
        this.paragraphSpace = 0.2
        this.columnWidth = 800
      }
      this.settingsVersion = READER_SETTINGS_VERSION
      this.settingsSyncing = false
    },
    markSettingsDirty(options = {}) {
      this.ensureReaderSettingsScope()
      if (options.localOnly) return
      if (!options.skipCustomConfigSync) this.syncActiveCustomConfig()
      this.settingsUpdatedAt = new Date().toISOString()
      this.settingsSyncError = ''
      this.scheduleSettingsSync()
    },
    syncActiveCustomConfig() {
      if (!this.customConfigName || !Array.isArray(this.customConfigList)) return
      const index = this.customConfigList.findIndex(item => item?.name === this.customConfigName)
      if (index < 0) return
      const current = this.customConfigList[index]
      const next = {
        ...readerConfigSnapshot(this, current.name, current.configDefaultType || ''),
        builtin: current.builtin === true,
      }
      this.customConfigList = this.customConfigList.map((item, itemIndex) => itemIndex === index ? next : item)
    },
    scheduleSettingsSync() {
      this.ensureReaderSettingsScope()
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return
      clearTimeout(readerSettingsSyncTimer)
      readerSettingsSyncTimer = setTimeout(() => {
        this.saveReaderSettings().catch(() => {})
      }, 700)
    },
    applyReaderSettings(payload, updatedAt = '') {
      this.ensureReaderSettingsScope()
      if (!payload || typeof payload !== 'object') return
      const next = sanitizeReaderSettings(payload)
      Object.assign(this, next)
      this.normalizeSettings()
      if (updatedAt) {
        this.settingsSyncBaseUpdatedAt = updatedAt
        this.settingsUpdatedAt = updatedAt
      }
      this.settingsSyncError = ''
    },
    async loadReaderSettings(options = {}) {
      this.ensureReaderSettingsScope()
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return null
      const operation = readerSettingsOperations.begin('reader')
      this.settingsSyncing = false
      try {
        const { data } = await api.get('/settings/reader')
        if (!readerSettingsOperations.canCommit(operation)) return null
        const serverUpdatedAt = data?.updatedAt || ''
        if (data?.value && typeof data.value === 'object') {
          if (options.createIfMissing !== false && this.settingsUpdatedAt && serverUpdatedAt && this.settingsUpdatedAt > serverUpdatedAt && this.settingsSyncBaseUpdatedAt !== serverUpdatedAt) {
            return await this.saveReaderSettings()
          }
          this.applyReaderSettings(data.value, serverUpdatedAt)
          return data.value
        }
        if (options.createIfMissing === false) {
          this.settingsSyncError = '没有备份文件'
          return null
        }
        return await this.saveReaderSettings()
      } catch (err) {
        if (!readerSettingsOperations.canCommit(operation)) return null
        this.settingsSyncError = readErrorMessage(err)
        return null
      }
    },
    reconcileReaderSettingsUpdate(updatedAt = '') {
      this.ensureReaderSettingsScope()
      const remoteUpdatedAt = String(updatedAt || '').trim()
      if (remoteUpdatedAt && !readerSettingTimestampIsNewer(remoteUpdatedAt, this.settingsSyncBaseUpdatedAt)) {
        return null
      }
      if (this.settingsSyncing) {
        if (remoteUpdatedAt) {
          if (readerSettingTimestampIsNewer(remoteUpdatedAt, readerSettingsPendingRemoteUpdatedAt)) {
            readerSettingsPendingRemoteUpdatedAt = remoteUpdatedAt
          }
        } else {
          readerSettingsPendingRemoteUnknown = true
        }
        return null
      }
      return this.loadReaderSettings({ createIfMissing: false })
    },
    async saveReaderSettings(options = {}) {
      this.ensureReaderSettingsScope()
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return null
      clearTimeout(readerSettingsSyncTimer)
      const operation = readerSettingsOperations.begin('reader')
      this.settingsSyncing = true
      this.settingsSyncError = ''
      try {
        const { data, headers } = await api.put('/settings/reader', {
          value: readerSettingsPayload(this),
          baseUpdatedAt: this.settingsSyncBaseUpdatedAt || '',
          ...(options.force === true ? { force: true } : {}),
        })
        if (!readerSettingsOperations.canCommit(operation)) return null
        if (data?.value && headers?.['x-openreader-setting-conflict']) {
          this.applyReaderSettings(data.value, data.updatedAt || '')
          return data.value
        }
        if (data?.updatedAt) {
          this.settingsSyncBaseUpdatedAt = data.updatedAt
          this.settingsUpdatedAt = data.updatedAt
        }
        return data?.value || readerSettingsPayload(this)
      } catch (err) {
        if (!readerSettingsOperations.canCommit(operation)) return null
        this.settingsSyncError = readErrorMessage(err)
        return null
      } finally {
        if (readerSettingsOperations.canCommit(operation)) {
          this.settingsSyncing = false
          const pendingUpdatedAt = readerSettingsPendingRemoteUpdatedAt
          const pendingUnknown = readerSettingsPendingRemoteUnknown
          readerSettingsPendingRemoteUpdatedAt = ''
          readerSettingsPendingRemoteUnknown = false
          if (pendingUnknown || readerSettingTimestampIsNewer(pendingUpdatedAt, this.settingsSyncBaseUpdatedAt)) {
            Promise.resolve().then(() => {
              this.reconcileReaderSettingsUpdate(pendingUnknown ? '' : pendingUpdatedAt)?.catch?.(() => {})
            })
          }
        }
      }
    },
    applyProgress(progress) {
      if (!progress?.bookId) return
      this.ensureProgressScope()
      const current = pickNewestProgress(this.progressByBook[progress.bookId], readLocalChapterProgress(progress.bookId))
      const next = pickNewestProgress(current, progress)
      if (!next) return
      this.progressByBook[progress.bookId] = next
      persistLocalChapterProgress(next)
    },
    applyServerProgress(progress) {
      if (!progress?.bookId) return null
      this.ensureProgressScope()
      const local = newestProgress(this.progressByBook[progress.bookId], readLocalChapterProgress(progress.bookId))
      if (local?.bookId && progressUpdatedAt(local) > progressUpdatedAt(progress)) {
        if (local.pendingSync) this.syncLocalProgress(local, local.baseUpdatedAt || progress.updatedAt || '').catch(() => {})
        return local
      }
      this.replaceProgress(progress)
      return progress
    },
    async reconcileShelfProgress(books, options = {}) {
      this.ensureProgressScope()
      const reconciled = {}
      const serverBooks = Array.isArray(books) ? books : []
      await Promise.all(serverBooks.map(async (book) => {
        const bookId = Number(book?.id || book?.progress?.bookId || 0)
        if (!bookId) return
        const local = newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
        const server = book?.progress?.bookId ? book.progress : null
        const decision = reconcileAuthoritativeShelfProgress(local, server)
        if (decision.retryPending) {
          this.progressByBook[bookId] = decision.progress
          persistLocalChapterProgress(decision.progress)
          const synchronization = this.syncLocalProgress(
            decision.progress,
            decision.progress.baseUpdatedAt || server?.updatedAt || '',
            options.awaitPending
              ? { acceptConflict: true, throwOnError: true }
              : {},
          )
          if (options.awaitPending) {
            const winner = await synchronization
            reconciled[bookId] = winner?.bookId
              ? winner
              : this.progressByBook[bookId] || decision.progress
          } else {
            synchronization.catch(() => {})
            reconciled[bookId] = decision.progress
          }
          return
        }
        if (decision.progress) {
          this.replaceProgress(decision.progress)
          reconciled[bookId] = this.progressByBook[bookId]
          return
        }
        this.clearProgress(bookId)
        reconciled[bookId] = null
      }))
      return reconciled
    },
    cachedProgress(bookId) {
      if (!bookId) return null
      this.ensureProgressScope()
      return newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
    },
    replaceProgress(progress) {
      if (!progress?.bookId) return
      this.ensureProgressScope()
      const next = clearLocalProgressFlags(progress)
      this.progressByBook[progress.bookId] = next
      persistLocalChapterProgress(next)
    },
    clearProgress(bookId) {
      if (!bookId) return
      this.ensureProgressScope()
      delete this.progressByBook[bookId]
      removeLocalChapterProgress(bookId)
    },
    async saveProgress(payload) {
      this.ensureProgressScope()
      const operation = readerProgressOperations.begin(`book:${payload.bookId}`)
      const currentProgress = this.progressByBook[payload.bookId]
      const optimistic = {
        ...payload,
        mode: this.mode,
        updatedAt: new Date().toISOString(),
        pendingSync: true,
        baseUpdatedAt: payload.baseUpdatedAt || progressServerBaseUpdatedAt(currentProgress),
      }
      this.applyProgress(optimistic)
      const response = await api.put('/progress', {
        ...payload,
        mode: this.mode,
        baseUpdatedAt: optimistic.baseUpdatedAt,
        clientUpdatedAt: optimistic.updatedAt,
        clientId: this.ensureClientId(),
      })
      if (!readerProgressOperations.canCommit(operation)) return null
      const merged = mergeProgressResponse(response.data, optimistic)
      if (isProgressConflict(response) && shouldRetryProgressConflict(optimistic, merged)) {
        const retried = await this.syncLocalProgress(optimistic, merged?.updatedAt || optimistic.baseUpdatedAt || '', {
          force: true,
          operation,
        })
        if (retried?.bookId) return retried
      }
      if (!readerProgressOperations.canCommit(operation)) return null
      this.replaceProgress(merged)
      return merged
    },
    async loadProgress(bookId, options = {}) {
      this.ensureProgressScope()
      const operation = readerProgressOperations.begin(`book:${bookId}`)
      const local = newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
      if (options.preferLocal && local?.bookId && local.pendingSync) {
        api.get(`/progress/${bookId}`)
          .then(({ data }) => {
            if (readerProgressOperations.canCommit(operation) && data?.bookId) this.applyServerProgress(data)
          })
          .catch(() => {})
        return local
      }
      let data = null
      try {
        const res = await api.get(`/progress/${bookId}`)
        if (!readerProgressOperations.canCommit(operation)) return null
        data = res.data
      } catch {
        if (!readerProgressOperations.canCommit(operation)) return null
        return local || null
      }
      if (data?.bookId) {
        if (local?.pendingSync && progressUpdatedAt(local) > progressUpdatedAt(data)) {
          this.syncLocalProgress(local, local.baseUpdatedAt || data.updatedAt, { operation })
          return local
        }
        this.replaceProgress(data)
        return data
      }
      if (local?.bookId && local.pendingSync) {
        this.syncLocalProgress(local, local.baseUpdatedAt || data?.updatedAt, { operation })
      }
      return local || data
    },
    async syncLocalProgress(progress, baseUpdatedAt = '', options = {}) {
      if (!progress?.bookId) return null
      this.ensureProgressScope()
      const operation = options.operation || readerProgressOperations.begin(`book:${progress.bookId}`)
      if (!readerProgressOperations.canCommit(operation)) return null
      try {
        const response = await api.put('/progress', {
          bookId: progress.bookId,
          chapterId: progress.chapterId,
          chapterIndex: progress.chapterIndex,
          offset: progress.offset,
          percent: progress.percent,
          chapterPercent: progress.chapterPercent,
          chapterTitle: progress.chapterTitle,
          mode: progress.mode || this.mode,
          baseUpdatedAt: baseUpdatedAt || progress.baseUpdatedAt || '',
          clientUpdatedAt: progress.updatedAt || '',
          clientId: this.ensureClientId(),
        })
        if (!readerProgressOperations.canCommit(operation)) return null
        const next = mergeProgressResponse(response.data, progress)
        if (
          isProgressConflict(response)
          && shouldRetryProgressConflict(progress, next)
          && !options.force
          && !options.acceptConflict
        ) {
          return await this.syncLocalProgress(progress, next?.updatedAt || progress.baseUpdatedAt || '', {
            force: true,
            operation,
          })
        }
        if (!readerProgressOperations.canCommit(operation)) return null
        this.replaceProgress(next)
        return next
      } catch (error) {
        if (options.throwOnError) throw error
        return null
      }
    },
  },
})

function newestProgress(a, b) {
  return pickNewestProgress(a, b)
}

function clearLocalProgressFlags(progress) {
  if (!progress) return progress
  const { pendingSync, baseUpdatedAt, ...rest } = progress
  return rest
}

function mergeProgressResponse(data, fallback) {
  if (!data?.bookId) return data
  return {
    ...data,
    chapterPercent: Number.isFinite(Number(data.chapterPercent))
      ? Number(data.chapterPercent)
      : fallback?.chapterPercent,
    chapterTitle: data.chapterTitle || fallback?.chapterTitle,
  }
}

function isProgressConflict(response) {
  return String(response?.headers?.['x-openreader-progress-conflict'] || '') === '1'
}

function shouldRetryProgressConflict(local, server) {
  if (!local?.bookId || !server?.bookId) return false
  if (Number(local.bookId) !== Number(server.bookId)) return false
  if (progressUpdatedAt(local) <= progressUpdatedAt(server)) return false
  return progressSaveFingerprint(local) !== progressSaveFingerprint(server)
}

function progressSaveFingerprint(progress) {
  if (!progress) return ''
  return [
    Number(progress.bookId || 0),
    Number(progress.chapterId || 0),
    Number(progress.chapterIndex || 0),
    Number(progress.offset || 0),
    Math.round(Number(progress.percent || 0) * 100000),
    Math.round(Number(progress.chapterPercent || 0) * 100000),
    progress.chapterTitle || '',
    progress.mode || '',
  ].join(':')
}

function progressServerBaseUpdatedAt(progress) {
  if (!progress) return ''
  if (progress.pendingSync) return progress.baseUpdatedAt || ''
  return progress.updatedAt || ''
}

function localChapterProgressKey(bookId) {
  return `openreader_chapter_progress@${currentUserScope()}@${bookId}`
}

function legacyLocalChapterProgressKey(bookId) {
  return `openreader_chapter_progress@${bookId}`
}

function persistLocalChapterProgress(progress) {
  if (typeof localStorage === 'undefined' || !progress?.bookId) return
  const chapterPercent = Number(progress.chapterPercent)
  try {
    const payload = {
      bookId: progress.bookId,
      chapterId: progress.chapterId || 0,
      chapterIndex: Number(progress.chapterIndex || 0),
      offset: Math.max(0, Math.floor(Number(progress.offset || 0))),
      percent: Math.max(0, Math.min(1, Number(progress.percent || 0))),
      mode: progress.mode || '',
      chapterTitle: progress.chapterTitle || '',
      updatedAt: progress.updatedAt || new Date().toISOString(),
    }
    if (progress.pendingSync) {
      payload.pendingSync = true
      payload.baseUpdatedAt = progress.baseUpdatedAt || ''
    }
    if (Number.isFinite(chapterPercent)) {
      payload.chapterPercent = Math.max(0, Math.min(1, chapterPercent))
    }
    localStorage.setItem(localChapterProgressKey(progress.bookId), JSON.stringify(payload))
  } catch {
    // localStorage may be unavailable in private or restricted browser modes.
  }
}

function readLocalChapterProgress(bookId) {
  if (typeof localStorage === 'undefined' || !bookId) return null
  try {
    const scopedKey = localChapterProgressKey(bookId)
    const legacyKey = legacyLocalChapterProgressKey(bookId)
    const scopedRaw = localStorage.getItem(scopedKey)
    const legacyRaw = scopedRaw ? '' : localStorage.getItem(legacyKey)
    const raw = scopedRaw || legacyRaw
    if (!raw) return null
    const data = JSON.parse(raw)
    if (!data || Number(data.bookId) !== Number(bookId)) return null
    const progress = {
      ...data,
      bookId: Number(data.bookId),
      chapterIndex: Math.max(0, Math.floor(Number(data.chapterIndex || 0))),
      offset: Math.max(0, Math.floor(Number(data.offset || 0))),
      percent: Math.max(0, Math.min(1, Number(data.percent || 0))),
    }
    if (data.chapterPercent !== undefined && data.chapterPercent !== null) {
      const chapterPercent = Number(data.chapterPercent)
      if (Number.isFinite(chapterPercent)) progress.chapterPercent = Math.max(0, Math.min(1, chapterPercent))
    }
    if (!scopedRaw && legacyRaw && currentUserScope() !== 'anonymous') {
      localStorage.setItem(scopedKey, JSON.stringify(progress))
      localStorage.removeItem(legacyKey)
    }
    return progress
  } catch {
    return null
  }
}

function removeLocalChapterProgress(bookId) {
  if (typeof localStorage === 'undefined' || !bookId) return
  try {
    localStorage.removeItem(localChapterProgressKey(bookId))
    localStorage.removeItem(legacyLocalChapterProgressKey(bookId))
  } catch {
    // localStorage may be unavailable in private or restricted browser modes.
  }
}

function clampNumber(value, min, max, fallback) {
  const number = Number(value)
  return Math.max(min, Math.min(max, Number.isFinite(number) ? number : fallback))
}

function numberAtLeast(value, min, fallback) {
  const number = Number(value)
  return Math.max(min, Number.isFinite(number) ? number : fallback)
}

function readerClientId() {
  if (typeof sessionStorage === 'undefined') return makeClientId()
  const key = 'openreader_reader_client_id'
  try {
    const current = sessionStorage.getItem(key)
    if (current) return current
    const next = makeClientId()
    sessionStorage.setItem(key, next)
    return next
  } catch {
    return makeClientId()
  }
}

function makeClientId() {
  const random = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  return `web-${random}`
}

function readerSettingsPayload(state) {
  return {
    mode: state.mode,
    pageType: state.pageType,
    pageMode: state.pageMode,
    clickMethod: state.clickMethod,
    selectionAction: state.selectionAction,
    fontFamily: state.fontFamily,
    customFontsMap: state.customFontsMap || {},
    chineseFont: state.chineseFont,
    fontSize: state.fontSize,
    fontWeight: state.fontWeight,
    fontColor: state.fontColor || '',
    theme: state.theme,
    themeType: normalizeReaderThemeType(state.themeType, state.theme),
    customBodyColor: state.customBodyColor || '',
    customPopupColor: state.customPopupColor || '',
    customBgColor: state.customBgColor,
    customBgImage: state.customBgImage,
    customBgImageList: Array.isArray(state.customBgImageList) ? state.customBgImageList : [],
    customConfigName: state.customConfigName || '内置白天',
    customConfigList: sanitizeCustomConfigList(state.customConfigList),
    autoTheme: state.autoTheme === true,
    brightness: state.brightness,
    autoReadSpeed: state.autoReadSpeed,
    autoReadingMethod: state.autoReadingMethod,
    autoReadingPixel: state.autoReadingPixel,
    autoReadingLineTime: state.autoReadingLineTime,
    animateDuration: state.animateDuration,
    ttsRate: state.ttsRate,
    ttsPitch: state.ttsPitch,
    ttsVoiceURI: state.ttsVoiceURI,
    lineHeight: state.lineHeight,
    paragraphSpace: state.paragraphSpace,
    columnWidth: state.columnWidth,
    normalPageConfig: sanitizePageConfigSnapshot(state.normalPageConfig),
    kindlePageConfig: sanitizePageConfigSnapshot(state.kindlePageConfig),
    settingsVersion: READER_SETTINGS_VERSION,
  }
}

function defaultReaderSettings() {
  return {
    mode: 'page',
    pageType: 'normal',
    pageMode: 'auto',
    clickMethod: 'auto',
    selectionAction: '操作弹窗',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    fontColor: '#262626',
    theme: 'parchment',
    themeType: 'day',
    customBodyColor: '#eadfca',
    customPopupColor: '#ede7da',
    customBgColor: '#ffffff',
    customBgImage: '',
    customBgImageList: [],
    customConfigName: '内置白天',
    customConfigList: defaultCustomConfigList(),
    autoTheme: true,
    brightness: 100,
    autoReadSpeed: 1,
    autoReadingMethod: '像素滚动',
    autoReadingPixel: 1,
    autoReadingLineTime: 1000,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: READER_SETTINGS_VERSION,
    normalModeSnapshot: null,
    normalPageConfig: null,
    kindlePageConfig: null,
  }
}

function sanitizeReaderSettings(payload, options = {}) {
  const includeCustomConfigs = options.includeCustomConfigs !== false
  const settings = {}
  if (['scroll', 'scroll2', 'flip', 'page'].includes(payload.mode)) settings.mode = payload.mode
  if (['normal', 'kindle'].includes(payload.pageType)) settings.pageType = payload.pageType
  if (payload.pageType === 'simple' || payload.pageType === 'Kindle') settings.pageType = 'kindle'
  if (['auto', 'mobile'].includes(payload.pageMode)) settings.pageMode = payload.pageMode
  if (['next', 'auto', 'none'].includes(payload.clickMethod)) settings.clickMethod = payload.clickMethod
  if (['操作弹窗', '忽略'].includes(payload.selectionAction)) settings.selectionAction = payload.selectionAction
  if (READER_FONT_FAMILIES.includes(payload.fontFamily)) settings.fontFamily = payload.fontFamily
  settings.customFontsMap = sanitizeCustomFontsMap(payload.customFontsMap)
  settings.chineseFont = payload.chineseFont === '繁体' ? '繁体' : '简体'
  if (typeof payload.theme === 'string') settings.theme = payload.theme
  settings.themeType = normalizeReaderThemeType(payload.themeType, payload.theme)
  settings.customBodyColor = typeof payload.customBodyColor === 'string' ? payload.customBodyColor : '#eadfca'
  settings.customPopupColor = typeof payload.customPopupColor === 'string' ? payload.customPopupColor : '#ede7da'
  if (typeof payload.customBgColor === 'string') settings.customBgColor = payload.customBgColor
  if (typeof payload.customBgImage === 'string') settings.customBgImage = payload.customBgImage
  settings.customBgImageList = sanitizeStringList(payload.customBgImageList)
  if (includeCustomConfigs) {
    if (typeof payload.customConfigName === 'string') settings.customConfigName = payload.customConfigName
    settings.customConfigList = sanitizeCustomConfigList(payload.customConfigList)
    if (Object.prototype.hasOwnProperty.call(payload, 'autoTheme')) {
      settings.autoTheme = payload.autoTheme === true
    }
  }
  if (typeof payload.ttsVoiceURI === 'string') settings.ttsVoiceURI = payload.ttsVoiceURI
  settings.fontSize = numberAtLeast(payload.fontSize, 8, 18)
  settings.fontWeight = clampNumber(payload.fontWeight, 100, 900, 400)
  settings.fontColor = typeof payload.fontColor === 'string' ? payload.fontColor : '#262626'
  settings.brightness = clampNumber(payload.brightness, 50, 150, 100)
  settings.autoReadingMethod = payload.autoReadingMethod === '段落滚动' ? '段落滚动' : '像素滚动'
  settings.autoReadingPixel = numberAtLeast(payload.autoReadingPixel ?? payload.autoReadSpeed, 1, 1)
  settings.autoReadSpeed = settings.autoReadingPixel
  settings.autoReadingLineTime = numberAtLeast(payload.autoReadingLineTime, 10, 1000)
  settings.animateDuration = clampNumber(payload.animateDuration, 0, 500, 300)
  settings.ttsRate = normalizeTTSRate(payload.ttsRate)
  settings.ttsPitch = normalizeTTSPitch(payload.ttsPitch)
  settings.lineHeight = clampNumber(payload.lineHeight, 1, 5, 1.8)
  settings.paragraphSpace = clampNumber(payload.paragraphSpace, 0, 5, 0.2)
  settings.columnWidth = numberAtLeast(payload.columnWidth, 160, 800)
  settings.normalPageConfig = sanitizePageConfigSnapshot(payload.normalPageConfig || payload.normalModeSnapshot)
  settings.kindlePageConfig = sanitizePageConfigSnapshot(payload.kindlePageConfig)
  settings.settingsVersion = READER_SETTINGS_VERSION
  return settings
}

function sanitizeCustomFontsMap(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return READER_FONT_FAMILIES.reduce((map, key) => {
    if (typeof value[key] === 'string' && value[key]) map[key] = value[key]
    return map
  }, {})
}

function sanitizeStringList(value) {
  if (!Array.isArray(value)) return []
  return [...new Set(value.filter(item => typeof item === 'string' && item))]
}

function sanitizeReaderScheme(value) {
  const source = value && typeof value === 'object' ? value : {}
  const sanitized = sanitizeReaderSettings(source, { includeCustomConfigs: false })
  return pickReaderFields(sanitized, READER_SCHEME_FIELDS)
}

function readerPageConfigSnapshot(state) {
  return {
    ...sanitizeReaderScheme(state),
    customConfigName: typeof state?.customConfigName === 'string' && state.customConfigName
      ? state.customConfigName
      : '内置白天',
    autoTheme: state?.autoTheme === true,
  }
}

function sanitizePageConfigSnapshot(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const snapshot = {
    ...sanitizeReaderScheme(value),
  }
  if (typeof value.customConfigName === 'string' && value.customConfigName) {
    snapshot.customConfigName = value.customConfigName
  }
  if (Object.prototype.hasOwnProperty.call(value, 'autoTheme')) {
    snapshot.autoTheme = value.autoTheme === true
  }
  return snapshot
}

function applyReaderPageConfig(state, value) {
  const snapshot = sanitizePageConfigSnapshot(value)
  if (!snapshot) return false
  Object.assign(state, pickReaderFields(snapshot, READER_PAGE_CONFIG_FIELDS))
  return true
}

function pickReaderFields(value, fields) {
  return fields.reduce((result, field) => {
    if (Object.prototype.hasOwnProperty.call(value, field)) result[field] = value[field]
    return result
  }, {})
}

function readerConfigSnapshot(state, name, configDefaultType) {
  return {
    ...sanitizeReaderScheme(state),
    name,
    configDefaultType,
  }
}

function defaultCustomConfigList() {
  return [
    {
      mode: 'page',
      pageMode: 'auto',
      clickMethod: 'auto',
      selectionAction: '操作弹窗',
      fontFamily: 'system',
      chineseFont: '简体',
      fontSize: 18,
      fontWeight: 400,
      fontColor: '#262626',
      theme: 'parchment',
      themeType: 'day',
      customBodyColor: '#eadfca',
      customPopupColor: '#ede7da',
      customBgColor: '#ffffff',
      customBgImage: '',
      brightness: 100,
      autoReadingMethod: '像素滚动',
      autoReadingPixel: 1,
      autoReadingLineTime: 1000,
      animateDuration: 300,
      lineHeight: 1.8,
      paragraphSpace: 0.2,
      columnWidth: 800,
      name: '内置白天',
      configDefaultType: '白天默认',
      builtin: true,
    },
    {
      mode: 'page',
      pageMode: 'auto',
      clickMethod: 'auto',
      selectionAction: '操作弹窗',
      fontFamily: 'system',
      chineseFont: '简体',
      fontSize: 18,
      fontWeight: 400,
      fontColor: '#ffffff',
      theme: 'dark',
      themeType: 'night',
      customBodyColor: '#121212',
      customPopupColor: '#121212',
      customBgColor: '#171717',
      customBgImage: '',
      brightness: 100,
      autoReadingMethod: '像素滚动',
      autoReadingPixel: 1,
      autoReadingLineTime: 1000,
      animateDuration: 300,
      lineHeight: 1.8,
      paragraphSpace: 0.2,
      columnWidth: 800,
      name: '内置黑夜',
      configDefaultType: '黑夜默认',
      builtin: true,
    },
  ]
}

function sanitizeCustomConfigList(value) {
  const source = Array.isArray(value) && value.length ? value : defaultCustomConfigList()
  return source
    .map((item, index) => {
      if (!item || typeof item !== 'object') return null
      const name = typeof item.name === 'string' && item.name.trim() ? item.name.trim() : ''
      if (!name) return null
      return {
        ...sanitizeReaderScheme(item),
        name,
        configDefaultType: typeof item.configDefaultType === 'string' ? item.configDefaultType : '',
        builtin: item.builtin === true || index <= 1 && ['内置白天', '内置黑夜'].includes(name),
      }
    })
    .filter(Boolean)
}

function readerSettingTimestampIsNewer(candidate, baseline) {
  const next = String(candidate || '').trim()
  const current = String(baseline || '').trim()
  if (!next) return false
  if (!current) return true
  const nextTime = Date.parse(next)
  const currentTime = Date.parse(current)
  if (Number.isFinite(nextTime) && Number.isFinite(currentTime)) {
    return nextTime > currentTime
  }
  return next > current
}

function readErrorMessage(err) {
  return err?.response?.data?.error || err?.message || '同步失败'
}
