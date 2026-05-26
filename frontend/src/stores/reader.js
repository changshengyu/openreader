import { defineStore } from 'pinia'
import api from '../api/client'

export const themePresets = {
  parchment: { label: '羊皮纸', bg: '#f4e9bd', text: '#24282c' },
  white:    { label: '纯白',   bg: '#ffffff', text: '#1f2933' },
  green:    { label: '护眼绿', bg: '#c8dcc8', text: '#1f2933' },
  dark:     { label: '深色',   bg: '#2d2d2d', text: '#d8d4c8' },
  black:    { label: '纯黑',   bg: '#000000', text: '#aaaaaa' },
}

export const useReaderStore = defineStore('reader', {
  state: () => ({
    mode: 'scroll',
    clickMethod: 'auto',
    fontFamily: 'system',
    fontSize: 18,
    fontWeight: 400,
    theme: 'parchment',
    customBgColor: '',
    customBgImage: '',
    brightness: 100,
    autoReadSpeed: 12,
    animateDuration: 180,
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
      this.mode = ['scroll', 'scroll2', 'flip', 'page'].includes(mode) ? mode : 'scroll'
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
      this.animateDuration = Math.max(0, Math.min(1000, Number.isFinite(value) ? value : 180))
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
      this.columnWidth = Math.max(320, Math.min(1200, Number(columnWidth) || 670))
    },
    normalizeSettings() {
      if (!['scroll', 'scroll2', 'flip', 'page'].includes(this.mode)) this.mode = 'scroll'
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
      this.progressByBook[progress.bookId] = progress
    },
    async saveProgress(payload) {
      const optimistic = {
        ...payload,
        mode: this.mode,
        updatedAt: new Date().toISOString(),
      }
      this.applyProgress(optimistic)
      const { data } = await api.put('/progress', { ...payload, mode: this.mode })
      this.applyProgress(data)
      return data
    },
    async loadProgress(bookId) {
      const local = this.progressByBook[bookId]
      const { data } = await api.get(`/progress/${bookId}`)
      if (data?.bookId) {
        if (progressUpdatedAt(local) > progressUpdatedAt(data)) {
          return local
        }
        this.applyProgress(data)
        return data
      }
      return local || data
    },
  },
})

function progressUpdatedAt(progress) {
  if (!progress?.updatedAt) return 0
  const timestamp = Date.parse(progress.updatedAt)
  return Number.isFinite(timestamp) ? timestamp : 0
}
