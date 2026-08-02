import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const panelSource = readFileSync(new URL('../src/components/reader/ReaderSettingsPanel.vue', import.meta.url), 'utf8')
const mobileWorkspaceSource = readFileSync(new URL('../src/components/reader/ReaderMobileWorkspacePanel.vue', import.meta.url), 'utf8')
const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const readerStoreSource = readFileSync(new URL('../src/stores/reader.js', import.meta.url), 'utf8')

test('ReaderSettingsPanel exposes upstream canonical settings labels', () => {
  for (const label of ['阅读主题', '正文字体', '字体大小', '字体粗细', '段落行高']) {
    assert.match(panelSource, new RegExp(`>${label}<`), `missing canonical label ${label}`)
  }
  for (const shortened of ['>主题<', '>字体<', '>字号<', '>字重<', '>行高<']) {
    assert.doesNotMatch(panelSource, new RegExp(shortened), `should not expose shortened label ${shortened}`)
  }
})

test('ReaderSettingsPanel preserves the fixed-baseline core order and exact operation zone', () => {
  const orderedLabels = [
    '特殊模式',
    '配置方案',
    '方案类型',
    '阅读主题',
    '正文字体',
    '简繁转换',
    '字体大小',
    '字体粗细',
    '段落行高',
    '段落间距',
    '字体颜色',
    '页面模式',
    '页面宽度',
    '翻页方式',
    '动画时长',
    '自动翻页',
    '滚动像素',
    '翻页速度',
    '全屏点击',
    '选择文字',
  ]
  let last = -1
  for (const label of orderedLabels) {
    const position = panelSource.indexOf(`>${label}<`)
    assert(position > last, `missing or out-of-order fixed-baseline setting ${label}`)
    last = position
  }

  assert.equal((panelSource.match(/class="setting-divider"/g) || []).length, 2, 'settings must keep both upstream dividers')
  assert.match(panelSource, /class="operation-zone"[\s\S]*?>显示翻页区域<[\s\S]*?>过滤规则管理</)
  assert.doesNotMatch(panelSource, /<label class="setting-label">替换规则<\/label>/)
  assert.doesNotMatch(panelSource, /管理全局替换规则/)
})

test('ReaderSettingsPanel keeps requested anti-mistap additions without duplicating the TTS bar', () => {
  for (const requested of ['>亮度<', 'class="font-preview"', 'class="font-size-presets"']) {
    assert.match(panelSource, new RegExp(requested), `missing requested settings affordance ${requested}`)
  }
  for (const duplicate of ['朗读语速', '朗读音调', '朗读语音']) {
    assert.doesNotMatch(panelSource, new RegExp(duplicate), `ReaderSettings must not duplicate TTS bar control ${duplicate}`)
  }
  assert.doesNotMatch(panelSource, /ttsVoices|ttsRateChange|ttsPitchChange|ttsVoiceChange/)
  assert.doesNotMatch(panelSource, /reader\.fontColor[\s\S]{0,160}恢复默认/)
})

test('ReaderSettingsPanel restores all fourteen built-in background choices in upstream order', () => {
  const backgroundSources = `${readerStoreSource}\n${panelSource}`
  const backgrounds = [
    '山水画.jpg', '山水墨影.jpg', '羊皮纸1.jpg', '护眼漫绿.jpg', '羊皮纸2.jpg', '新羊皮纸.jpg',
    '羊皮纸3.jpg', '明媚倾城.jpg', '羊皮纸4.jpg', '深宫魅影.jpg', '午后沙滩.jpg', '清新时光.jpg',
    '宁静夜色.jpg', '边彩画布.jpg',
  ]
  let last = -1
  for (const filename of backgrounds) {
    const position = backgroundSources.indexOf(filename)
    assert(position > last, `missing or out-of-order built-in background ${filename}`)
    assert.equal(
      existsSync(new URL(`../public/bg/${filename}`, import.meta.url)),
      true,
      `missing copied fixed-baseline background asset ${filename}`,
    )
    last = position
  }
  assert.match(panelSource, /readerBuiltInBackgrounds/)
  assert.match(panelSource, /v-for="image in builtInBackgrounds"/)
  assert.match(panelSource, /v-for="image in reader\.customBgImageList"/)
})

