export function useReaderAppearanceAssets(options) {
  function setTheme(theme) {
    options.reader.setTheme(theme)
  }

  function toggleNight() {
    const theme = options.reader.theme
    options.reader.setTheme(theme === 'dark' || theme === 'black' ? 'parchment' : 'dark')
  }

  async function pickBgImage(data) {
    const file = data?.raw || data?.file
    if (!file) return null
    try {
      const result = await options.upload({ file, type: 'background' })
      const url = result?.data?.url
      if (!url) throw new Error('上传结果缺少背景图地址')
      options.reader.addCustomBgImage(url)
      options.onSuccess?.('阅读背景图已上传')
      return url
    } catch (error) {
      options.onError?.(error, '上传背景图失败')
      return null
    }
  }

  async function clearBgImage(image) {
    if (!image) return false
    try {
      await options.removeAsset(image)
      options.reader.removeCustomBgImage(image)
      options.onSuccess?.('已删除阅读背景图')
      return true
    } catch (error) {
      options.onError?.(error, '删除背景图失败')
      return false
    }
  }

  async function pickFontFile({ file, font } = {}) {
    const rawFile = file?.raw || file?.file || file
    if (!rawFile || !font?.value) return null
    try {
      const result = await options.upload({ file: rawFile, type: 'font' })
      const url = result?.data?.url
      if (!url) throw new Error('上传结果缺少字体地址')
      options.reader.setCustomFont(font.value, url)
      options.reader.setFontFamily(font.value)
      options.syncFonts(options.reader.customFontsMap)
      options.onSuccess?.(`已上传${font.label}字体`)
      return url
    } catch (error) {
      options.onError?.(error, '上传字体失败')
      return null
    }
  }

  async function clearFontFile(font) {
    const url = options.reader.customFontsMap?.[font?.value]
    if (!url || !font?.value) return false
    try {
      await options.removeAsset(url)
      options.reader.clearCustomFont(font.value)
      options.syncFonts(options.reader.customFontsMap)
      options.onSuccess?.(`已恢复默认${font.label}字体`)
      return true
    } catch (error) {
      options.onError?.(error, '恢复默认字体失败')
      return false
    }
  }

  return {
    clearBgImage,
    clearFontFile,
    pickBgImage,
    pickFontFile,
    setTheme,
    toggleNight,
  }
}
