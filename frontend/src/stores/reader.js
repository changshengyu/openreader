import { defineStore } from 'pinia'
import api from '../api/client'
import { currentUserScope } from '../utils/authScope'
import { newestProgress as pickNewestProgress, progressUpdatedAt } from '../utils/bookOrder'

let readerSettingsSyncTimer

export const themePresets = {
  parchment: { label: '羊皮纸', bg: '#f4e9bd', text: '#24282c' },
  white:    { label: '纯白',   bg: '#ffffff', text: '#1f2933' },
  green:    { label: '护眼绿', bg: '#c8dcc8', text: '#1f2933' },
  dark:     { label: '深色',   bg: '#2d2d2d', text: '#d8d4c8' },
  black:    { label: '纯黑',   bg: '#000000', text: '#aaaaaa' },
}

export const useReaderStore = defineStore('reader', {
  state: () => ({
    mode: 'page',
    pageType: 'normal',
    pageMode: 'auto',
    clickMethod: 'auto',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    theme: 'parchment',
    customBgColor: '',
    customBgImage: '',
    customBgImageList: [],
    brightness: 100,
    autoReadSpeed: 12,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: 7,
    settingsUpdatedAt: '',
    settingsSyncBaseUpdatedAt: '',
    settingsSyncing: false,
    settingsSyncError: '',
    progressByBook: {},
    normalModeSnapshot: null,
  }),
  persist: true,
  getters: {
    currentTheme(state) {
      if (state.theme === 'custom') {
        return {
          label: '自定义',
          bg: state.customBgColor || '#f4e9bd',
          text: '#24282c',
        }
      }
      return themePresets[state.theme] || themePresets.parchment
    },
  },
  actions: {
    setMode(mode) {
      this.mode = ['scroll', 'scroll2', 'flip', 'page'].includes(mode) ? mode : 'page'
      this.markSettingsDirty()
    },
    setPageType(pageType) {
      const nextType = pageType === 'simple' ? 'simple' : 'normal'
      if (nextType === this.pageType) return
      if (nextType === 'simple') {
        this.normalModeSnapshot = {
          pageMode: this.pageMode,
          animateDuration: this.animateDuration,
          mode: this.mode,
          fontSize: this.fontSize,
          theme: this.theme,
        }
        this.pageType = 'simple'
        this.animateDuration = 0
        this.pageMode = 'mobile'
        this.mode = 'flip'
        this.fontSize = Math.min(this.fontSize, 20)
        this.theme = 'white'
      } else {
        const snapshot = this.normalModeSnapshot || {}
        this.pageType = 'normal'
        this.pageMode = snapshot.pageMode === 'mobile' ? 'mobile' : 'auto'
        this.animateDuration = clampNumber(snapshot.animateDuration, 0, 1000, 300)
        if (['scroll', 'scroll2', 'flip', 'page'].includes(snapshot.mode)) this.mode = snapshot.mode
        if (snapshot.fontSize !== undefined) this.fontSize = clampNumber(snapshot.fontSize, 8, 36, 18)
        if (typeof snapshot.theme === 'string') this.theme = snapshot.theme
        this.normalModeSnapshot = null
      }
      this.markSettingsDirty()
    },
    setPageMode(pageMode) {
      this.pageMode = pageMode === 'mobile' ? 'mobile' : 'auto'
    },
    setClickMethod(method) {
      this.clickMethod = ['next', 'auto', 'none'].includes(method) ? method : 'auto'
      this.markSettingsDirty()
    },
    setFontFamily(fontFamily) {
      this.fontFamily = ['system', 'serif', 'kai', 'mono'].includes(fontFamily) ? fontFamily : 'system'
      this.markSettingsDirty()
    },
    setChineseFont(chineseFont) {
      this.chineseFont = chineseFont === '繁体' ? '繁体' : '简体'
      this.markSettingsDirty()
    },
    setCustomFont(fontFamily, url) {
      if (!['system', 'serif', 'kai', 'mono'].includes(fontFamily) || !url) return
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
      this.fontSize = clampNumber(fontSize, 8, 36, 18)
      this.markSettingsDirty()
    },
    setFontWeight(fontWeight) {
      this.fontWeight = clampNumber(fontWeight, 300, 900, 400)
      this.markSettingsDirty()
    },
    setTheme(theme) {
      this.theme = theme
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
      if (this.customBgImage === image) this.customBgImage = ''
      this.markSettingsDirty()
    },
    setBrightness(brightness) {
      this.brightness = clampNumber(brightness, 50, 150, 100)
      this.markSettingsDirty()
    },
    setAutoReadSpeed(speed) {
      this.autoReadSpeed = clampNumber(speed, 2, 40, 12)
      this.markSettingsDirty()
    },
    setAnimateDuration(duration) {
      this.animateDuration = this.pageType === 'simple' ? 0 : clampNumber(duration, 0, 1000, 300)
      this.markSettingsDirty()
    },
    setTTSRate(rate) {
      this.ttsRate = clampNumber(rate, 0.5, 3, 1)
      this.markSettingsDirty()
    },
    setTTSPitch(pitch) {
      this.ttsPitch = clampNumber(pitch, 0.5, 2, 1)
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
      this.paragraphSpace = clampNumber(paragraphSpace, 0, 3, 0)
      this.markSettingsDirty()
    },
    setColumnWidth(columnWidth) {
      this.columnWidth = clampNumber(columnWidth, 320, 1200, 800)
      this.markSettingsDirty()
    },
    resetReaderSettings() {
      Object.assign(this, defaultReaderSettings())
      this.markSettingsDirty()
    },
    normalizeSettings() {
      if (!['scroll', 'scroll2', 'flip', 'page'].includes(this.mode)) this.mode = 'page'
      if (!['normal', 'simple'].includes(this.pageType)) this.pageType = 'normal'
      if (!['auto', 'mobile'].includes(this.pageMode)) this.pageMode = 'auto'
      if (!['next', 'auto', 'none'].includes(this.clickMethod)) this.clickMethod = 'auto'
      if (!['system', 'serif', 'kai', 'mono'].includes(this.fontFamily)) this.fontFamily = 'system'
      if (!['简体', '繁体'].includes(this.chineseFont)) this.chineseFont = '简体'
      if (!this.customFontsMap || typeof this.customFontsMap !== 'object' || Array.isArray(this.customFontsMap)) this.customFontsMap = {}
      if (!Array.isArray(this.customBgImageList)) this.customBgImageList = []
      this.fontSize = clampNumber(this.fontSize, 8, 36, 18)
      this.fontWeight = clampNumber(this.fontWeight, 300, 900, 400)
      this.lineHeight = clampNumber(this.lineHeight, 1, 5, 1.8)
      this.paragraphSpace = clampNumber(this.paragraphSpace, 0, 3, 0)
      this.columnWidth = clampNumber(this.columnWidth, 320, 1200, 800)
      this.brightness = clampNumber(this.brightness, 50, 150, 100)
      this.autoReadSpeed = clampNumber(this.autoReadSpeed, 2, 40, 12)
      this.animateDuration = clampNumber(this.animateDuration, 0, 1000, 300)
      if (this.pageType === 'simple') {
        this.animateDuration = 0
      }
      this.ttsRate = clampNumber(this.ttsRate, 0.5, 3, 1)
      this.ttsPitch = clampNumber(this.ttsPitch, 0.5, 2, 1)
      if ((this.settingsVersion || 0) < 4) {
        this.fontSize = 18
        this.fontWeight = 400
        this.lineHeight = 1.8
        this.paragraphSpace = 0.2
        this.columnWidth = 800
      }
      this.settingsVersion = 7
      this.settingsSyncing = false
    },
    markSettingsDirty() {
      this.settingsUpdatedAt = new Date().toISOString()
      this.settingsSyncError = ''
      this.scheduleSettingsSync()
    },
    scheduleSettingsSync() {
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return
      clearTimeout(readerSettingsSyncTimer)
      readerSettingsSyncTimer = setTimeout(() => {
        this.saveReaderSettings().catch(() => {})
      }, 700)
    },
    applyReaderSettings(payload, updatedAt = '') {
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
    async loadReaderSettings() {
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return null
      try {
        const { data } = await api.get('/settings/reader')
        const serverUpdatedAt = data?.updatedAt || ''
        if (data?.value && typeof data.value === 'object') {
          if (this.settingsUpdatedAt && serverUpdatedAt && this.settingsUpdatedAt > serverUpdatedAt && this.settingsSyncBaseUpdatedAt !== serverUpdatedAt) {
            await this.saveReaderSettings()
            return readerSettingsPayload(this)
          }
          this.applyReaderSettings(data.value, serverUpdatedAt)
          return data.value
        }
        await this.saveReaderSettings()
        return readerSettingsPayload(this)
      } catch (err) {
        this.settingsSyncError = readErrorMessage(err)
        return null
      }
    },
    async saveReaderSettings() {
      if (typeof localStorage === 'undefined' || !localStorage.getItem('openreader_token')) return null
      clearTimeout(readerSettingsSyncTimer)
      this.settingsSyncing = true
      this.settingsSyncError = ''
      try {
        const { data, headers } = await api.put('/settings/reader', {
          value: readerSettingsPayload(this),
          baseUpdatedAt: this.settingsSyncBaseUpdatedAt || '',
        })
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
        this.settingsSyncError = readErrorMessage(err)
        return null
      } finally {
        this.settingsSyncing = false
      }
    },
    applyProgress(progress) {
      if (!progress?.bookId) return
      const current = pickNewestProgress(this.progressByBook[progress.bookId], readLocalChapterProgress(progress.bookId))
      const next = pickNewestProgress(current, progress)
      if (!next) return
      this.progressByBook[progress.bookId] = next
      persistLocalChapterProgress(next)
    },
    applyServerProgress(progress) {
      if (!progress?.bookId) return null
      const local = newestProgress(this.progressByBook[progress.bookId], readLocalChapterProgress(progress.bookId))
      if (local?.pendingSync && progressUpdatedAt(local) > progressUpdatedAt(progress)) {
        this.syncLocalProgress(local, local.baseUpdatedAt || progress.updatedAt || '').catch(() => {})
        return local
      }
      this.replaceProgress(progress)
      return progress
    },
    replaceProgress(progress) {
      if (!progress?.bookId) return
      const next = clearLocalProgressFlags(progress)
      this.progressByBook[progress.bookId] = next
      persistLocalChapterProgress(next)
    },
    async saveProgress(payload) {
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
      })
      const { data } = response
      const merged = data?.bookId ? {
        ...data,
        chapterPercent: Number.isFinite(Number(data.chapterPercent))
          ? Number(data.chapterPercent)
          : optimistic.chapterPercent,
        chapterTitle: data.chapterTitle || optimistic.chapterTitle,
      } : data
      this.replaceProgress(merged)
      return merged
    },
    async loadProgress(bookId, options = {}) {
      const local = newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
      if (options.preferLocal && local?.bookId) {
        api.get(`/progress/${bookId}`)
          .then(({ data }) => {
            if (data?.bookId) this.applyServerProgress(data)
          })
          .catch(() => {})
        return local
      }
      let data = null
      try {
        const res = await api.get(`/progress/${bookId}`)
        data = res.data
      } catch {
        return local || null
      }
      if (data?.bookId) {
        if (local?.pendingSync && progressUpdatedAt(local) > progressUpdatedAt(data)) {
          this.syncLocalProgress(local, local.baseUpdatedAt || data.updatedAt)
          return local
        }
        this.replaceProgress(data)
        return data
      }
      if (local?.bookId && local.pendingSync) this.syncLocalProgress(local, local.baseUpdatedAt || data?.updatedAt)
      return local || data
    },
    async syncLocalProgress(progress, baseUpdatedAt = '') {
      if (!progress?.bookId) return null
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
        })
        const { data } = response
        const next = data?.bookId ? {
          ...data,
          chapterPercent: Number.isFinite(Number(data.chapterPercent))
            ? Number(data.chapterPercent)
            : progress.chapterPercent,
          chapterTitle: data.chapterTitle || progress.chapterTitle,
        } : data
        this.replaceProgress(next)
        return next
      } catch {
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
    const raw = localStorage.getItem(localChapterProgressKey(bookId)) || localStorage.getItem(legacyLocalChapterProgressKey(bookId))
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
    return progress
  } catch {
    return null
  }
}

