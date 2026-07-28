import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const overlay = readFileSync(
  new URL('../src/components/overlays/OverlayReplaceRules.vue', import.meta.url),
  'utf8',
)
const manager = overlay.split('<el-dialog\n    v-model="replaceRuleDialog"')[0]

test('replace-rule manager keeps the fixed upstream dialog, table, and footer structure', () => {
  assert.match(manager, /title="替换规则管理"/)
  assert.match(manager, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(manager, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(manager, /:fullscreen="isMobile"/)
  assert.match(manager, />\s*导入\s*</)
  assert.match(
    manager,
    /:height="isMobile \? 'calc\(100dvh - 184px\)' : 'min\(400px, calc\(70dvh - 184px\)\)'"/,
  )

  for (const label of ['规则名称', '替换范围', '是否启用', '操作']) {
    assert.match(manager, new RegExp(`label="${label}"`), `missing upstream table column ${label}`)
  }
  assert.match(manager, /type="selection"\s+width="25"\s+:fixed="isMobile"/)
  assert.match(manager, /prop="name"[\s\S]*?label="规则名称"[\s\S]*?:fixed="isMobile"/)
  assert.match(manager, />\s*编辑\s*</)
  assert.match(manager, />\s*批量删除\s*</)
  assert.match(manager, /已选择\s*\{\{\s*selectedReplaceRuleIds\.length\s*\}\}\s*个/)
  assert.match(manager, />\s*取消\s*</)

  assert.doesNotMatch(manager, /新增规则|>\s*刷新\s*<|逐行删除/)
  assert.doesNotMatch(manager, /label="匹配"|label="替换为"|label="正则"/)
  assert.doesNotMatch(manager, /mobile-rule-list|mobile-rule-card/)
  assert.doesNotMatch(manager, /removeReplaceRule/)
  assert.doesNotMatch(manager, /:icon="Upload"/)
})

test('replace-rule editor is the fixed sibling form without a competing test surface', () => {
  assert.match(overlay, /v-model="replaceRuleDialog"[\s\S]*?title="替换规则"/)
  assert.match(overlay, /v-model="replaceRuleDialog"[\s\S]*?width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(
    overlay,
    /v-model="replaceRuleDialog"[\s\S]*?top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/,
  )

  for (const label of ['名称', '规则', '替换为', '替换范围']) {
    assert.match(overlay, new RegExp(`<el-form-item label="${label}">`))
  }
  assert.match(overlay, /<el-checkbox\s+v-model="replaceRuleDraft\.isRegex">\s*使用正则表达式\s*<\/el-checkbox>/)
  assert.match(overlay, /<el-checkbox\s+v-model="replaceRuleDraft\.enabled">\s*是否启用\s*<\/el-checkbox>/)
  assert.match(overlay, />\s*取 消\s*</)
  assert.match(overlay, />\s*确 定\s*</)

  assert.doesNotMatch(overlay, /新增替换规则|编辑替换规则/)
  assert.doesNotMatch(overlay, /测试文本|测试规则|replaceRuleTest/)
  assert.doesNotMatch(overlay, /active-text="使用正则表达式"|active-text="启用"/)
  assert.match(overlay, /managerOperationGuard:\s*managerOperations/)
  assert.match(overlay, /editorOperationGuard:\s*editorOperations/)
})
