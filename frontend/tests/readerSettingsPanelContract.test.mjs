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
  assert.match(panelSource, /\.theme-dot\.active \{[\s\S]*?#ed4259/, 'theme dots should use upstream accent color')
  assert.match(panelSource, /\.bg-image-option\.active \{[\s\S]*?#ed4259/, 'background selections should use upstream accent color')
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