function clampNumber(value, min, max, fallback) {
  const number = Number(value)
  return Math.max(min, Math.min(max, Number.isFinite(number) ? number : fallback))
}

function readerSettingsPayload(state) {
  return {
    mode: state.mode,
    pageType: state.pageType,
    clickMethod: state.clickMethod,
    fontFamily: state.fontFamily,
    customFontsMap: state.customFontsMap || {},
    chineseFont: state.chineseFont,
    fontSize: state.fontSize,
    fontWeight: state.fontWeight,
    theme: state.theme,
    customBgColor: state.customBgColor,
    customBgImage: state.customBgImage,
    customBgImageList: Array.isArray(state.customBgImageList) ? state.customBgImageList : [],
    brightness: state.brightness,
    autoReadSpeed: state.autoReadSpeed,
    animateDuration: state.animateDuration,
    ttsRate: state.ttsRate,
    ttsPitch: state.ttsPitch,
    ttsVoiceURI: state.ttsVoiceURI,
    lineHeight: state.lineHeight,
    paragraphSpace: state.paragraphSpace,
    columnWidth: state.columnWidth,
    settingsVersion: 7,
  }
}

function defaultReaderSettings() {
  return {
    mode: 'page',
    pageType: 'normal',
    clickMethod: 'auto',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    theme: 'parchment',
    customBgColor: '',
    customBgImage: '',
    customBgImageList: [],
    brightness: 100,
    autoReadSpeed: 12,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: 7,
    normalModeSnapshot: null,
  }
}

