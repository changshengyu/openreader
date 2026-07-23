export function useReaderAppearanceAssets(options) {
  function setTheme(theme) {
    options.reader.setTheme(theme)
  }

  function toggleNight() {
    options.reader.setTheme(options.reader.themeType === 'night' ? 'parchment' : 'dark')
  }

  async function pickBgImage(data) {
    const file = data?.raw || data?.file
    if (!file) return null
    const previous = snapshotAppearance(options.reader)
    let url = ''
    let persisted = false
    try {
      const result = await options.upload({ file, type: 'background' })
      url = result?.data?.url
      if (!url) throw new Error('上传结果缺少背景图地址')
      options.reader.addCustomBgImage(url)
      const saved = await persistAssetReference(value => (
        value?.customBgImage === url
        && Array.isArray(value?.customBgImageList)
        && value.customBgImageList.includes(url)
      ))
      if (!saved) throw new Error('阅读设置同步失败')
      persisted = true
      options.onSuccess?.('阅读背景图已上传')
      return url
    } catch (error) {
      if (url && !persisted) {
        if (backgroundAttemptIsCurrent(options.reader, url)) {
          restoreAppearance(options.reader, previous)
        }
        await cleanupManagedAsset(url)
      }
      options.onError?.(error, '上传背景图失败')
      return null
    }
  }

  async function clearBgImage(image) {
    if (!image) return false
    const previous = snapshotAppearance(options.reader)
    let persisted = false
    try {
      options.reader.removeCustomBgImage(image)
      const saved = await persistAssetReference(value => (
        value?.customBgImage !== image
        && (!Array.isArray(value?.customBgImageList) || !value.customBgImageList.includes(image))
      ))
      if (!saved) throw new Error('阅读设置同步失败')
      persisted = true
      await cleanupManagedAsset(image, { warn: true })
      options.onSuccess?.('已删除阅读背景图')
      return true
    } catch (error) {
      if (!persisted && backgroundAttemptIsRemoved(options.reader, image)) {
        restoreAppearance(options.reader, previous)
      }
      options.onError?.(error, '删除背景图失败')
      return false
    }
  }

  async function pickFontFile({ file, font } = {}) {
    const rawFile = file?.raw || file?.file || file
    if (!rawFile || !font?.value) return null
    const previous = snapshotAppearance(options.reader)
    const previousURL = options.reader.customFontsMap?.[font.value] || ''
    let url = ''
    let persisted = false
    try {
      const result = await options.upload({ file: rawFile, type: 'font' })
      url = result?.data?.url
      if (!url) throw new Error('上传结果缺少字体地址')
      options.reader.setCustomFont(font.value, url)
      options.reader.setFontFamily(font.value)
      options.syncFonts(options.reader.customFontsMap)
      const saved = await persistAssetReference(value => (
        value?.customFontsMap?.[font.value] === url
        && value?.fontFamily === font.value
      ))
      if (!saved) throw new Error('阅读设置同步失败')
      persisted = true
      if (previousURL && previousURL !== url) {
        await cleanupManagedAsset(previousURL, { warn: true })
      }
      options.onSuccess?.(`已上传${font.label}字体`)
      return url
    } catch (error) {
      if (url && !persisted) {
        if (options.reader.customFontsMap?.[font.value] === url) {
          restoreAppearance(options.reader, previous)
          options.syncFonts(options.reader.customFontsMap)
        }
        await cleanupManagedAsset(url)
      }
      options.onError?.(error, '上传字体失败')
      return null
    }
  }

  async function clearFontFile(font) {
    const url = options.reader.customFontsMap?.[font?.value]
    if (!url || !font?.value) return false
    const previous = snapshotAppearance(options.reader)
    let persisted = false
    try {
      options.reader.clearCustomFont(font.value)
      options.syncFonts(options.reader.customFontsMap)
      const saved = await persistAssetReference(value => value?.customFontsMap?.[font.value] !== url)
      if (!saved) throw new Error('阅读设置同步失败')
      persisted = true
      await cleanupManagedAsset(url, { warn: true })
      options.onSuccess?.(`已恢复默认${font.label}字体`)
      return true
    } catch (error) {
      if (!persisted && options.reader.customFontsMap?.[font.value] !== url) {
        restoreAppearance(options.reader, previous)
        options.syncFonts(options.reader.customFontsMap)
      }
      options.onError?.(error, '恢复默认字体失败')
      return false
    }
  }

  async function persistAssetReference(validate) {
    const saved = typeof options.saveSettings === 'function'
      ? await options.saveSettings()
      : appearanceSettingValue(options.reader)
    return saved && validate(saved) ? saved : null
  }

  async function cleanupManagedAsset(url, cleanupOptions = {}) {
    if (!managedAssetURL(url) || typeof options.removeAsset !== 'function') {
      return { status: 'skipped' }
    }
    try {
      await options.removeAsset(url)
      return { status: 'deleted' }
    } catch (error) {
      if (Number(error?.response?.status) === 409) {
        return { status: 'referenced' }
      }
      if (cleanupOptions.warn) {
        options.onWarning?.('阅读设置已保存，但旧文件清理失败')
      }
      return { status: 'failed', error }
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

function snapshotAppearance(reader) {
  return {
    customBgImage: reader.customBgImage || '',
    customBgImageList: Array.isArray(reader.customBgImageList) ? [...reader.customBgImageList] : [],
    customFontsMap: { ...(reader.customFontsMap || {}) },
    fontFamily: reader.fontFamily,
    customConfigList: cloneCustomConfigList(reader.customConfigList),
    settingsUpdatedAt: reader.settingsUpdatedAt,
  }
}

function restoreAppearance(reader, snapshot) {
  reader.customBgImage = snapshot.customBgImage
  reader.customBgImageList = [...snapshot.customBgImageList]
  reader.customFontsMap = { ...snapshot.customFontsMap }
  if (snapshot.fontFamily !== undefined) reader.fontFamily = snapshot.fontFamily
  if (snapshot.customConfigList !== undefined) {
    reader.customConfigList = cloneCustomConfigList(snapshot.customConfigList)
  }
  if (snapshot.settingsUpdatedAt !== undefined) reader.settingsUpdatedAt = snapshot.settingsUpdatedAt
}

function cloneCustomConfigList(value) {
  if (!Array.isArray(value)) return undefined
  return value.map(config => ({
    ...config,
    customBgImageList: Array.isArray(config?.customBgImageList) ? [...config.customBgImageList] : config?.customBgImageList,
    customFontsMap: config?.customFontsMap ? { ...config.customFontsMap } : config?.customFontsMap,
  }))
}

function appearanceSettingValue(reader) {
  return {
    customBgImage: reader.customBgImage || '',
    customBgImageList: Array.isArray(reader.customBgImageList) ? [...reader.customBgImageList] : [],
    customFontsMap: { ...(reader.customFontsMap || {}) },
    fontFamily: reader.fontFamily,
  }
}

function backgroundAttemptIsCurrent(reader, image) {
  return reader.customBgImage === image
    && Array.isArray(reader.customBgImageList)
    && reader.customBgImageList.includes(image)
}

function backgroundAttemptIsRemoved(reader, image) {
  return reader.customBgImage !== image
    && (!Array.isArray(reader.customBgImageList) || !reader.customBgImageList.includes(image))
}

function managedAssetURL(url) {
  return /^\/uploads\/users\/[1-9]\d*\/(?:covers|backgrounds|fonts|misc)\/[^/?#]+$/.test(String(url || ''))
}
