import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const panelSource = readFileSync(new URL('../src/components/reader/ReaderSettingsPanel.vue', import.meta.url), 'utf8')
const mobileWorkspaceSource = readFileSync(new URL('../src/components/reader/ReaderMobileWorkspacePanel.vue', import.meta.url), 'utf8')
const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

test('ReaderSettingsPanel exposes upstream canonical settings labels', () => {
  for (const label of ['阅读主题', '正文字体', '字体大小', '字体粗细', '段落行高']) {
    assert.match(panelSource, new RegExp(`>${label}<`), `missing canonical label ${label}`)
  }
  for (const shortened of ['>主题<', '>字体<', '>字号<', '>字重<', '>行高<']) {
    assert.doesNotMatch(panelSource, new RegExp(shortened), `should not expose shortened label ${shortened}`)
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

test('reader settings selected controls use upstream accent color', () => {
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
  assert.match(panelSource, /\.selection-button \{[\s\S]*?min-width: 78px;[\s\S]*?height: 34px;/, 'selection buttons should keep upstream span-item dimensions')
  assert.match(panelSource, /\.selection-button\.active \{[\s\S]*?color: #ed4259;[\s\S]*?border-color: #ed4259;/, 'selection buttons should keep upstream selected color')
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
  assert.match(panelSource, /\.delete-bg-icon \{[\s\S]*?top: -6px;[\s\S]*?right: -6px;[\s\S]*?color: #ed4259;/, 'background delete icons should keep upstream top-right red placement')
  assert.match(panelSource, /\.upload-bg-btn \{[\s\S]*?display: inline-block;[\s\S]*?color: #ed4259;/, 'background upload should keep upstream inline red style')
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
  assert.match(panelSource, /\.font-family-option\.active \{[\s\S]*?color: #ed4259;[\s\S]*?border-color: #ed4259;/, 'font options should keep upstream selected color')
  assert.match(panelSource, /\.font-family-actions \{[\s\S]*?position: absolute;[\s\S]*?top: -10px;[\s\S]*?right: -10px;/, 'font upload actions should be positioned like upstream upload icons')
  assert.match(panelSource, /\.font-action-btn\.active,[\s\S]*?\.font-action-btn:hover \{[\s\S]*?color: #ed4259;/, 'uploaded font icons should use upstream active color')
})
