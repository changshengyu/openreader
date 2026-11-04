import assert from 'node:assert/strict'
import test from 'node:test'
import { epubTocRuleOptions } from '../src/utils/localBookToc.js'

// reader-dev PopCatalog.vue 110–117 and Index.vue 3013–3020 (fixed baseline fa22f27) name the six EPUB
// catalogue rules the same way in the Reader "修改规则" dialog and the import dialog
// (docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md R2).
test('EPUB catalogue rule options use the upstream names and values', () => {
  assert.deepEqual(epubTocRuleOptions, [
    { label: '根据 Spin 获取章节，使用 Toc 补充章节名', value: 'spin+toc' },
    { label: '根据 Spin 获取章节，强制使用 Toc 章节名', value: 'spin<toc' },
    { label: '根据 Spin 获取章节', value: 'spin' },
    { label: '根据 Toc 获取章节，使用 Spin 补充章节名', value: 'toc+spin' },
    { label: '根据 Toc 获取章节，强制使用 Spin 章节名', value: 'toc<spin' },
    { label: '根据 Toc 获取章节', value: 'toc' },
  ])
})