test('ReaderSettingsPanel uses one upstream font action and leaves animation editable in Kindle mode', () => {
  assert.match(panelSource, /function manageFont\(font\)/)
  assert.match(panelSource, /继续上传/)
  assert.match(panelSource, /恢复默认/)
  assert.doesNotMatch(panelSource, /class="font-family-actions"/)
  assert.doesNotMatch(panelSource, /:disabled="reader\.pageType === 'kindle'"/)
  assert.match(panelSource, /<label class="setting-label">自动翻页<\/label>/)
})

test('reader preset surfaces map every fixed-baseline theme resource while retaining pure-black night', () => {
  for (const index of [0, 1, 2, 3, 5]) {
    assert.match(readerStoreSource, new RegExp(`body_${index}\\.png|body-${index}\\.png`), `missing body texture ${index}`)
    assert.match(readerStoreSource, new RegExp(`content_${index}\\.png|content-${index}\\.png`), `missing content texture ${index}`)
    assert.match(readerStoreSource, new RegExp(`popup_${index}\\.png|popup-${index}\\.png`), `missing popup texture ${index}`)
    for (const surface of ['body', 'content', 'popup']) {
      assert.equal(
        existsSync(new URL(`../public/themes/${surface}_${index}.png`, import.meta.url)),
        true,
        `missing copied fixed-baseline ${surface} texture ${index}`,
      )
    }
  }
  for (const surface of ['body', 'content']) {
    assert.equal(
      existsSync(new URL(`../public/themes/${surface}_6.png`, import.meta.url)),
      true,
      `missing copied fixed-baseline night ${surface} texture`,
    )
  }
})

test('mobile reader settings suppresses the generic workspace header', () => {
  assert.match(mobileWorkspaceSource, /showHeader/, 'mobile workspace must expose header visibility control')
  assert.match(readerViewSource, /title="设置"\s+:show-header="false"/, 'mobile settings should not add a second generic 设置 title')
  assert.match(panelSource, /class="settings-title"/, 'settings panel must keep the upstream ReadSettings title')
  assert.match(panelSource, /class="settings-title"[\s\S]*?设置[\s\S]*?重置为默认配置/, 'settings title must own the upstream reset action')
  assert.match(panelSource, /<\/div>\s*<div class="settings-list">/, 'settings title and scrollable settings list must be siblings')
  assert.doesNotMatch(panelSource, /<strong>设置<\/strong>/, 'settings title must not retain the rewritten underlined strong label')
})