function sanitizeReaderSettings(payload) {
  const settings = {}
  if (['scroll', 'scroll2', 'flip', 'page'].includes(payload.mode)) settings.mode = payload.mode
  if (['normal', 'simple'].includes(payload.pageType)) settings.pageType = payload.pageType
  if (['next', 'auto', 'none'].includes(payload.clickMethod)) settings.clickMethod = payload.clickMethod
  if (['system', 'serif', 'kai', 'mono'].includes(payload.fontFamily)) settings.fontFamily = payload.fontFamily
  settings.customFontsMap = sanitizeCustomFontsMap(payload.customFontsMap)
  settings.chineseFont = payload.chineseFont === '繁体' ? '繁体' : '简体'
  if (typeof payload.theme === 'string') settings.theme = payload.theme
  if (typeof payload.customBgColor === 'string') settings.customBgColor = payload.customBgColor
  if (typeof payload.customBgImage === 'string') settings.customBgImage = payload.customBgImage
  settings.customBgImageList = sanitizeStringList(payload.customBgImageList)
  if (typeof payload.ttsVoiceURI === 'string') settings.ttsVoiceURI = payload.ttsVoiceURI
  settings.fontSize = clampNumber(payload.fontSize, 8, 36, 18)
  settings.fontWeight = clampNumber(payload.fontWeight, 300, 900, 400)
  settings.brightness = clampNumber(payload.brightness, 50, 150, 100)
  settings.autoReadSpeed = clampNumber(payload.autoReadSpeed, 2, 40, 12)
  settings.animateDuration = clampNumber(payload.animateDuration, 0, 1000, 300)
  settings.ttsRate = clampNumber(payload.ttsRate, 0.5, 3, 1)
  settings.ttsPitch = clampNumber(payload.ttsPitch, 0.5, 2, 1)
  settings.lineHeight = clampNumber(payload.lineHeight, 1, 5, 1.8)
  settings.paragraphSpace = clampNumber(payload.paragraphSpace, 0, 3, 0.2)
  settings.columnWidth = clampNumber(payload.columnWidth, 320, 1200, 800)
  settings.settingsVersion = 7
  return settings
}

function sanitizeCustomFontsMap(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return ['system', 'serif', 'kai', 'mono'].reduce((map, key) => {
    if (typeof value[key] === 'string' && value[key]) map[key] = value[key]
    return map
  }, {})
}

function sanitizeStringList(value) {
  if (!Array.isArray(value)) return []
  return [...new Set(value.filter(item => typeof item === 'string' && item))]
}

function readErrorMessage(err) {
  return err?.response?.data?.error || err?.message || '同步失败'
}
