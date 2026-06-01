import { defineStore } from 'pinia'
import api from '../api/client'
import { newestProgress as pickNewestProgress, progressUpdatedAt } from '../utils/bookOrder'

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
    clickMethod: 'auto',
    fontFamily: 'system',
    fontSize: 18,
    fontWeight: 400,
    theme: 'parchment',
    customBgColor: '',
    customBgImage: '',
    brightness: 100,
    autoReadSpeed: 12,
    animateDuration: 300,
    ttsRate: 1,
    ttsPitch: 1,
    ttsVoiceURI: '',
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    settingsVersion: 6,
    progressByBook: {},
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
    },
    setClickMethod(method) {
      this.clickMethod = ['next', 'auto', 'none'].includes(method) ? method : 'auto'
    },
    setFontFamily(fontFamily) {
      this.fontFamily = ['system', 'serif', 'kai', 'mono'].includes(fontFamily) ? fontFamily : 'system'
    },
    setFontSize(fontSize) {
      this.fontSize = Math.max(8, Math.min(36, Number(fontSize) || 18))
    },
    setFontWeight(fontWeight) {
      this.fontWeight = Math.max(300, Math.min(900, Number(fontWeight) || 400))
    },
    setTheme(theme) {
      this.theme = theme
    },
    setCustomBgColor(color) {
      this.customBgColor = color
    },
    setCustomBgImage(image) {
      this.customBgImage = image
    },
    setBrightness(brightness) {
      this.brightness = Math.max(50, Math.min(150, Number(brightness) || 100))
    },
    setAutoReadSpeed(speed) {
      this.autoReadSpeed = Math.max(2, Math.min(40, Number(speed) || 12))
    },
    setAnimateDuration(duration) {
      const value = Number(duration)
      this.animateDuration = Math.max(0, Math.min(1000, Number.isFinite(value) ? value : 300))
    },
    setTTSRate(rate) {
      this.ttsRate = Math.max(0.5, Math.min(3, Number(rate) || 1))
    },
    setTTSPitch(pitch) {
      this.ttsPitch = Math.max(0.5, Math.min(2, Number(pitch) || 1))
    },
    setTTSVoice(uri) {
      this.ttsVoiceURI = uri || ''
    },
    setLineHeight(lineHeight) {
      this.lineHeight = Math.max(1, Math.min(5, Number(lineHeight) || 1.8))
    },
    setParagraphSpace(paragraphSpace) {
      this.paragraphSpace = Math.max(0, Math.min(3, Number(paragraphSpace) || 0))
    },
    setColumnWidth(columnWidth) {
      this.columnWidth = Math.max(320, Math.min(1200, Number(columnWidth) || 800))
    },
    normalizeSettings() {
      if (!['scroll', 'scroll2', 'flip', 'page'].includes(this.mode)) this.mode = 'page'
      if (!['next', 'auto', 'none'].includes(this.clickMethod)) this.clickMethod = 'auto'
      if (!['system', 'serif', 'kai', 'mono'].includes(this.fontFamily)) this.fontFamily = 'system'
      this.setFontSize(this.fontSize)
      this.setFontWeight(this.fontWeight)
      this.setLineHeight(this.lineHeight)
      this.setParagraphSpace(this.paragraphSpace)
      this.setColumnWidth(this.columnWidth)
      this.setBrightness(this.brightness)
      this.setAutoReadSpeed(this.autoReadSpeed)
      this.setAnimateDuration(this.animateDuration)
      this.setTTSRate(this.ttsRate)
      this.setTTSPitch(this.ttsPitch)
      if ((this.settingsVersion || 0) < 4) {
        this.fontSize = 18
        this.fontWeight = 400
        this.lineHeight = 1.8
        this.paragraphSpace = 0.2
        this.columnWidth = 800
      }
      this.settingsVersion = 6
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
      const optimistic = {
        ...payload,
        mode: this.mode,
        updatedAt: new Date().toISOString(),
        pendingSync: true,
        baseUpdatedAt: payload.baseUpdatedAt || this.progressByBook[payload.bookId]?.updatedAt || '',
      }
      this.applyProgress(optimistic)
      const response = await api.put('/progress', { ...payload, mode: this.mode })
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

function localChapterProgressKey(bookId) {
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
    const raw = localStorage.getItem(localChapterProgressKey(bookId))
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