test('reader settings keeps the upstream fixed title while only the list scrolls', () => {
  assert.match(
    panelSource,
    /\.settings-list\s*\{[\s\S]*?max-height:\s*45vh;[\s\S]*?overflow-y:\s*auto;/,
    'only the settings list should own the upstream 45vh vertical scrolling region',
  )
  assert.match(
    panelSource,
    /\.settings-title\s*\{[\s\S]*?font-size:\s*18px;[\s\S]*?line-height:\s*22px;[\s\S]*?margin-bottom:\s*28px;[\s\S]*?font-weight:\s*400;/,
    'settings title must retain upstream fixed-title typography and spacing',
  )
})

test('mobile reader settings keeps upstream-like two-column row geometry', () => {
  assert.match(panelSource, /@media \(max-width: 750px\)[\s\S]*?\.settings-list \{\s*gap: 20px;/, 'mobile settings rows should keep upstream-like 20px vertical density')
  assert.match(panelSource, /@media \(max-width: 750px\)[\s\S]*?\.setting-row \{[\s\S]*?grid-template-columns: 72px minmax\(0, 1fr\);/, 'mobile settings should use 56px label + 16px gutter geometry')
  assert.match(panelSource, /@media \(max-width: 750px\)[\s\S]*?\.setting-row > \.setting-label \{[\s\S]*?line-height: 36px;/, 'mobile settings labels should align with upstream 36px controls')
  assert.match(panelSource, /@media \(max-width: 750px\)[\s\S]*?\.setting-row > :not\(\.setting-label\) \{[\s\S]*?grid-column: 2;/, 'mobile settings controls should start in the second column')
})

test('desktop reader settings keeps the upstream 56px label plus 16px gutter', () => {
  assert.match(panelSource, /@media \(min-width: 751px\)[\s\S]*?\.setting-row \{[\s\S]*?grid-template-columns: 56px minmax\(0, 1fr\);[\s\S]*?column-gap: 16px;/, 'desktop settings should use the upstream 56px label and 16px gutter')
  assert.match(panelSource, /@media \(min-width: 751px\)[\s\S]*?\.typography-setting-row,[\s\S]*?\.stepper-setting-row \{[\s\S]*?grid-template-columns: 56px minmax\(0, 220px\);[\s\S]*?column-gap: 16px;/, 'desktop numeric settings should use the same upstream label gutter')
})

test('reader settings selected controls use the semantic accent with the upstream day fallback', () => {
  assert.match(panelSource, /\.theme-item\.active \{[\s\S]*?#ed4259/, 'theme items should use upstream accent color')
  assert.match(panelSource, /\.content-bg-preview\.selected \{[\s\S]*?#ed4259/, 'background selections should use upstream accent color')
  assert.match(panelSource, /\.font-family-option\.active \{[\s\S]*?#ed4259/, 'font selections should use upstream accent color')
  assert.match(panelSource, /\.font-size-preset\.active \{[\s\S]*?#ed4259/, 'font-size presets should use upstream accent color')
  for (const staleColor of ['#409eff', '#0f5451', '#2f6f6d']) {
    assert.doesNotMatch(panelSource, new RegExp(staleColor, 'i'), `settings panel should not keep stale active color ${staleColor}`)
  }
})

test('reader settings discrete options use upstream-like local buttons', () => {
  assert.doesNotMatch(panelSource, /<el-radio-group\b/, 'settings panel should not use Element radio groups for upstream span-item options')
  assert.doesNotMatch(panelSource, /<el-radio-button\b/, 'settings panel should not use Element radio buttons for upstream span-item options')
  assert.match(panelSource, /class="selection-zone"/, 'settings panel should expose upstream-like selection zones')
  assert.match(panelSource, /class="selection-button"/, 'settings panel should expose upstream-like selection buttons')
  assert.match(panelSource, /\.selection-button \{[\s\S]*?box-sizing: border-box;[\s\S]*?width: 78px;[\s\S]*?min-width: 78px;[\s\S]*?height: 34px;/, 'selection buttons should keep upstream 78x34 outer dimensions')
  assert.match(panelSource, /\.selection-button\.active \{[\s\S]*?color: var\(--reader-accent, #ed4259\);[\s\S]*?border-color: var\(--reader-accent, #ed4259\);/, 'selection buttons should use the semantic accent with the upstream day fallback')
})

test('reader settings keeps upstream warning and configuration option ownership', () => {
  const specialModeStart = panelSource.indexOf('<label class="setting-label">特殊模式</label>')
  const specialModeEnd = panelSource.indexOf('<div class="setting-row">', specialModeStart + 1)
  const specialMode = panelSource.slice(specialModeStart, specialModeEnd)
  const readerModeStart = panelSource.indexOf('<label class="setting-label">翻页方式</label>')
  const readerModeEnd = panelSource.indexOf('<div class="setting-row', readerModeStart + 1)
  const readerMode = panelSource.slice(readerModeStart, readerModeEnd)

  assert(specialModeStart >= 0 && specialModeEnd > specialModeStart, 'special-mode setting row missing')
  assert(readerModeStart >= 0 && readerModeEnd > readerModeStart, 'read-method setting row missing')
  assert.match(specialMode, /<div class="selection-zone">[\s\S]*?class="setting-help"/, 'special-mode warning must belong to its selection zone')
  assert.match(readerMode, /<div class="selection-zone">[\s\S]*?class="setting-help"/, 'read-method warning must belong to its selection zone')
  assert.match(panelSource, /class="selection-zone config-scheme-list"/, 'configuration schemes must use the shared discrete option zone')
  assert.match(panelSource, /class="selection-button config-scheme"/, 'configuration schemes must use compact option controls')
  assert.doesNotMatch(panelSource, /<small v-if="config\.configDefaultType">/, 'configuration type is its own upstream row, not a card subtitle')
  assert.match(panelSource, /\.config-scheme \{[\s\S]*?width: 78px;[\s\S]*?height: 34px;[\s\S]*?border-radius: 2px;/, 'configuration scheme controls must keep upstream 78x34 geometry')
  assert.doesNotMatch(panelSource, /\.config-scheme\s*\{[^}]*border-radius:\s*6px;/, 'configuration scheme controls must not retain card rounding')
  assert.match(panelSource, /\.setting-help \{[\s\S]*?flex-basis: 100%;/, 'compact warning text must wrap inside its owning option zone')
})

test('reader settings theme options use upstream theme-item geometry', () => {
  assert.match(panelSource, /class="selection-zone theme-grid"/, 'theme options should share upstream selection-zone structure')
  assert.match(panelSource, /class="theme-check"/, 'non-night theme options should expose the upstream selected check glyph')
  assert.match(panelSource, /class="moon-icon"/, 'night theme option should expose the upstream moon glyph')
  assert.match(panelSource, /class="selection-button theme-custom-button"/, 'custom theme should be a rectangular span-item-like button')
  assert.doesNotMatch(panelSource, /custom-dot/, 'custom theme should not be rendered as a circular plus dot')
  assert.doesNotMatch(panelSource, /\.theme-dot\b/, 'theme presets should not use the old dot component class')
  assert.match(panelSource, /\.theme-item \{[\s\S]*?width: 34px;[\s\S]*?height: 34px;[\s\S]*?border-radius: 100%;/, 'theme items should keep upstream 34px circular geometry')
  assert.doesNotMatch(panelSource, /\.theme-item\.active \{[\s\S]*?box-shadow:/, 'theme selected state should not use the old OpenReader box-shadow ring')
})

test('reader settings background previews use upstream thumbnail geometry', () => {
  assert.match(panelSource, /class="custom-theme-title bg-image-title"/, 'background image row should use upstream custom-theme-title inline structure')
  assert.match(panelSource, /class="content-bg-preview"/, 'background images should use upstream content-bg-preview thumbnails')
  assert.match(panelSource, /class="upload-bg-btn"/, 'background upload should use upstream inline upload text action')
  assert.doesNotMatch(panelSource, /class="bg-image-option"/, 'background images should not keep card-style option tiles')
  assert.doesNotMatch(panelSource, /\.bg-image-option\b/, 'background image CSS should not keep card-style option tiles')
  assert.doesNotMatch(panelSource, />使用中</, 'background thumbnails should not render active card overlay labels')
  assert.doesNotMatch(panelSource, />选择</, 'background thumbnails should not render selectable card overlay labels')
  assert.match(panelSource, /\.content-bg-preview \{[\s\S]*?width: 36px;[\s\S]*?height: 36px;[\s\S]*?display: inline-block;/, 'background thumbnails should keep upstream 36px inline geometry')
  assert.match(panelSource, /\.delete-bg-icon \{[\s\S]*?top: -6px;[\s\S]*?right: -6px;[\s\S]*?color: var\(--reader-accent, #ed4259\);/, 'background delete icons should keep upstream top-right placement with a night-readable accent')
  assert.match(panelSource, /\.upload-bg-btn \{[\s\S]*?display: inline-block;[\s\S]*?color: var\(--reader-accent, #ed4259\);/, 'background upload should keep the upstream inline accent style')
})

test('reader settings custom theme block uses upstream inline structure', () => {
  assert.match(panelSource, /<label class="setting-label">自定义<\/label>/, 'custom theme block should use the upstream left label 自定义')
  assert.match(panelSource, /class="custom-theme"/, 'custom theme controls should be grouped in one upstream-like custom-theme block')
  for (const label of ['主题模式', '页面背景颜色', '浮窗背景颜色', '阅读背景颜色', '阅读背景图片']) {
    assert.match(panelSource, new RegExp(`class="custom-theme-title[^\"]*"[^>]*>[\\s\\S]*?${label}`), `missing inline custom theme title ${label}`)
  }
  assert.match(panelSource, /v-for="option in themeTypeOptions"/, 'custom theme mode must expose the upstream day/night options')
  assert.match(panelSource, /:class="\{ active: themeTypeModel === option\.value \}"/, 'custom theme mode must expose its selected state')
  assert.match(panelSource, /@click="themeTypeModel = option\.value"/, 'custom theme mode must update the persisted semantic type')
  assert.match(panelSource, /\{ value: 'day', label: '白天' \}/, 'custom theme mode must expose 白天')
  assert.match(panelSource, /\{ value: 'night', label: '黑夜' \}/, 'custom theme mode must expose 黑夜')
  assert.match(panelSource, /const themeTypeModel = computed\(\{[\s\S]*?get: \(\) => props\.reader\.themeType,[\s\S]*?set: value => props\.reader\.setThemeType\(value\)/, 'custom theme mode must bind to the reader store')
  assert.match(panelSource, /\.custom-theme \{[\s\S]*?display: inline-block;/, 'custom theme block should keep upstream inline-block layout')
  assert.match(panelSource, /\.custom-theme-title \{[\s\S]*?display: inline-block;[\s\S]*?margin-right: 28px;[\s\S]*?margin-bottom: 5px;/, 'custom theme titles should keep upstream inline spacing')
  assert.doesNotMatch(panelSource, /reader\.customBodyColor[\s\S]{0,120}恢复默认/, 'custom body color should not keep a separate per-row reset button in Reader settings')
  assert.doesNotMatch(panelSource, /reader\.customPopupColor[\s\S]{0,120}恢复默认/, 'custom popup color should not keep a separate per-row reset button in Reader settings')
})

test('reader settings font options use upstream span-item geometry', () => {
  assert.match(panelSource, /class="selection-zone font-family-grid"/, 'font options should share the upstream selection-zone structure')
  assert.doesNotMatch(panelSource, />已上传</, 'font upload state should be represented by the upstream-like active upload icon, not extra text')
  assert.match(panelSource, /\.font-family-option \{[\s\S]*?width: 78px;[\s\S]*?height: 34px;[\s\S]*?border-radius: 2px;/, 'font options should keep upstream 78x34 span-item geometry')
  assert.match(panelSource, /\.font-family-option \{[\s\S]*?font: 14px \/ 34px/, 'font options should keep upstream 14px/34px font shorthand')
  assert.match(panelSource, /\.font-family-option\.active \{[\s\S]*?color: var\(--reader-accent, #ed4259\);[\s\S]*?border-color: var\(--reader-accent, #ed4259\);/, 'font options should use the semantic selected color')
  assert.match(panelSource, /\.font-action-btn \{[\s\S]*?position: absolute;[\s\S]*?top: -10px;[\s\S]*?right: -10px;/, 'the single font action should be positioned like the upstream upload icon')
  assert.match(panelSource, /\.font-action-btn\.active,[\s\S]*?\.font-action-btn:hover \{[\s\S]*?color: var\(--reader-accent, #ed4259\);/, 'uploaded font icons should use the semantic active color')
})
