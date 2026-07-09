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
  assert.match(panelSource, /<strong>设置<\/strong>/, 'settings panel must keep the upstream ReadSettings title row')
  assert.match(panelSource, /重置为默认配置/, 'settings panel must keep the upstream reset action')
})

test('mobile reader settings keeps upstream-like two-column row geometry', () => {
  assert.match(panelSource, /@media \(max-width: 750px\)[\s\S]*?\.settings-body \{\s*gap: 20px;/, 'mobile settings rows should keep upstream-like 20px vertical density')
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

test('reader settings font options use upstream span-item geometry', () => {
  assert.match(panelSource, /class="selection-zone font-family-grid"/, 'font options should share the upstream selection-zone structure')
  assert.doesNotMatch(panelSource, />已上传</, 'font upload state should be represented by the upstream-like active upload icon, not extra text')
  assert.match(panelSource, /\.font-family-option \{[\s\S]*?width: 78px;[\s\S]*?height: 34px;[\s\S]*?border-radius: 2px;/, 'font options should keep upstream 78x34 span-item geometry')
  assert.match(panelSource, /\.font-family-option \{[\s\S]*?font: 14px \/ 34px/, 'font options should keep upstream 14px/34px font shorthand')
  assert.match(panelSource, /\.font-family-option\.active \{[\s\S]*?color: #ed4259;[\s\S]*?border-color: #ed4259;/, 'font options should keep upstream selected color')
  assert.match(panelSource, /\.font-family-actions \{[\s\S]*?position: absolute;[\s\S]*?top: -10px;[\s\S]*?right: -10px;/, 'font upload actions should be positioned like upstream upload icons')
  assert.match(panelSource, /\.font-action-btn\.active,[\s\S]*?\.font-action-btn:hover \{[\s\S]*?color: #ed4259;/, 'uploaded font icons should use upstream active color')
})
