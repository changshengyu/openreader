import { defineStore } from 'pinia'
import api from '../api/client'
import { currentUserScope } from '../utils/authScope'
import { newestProgress as pickNewestProgress, progressUpdatedAt } from '../utils/bookOrder'

let readerSettingsSyncTimer
const READER_CLIENT_ID = readerClientId()

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
    selectionAction: '操作弹窗',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    fontColor: '',
    theme: 'parchment',
    customBodyColor: '',
    customPopupColor: '',
    customBgColor: '',
    customBgImage: '',
    customBgImageList: [],
    brightness: 100,
    autoReadSpeed: 12,
    autoReadingMethod: '像素滚动',
    autoReadingPixel: 12,
    autoReadingLineTime: 260,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: 11,
    settingsUpdatedAt: '',
    settingsSyncBaseUpdatedAt: '',
    settingsSyncing: false,
    settingsSyncError: '',
    progressByBook: {},
    clientId: READER_CLIENT_ID,
    normalModeSnapshot: null,
    customConfigName: '内置白天',
    customConfigList: defaultCustomConfigList(),
    autoTheme: false,
  }),
  persist: true,
  getters: {
    currentTheme(state) {
      if (state.theme === 'custom') {
        return {
          label: '自定义',
          bg: state.customBgColor || '#f4e9bd',
          text: state.fontColor || '#24282c',
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
    setMode(mode) {
      this.mode = ['scroll', 'scroll2', 'flip', 'page'].includes(mode) ? mode : 'page'
      this.markSettingsDirty()
    },
    setCustomConfig(name) {
      const config = (Array.isArray(this.customConfigList) ? this.customConfigList : []).find(item => item?.name === name)
      if (!config) return false
      const next = sanitizeReaderSettings(config, { includeCustomConfigs: false })
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
      const config = readerConfigSnapshot(this, cleanName, '')
      this.customConfigList = [...current, config]
      this.customConfigName = cleanName
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
      const type = isNight ? '黑夜默认' : '白天默认'
      const config = (Array.isArray(this.customConfigList) ? this.customConfigList : []).find(item => item?.configDefaultType === type)
      if (!config) return false
      return this.setCustomConfig(config.name)
    },
    setPageType(pageType) {
      const nextType = ['kindle', 'simple', 'Kindle'].includes(pageType) ? 'kindle' : 'normal'
      if (nextType === this.pageType) return
      if (nextType === 'kindle') {
        this.normalModeSnapshot = {
          pageMode: this.pageMode,
          animateDuration: this.animateDuration,
          mode: this.mode,
          fontSize: this.fontSize,
          theme: this.theme,
          customBodyColor: this.customBodyColor,
          customPopupColor: this.customPopupColor,
          fontColor: this.fontColor,
          clickMethod: this.clickMethod,
          selectionAction: this.selectionAction,
        }
        this.pageType = 'kindle'
        this.animateDuration = 0
        this.pageMode = 'mobile'
        this.mode = 'flip'
        this.fontSize = Math.min(this.fontSize, 20)
        this.theme = 'white'
        this.fontColor = ''
        this.clickMethod = 'none'
        this.selectionAction = '忽略'
      } else {
        const snapshot = this.normalModeSnapshot || {}
        this.pageType = 'normal'
        this.pageMode = snapshot.pageMode === 'mobile' ? 'mobile' : 'auto'
        this.animateDuration = clampNumber(snapshot.animateDuration, 0, 1000, 300)
        if (['scroll', 'scroll2', 'flip', 'page'].includes(snapshot.mode)) this.mode = snapshot.mode
        if (snapshot.fontSize !== undefined) this.fontSize = clampNumber(snapshot.fontSize, 8, 36, 18)
        if (typeof snapshot.theme === 'string') this.theme = snapshot.theme
        if (typeof snapshot.customBodyColor === 'string') this.customBodyColor = snapshot.customBodyColor
        if (typeof snapshot.customPopupColor === 'string') this.customPopupColor = snapshot.customPopupColor
        if (typeof snapshot.fontColor === 'string') this.fontColor = snapshot.fontColor
        if (['next', 'auto', 'none'].includes(snapshot.clickMethod)) this.clickMethod = snapshot.clickMethod
        if (['操作弹窗', '忽略'].includes(snapshot.selectionAction)) this.selectionAction = snapshot.selectionAction
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
    setSelectionAction(action) {
      this.selectionAction = ['操作弹窗', '忽略'].includes(action) ? action : '操作弹窗'
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
    setFontColor(fontColor) {
      this.fontColor = typeof fontColor === 'string' ? fontColor : ''
      this.markSettingsDirty()
    },
    setTheme(theme) {
      this.theme = theme
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
      if (this.customBgImage === image) this.customBgImage = ''
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
      this.autoReadingPixel = clampNumber(pixel, 1, 80, 12)
      this.autoReadSpeed = this.autoReadingPixel
      this.markSettingsDirty()
    },
    setAutoReadingLineTime(lineTime) {
      this.autoReadingLineTime = clampNumber(lineTime, 50, 3000, 260)
      this.markSettingsDirty()
    },
    setAnimateDuration(duration) {
      this.animateDuration = this.pageType === 'kindle' ? 0 : clampNumber(duration, 0, 1000, 300)
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
      if (this.pageType === 'simple' || this.pageType === 'Kindle') this.pageType = 'kindle'
      if (!['normal', 'kindle'].includes(this.pageType)) this.pageType = 'normal'
      if (!['auto', 'mobile'].includes(this.pageMode)) this.pageMode = 'auto'
      if (!['next', 'auto', 'none'].includes(this.clickMethod)) this.clickMethod = 'auto'
      if (!['操作弹窗', '忽略'].includes(this.selectionAction)) this.selectionAction = '操作弹窗'
      if (!['system', 'serif', 'kai', 'mono'].includes(this.fontFamily)) this.fontFamily = 'system'
      if (!['简体', '繁体'].includes(this.chineseFont)) this.chineseFont = '简体'
      if (!this.customFontsMap || typeof this.customFontsMap !== 'object' || Array.isArray(this.customFontsMap)) this.customFontsMap = {}
      if (!Array.isArray(this.customBgImageList)) this.customBgImageList = []
      if (!Array.isArray(this.customConfigList) || !this.customConfigList.length) this.customConfigList = defaultCustomConfigList()
      this.customConfigList = sanitizeCustomConfigList(this.customConfigList)
      if (!this.customConfigList.some(item => item.name === this.customConfigName)) {
        this.customConfigName = this.customConfigList[0]?.name || '内置白天'
      }
      this.autoTheme = this.autoTheme === true
      this.fontSize = clampNumber(this.fontSize, 8, 36, 18)
      this.fontWeight = clampNumber(this.fontWeight, 300, 900, 400)
      if (typeof this.fontColor !== 'string') this.fontColor = ''
      if (typeof this.customBodyColor !== 'string') this.customBodyColor = ''
      if (typeof this.customPopupColor !== 'string') this.customPopupColor = ''
      this.lineHeight = clampNumber(this.lineHeight, 1, 5, 1.8)
      this.paragraphSpace = clampNumber(this.paragraphSpace, 0, 3, 0)
      this.columnWidth = clampNumber(this.columnWidth, 320, 1200, 800)
      this.brightness = clampNumber(this.brightness, 50, 150, 100)
      if (!['像素滚动', '段落滚动'].includes(this.autoReadingMethod)) this.autoReadingMethod = '像素滚动'
      this.autoReadingPixel = clampNumber(this.autoReadingPixel ?? this.autoReadSpeed, 1, 80, 12)
      this.autoReadSpeed = this.autoReadingPixel
      this.autoReadingLineTime = clampNumber(this.autoReadingLineTime, 50, 3000, 260)
      this.animateDuration = clampNumber(this.animateDuration, 0, 1000, 300)
      if (this.pageType === 'kindle') {
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
      this.settingsVersion = 11
      this.settingsSyncing = false
    },
    markSettingsDirty(options = {}) {
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
      if (local?.bookId && progressUpdatedAt(local) > progressUpdatedAt(progress)) {
        if (local.pendingSync) this.syncLocalProgress(local, local.baseUpdatedAt || progress.updatedAt || '').catch(() => {})
        return local
      }
      this.replaceProgress(progress)
      return progress
    },
    cachedProgress(bookId) {
      if (!bookId) return null
      return newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
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
        clientId: this.ensureClientId(),
      })
      const merged = mergeProgressResponse(response.data, optimistic)
      if (isProgressConflict(response) && shouldRetryProgressConflict(optimistic, merged)) {
        const retried = await this.syncLocalProgress(optimistic, merged?.updatedAt || optimistic.baseUpdatedAt || '', { force: true })
        if (retried?.bookId) return retried
      }
      this.replaceProgress(merged)
      return merged
    },
    async loadProgress(bookId, options = {}) {
      const local = newestProgress(this.progressByBook[bookId], readLocalChapterProgress(bookId))
      if (options.preferLocal && local?.bookId && local.pendingSync) {
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
    async syncLocalProgress(progress, baseUpdatedAt = '', options = {}) {
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
          clientId: this.ensureClientId(),
        })
        const next = mergeProgressResponse(response.data, progress)
        if (isProgressConflict(response) && shouldRetryProgressConflict(progress, next) && !options.force) {
          return await this.syncLocalProgress(progress, next?.updatedAt || progress.baseUpdatedAt || '', { force: true })
        }
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
    clickMethod: state.clickMethod,
    selectionAction: state.selectionAction,
    fontFamily: state.fontFamily,
    customFontsMap: state.customFontsMap || {},
    chineseFont: state.chineseFont,
    fontSize: state.fontSize,
    fontWeight: state.fontWeight,
    fontColor: state.fontColor || '',
    theme: state.theme,
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
    settingsVersion: 11,
  }
}

function defaultReaderSettings() {
  return {
    mode: 'page',
    pageType: 'normal',
    clickMethod: 'auto',
    selectionAction: '操作弹窗',
    fontFamily: 'system',
    customFontsMap: {},
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    fontColor: '',
    theme: 'parchment',
    customBodyColor: '',
    customPopupColor: '',
    customBgColor: '',
    customBgImage: '',
    customBgImageList: [],
    customConfigName: '内置白天',
    customConfigList: defaultCustomConfigList(),
    autoTheme: false,
    brightness: 100,
    autoReadSpeed: 12,
    autoReadingMethod: '像素滚动',
    autoReadingPixel: 12,
    autoReadingLineTime: 260,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: 11,
    normalModeSnapshot: null,
  }
}

function sanitizeReaderSettings(payload, options = {}) {
  const includeCustomConfigs = options.includeCustomConfigs !== false
  const settings = {}
  if (['scroll', 'scroll2', 'flip', 'page'].includes(payload.mode)) settings.mode = payload.mode
  if (['normal', 'kindle'].includes(payload.pageType)) settings.pageType = payload.pageType
  if (payload.pageType === 'simple' || payload.pageType === 'Kindle') settings.pageType = 'kindle'
  if (['next', 'auto', 'none'].includes(payload.clickMethod)) settings.clickMethod = payload.clickMethod
  if (['操作弹窗', '忽略'].includes(payload.selectionAction)) settings.selectionAction = payload.selectionAction
  if (['system', 'serif', 'kai', 'mono'].includes(payload.fontFamily)) settings.fontFamily = payload.fontFamily
  settings.customFontsMap = sanitizeCustomFontsMap(payload.customFontsMap)
  settings.chineseFont = payload.chineseFont === '繁体' ? '繁体' : '简体'
  if (typeof payload.theme === 'string') settings.theme = payload.theme
  settings.customBodyColor = typeof payload.customBodyColor === 'string' ? payload.customBodyColor : ''
  settings.customPopupColor = typeof payload.customPopupColor === 'string' ? payload.customPopupColor : ''
  if (typeof payload.customBgColor === 'string') settings.customBgColor = payload.customBgColor
  if (typeof payload.customBgImage === 'string') settings.customBgImage = payload.customBgImage
  settings.customBgImageList = sanitizeStringList(payload.customBgImageList)
  if (includeCustomConfigs) {
    if (typeof payload.customConfigName === 'string') settings.customConfigName = payload.customConfigName
    settings.customConfigList = sanitizeCustomConfigList(payload.customConfigList)
    settings.autoTheme = payload.autoTheme === true
  }
  if (typeof payload.ttsVoiceURI === 'string') settings.ttsVoiceURI = payload.ttsVoiceURI
  settings.fontSize = clampNumber(payload.fontSize, 8, 36, 18)
  settings.fontWeight = clampNumber(payload.fontWeight, 300, 900, 400)
  settings.fontColor = typeof payload.fontColor === 'string' ? payload.fontColor : ''
  settings.brightness = clampNumber(payload.brightness, 50, 150, 100)
  settings.autoReadingMethod = payload.autoReadingMethod === '段落滚动' ? '段落滚动' : '像素滚动'
  settings.autoReadingPixel = clampNumber(payload.autoReadingPixel ?? payload.autoReadSpeed, 1, 80, 12)
  settings.autoReadSpeed = settings.autoReadingPixel
  settings.autoReadingLineTime = clampNumber(payload.autoReadingLineTime, 50, 3000, 260)
  settings.animateDuration = clampNumber(payload.animateDuration, 0, 1000, 300)
  settings.ttsRate = clampNumber(payload.ttsRate, 0.5, 3, 1)
  settings.ttsPitch = clampNumber(payload.ttsPitch, 0.5, 2, 1)
  settings.lineHeight = clampNumber(payload.lineHeight, 1, 5, 1.8)
  settings.paragraphSpace = clampNumber(payload.paragraphSpace, 0, 3, 0.2)
  settings.columnWidth = clampNumber(payload.columnWidth, 320, 1200, 800)
  settings.settingsVersion = 11
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

function readerConfigSnapshot(state, name, configDefaultType) {
  const payload = readerSettingsPayload({
    ...state,
    customConfigName: name,
    customConfigList: [],
  })
  delete payload.customConfigName
  delete payload.customConfigList
  return {
    ...payload,
    name,
    configDefaultType,
  }
}

function defaultCustomConfigList() {
  return [
    {
      mode: 'page',
      pageType: 'normal',
      clickMethod: 'auto',
      selectionAction: '操作弹窗',
      fontFamily: 'system',
      customFontsMap: {},
      chineseFont: '简体',
      fontSize: 18,
      fontWeight: 400,
      fontColor: '',
      theme: 'parchment',
      customBodyColor: '',
      customPopupColor: '',
      customBgColor: '',
      customBgImage: '',
      customBgImageList: [],
      customConfigName: '内置白天',
      customConfigList: [],
      brightness: 100,
      autoReadSpeed: 12,
      autoReadingMethod: '像素滚动',
      autoReadingPixel: 12,
      autoReadingLineTime: 260,
      animateDuration: 300,
      ttsRate: 1,
      ttsPitch: 1,
      ttsVoiceURI: '',
      lineHeight: 1.8,
      paragraphSpace: 0.2,
      columnWidth: 800,
      settingsVersion: 11,
      name: '内置白天',
      configDefaultType: '白天默认',
      builtin: true,
    },
    {
      mode: 'page',
      pageType: 'normal',
      clickMethod: 'auto',
      selectionAction: '操作弹窗',
      fontFamily: 'system',
      customFontsMap: {},
      chineseFont: '简体',
      fontSize: 18,
      fontWeight: 400,
      fontColor: '',
      theme: 'dark',
      customBodyColor: '',
      customPopupColor: '',
      customBgColor: '',
      customBgImage: '',
      customBgImageList: [],
      customConfigName: '内置黑夜',
      customConfigList: [],
      brightness: 100,
      autoReadSpeed: 12,
      autoReadingMethod: '像素滚动',
      autoReadingPixel: 12,
      autoReadingLineTime: 260,
      animateDuration: 300,
      ttsRate: 1,
      ttsPitch: 1,
      ttsVoiceURI: '',
      lineHeight: 1.8,
      paragraphSpace: 0.2,
      columnWidth: 800,
      settingsVersion: 11,
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
        ...sanitizeReaderSettings(item, { includeCustomConfigs: false }),
        name,
        configDefaultType: typeof item.configDefaultType === 'string' ? item.configDefaultType : '',
        builtin: item.builtin === true || index <= 1 && ['内置白天', '内置黑夜'].includes(name),
      }
    })
    .filter(Boolean)
}

function readErrorMessage(err) {
  return err?.response?.data?.error || err?.message || '同步失败'
}
