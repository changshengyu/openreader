import { h, ref, unref } from 'vue'
import { epubTocRuleOptions } from '../utils/localBookToc.js'

export function useReaderLocalTocRulePicker(options) {
  async function choose() {
    const currentBook = unref(options.book)
    if (!unref(options.isEPUBLocalBook)) {
      const result = await options.prompt(
        '填写 TXT 目录行正则，留空则使用默认目录规则。',
        '修改目录规则',
        {
          confirmButtonText: '刷新目录',
          cancelButtonText: '取消',
          inputType: 'textarea',
          inputValue: currentBook?.tocRule || '',
          inputPlaceholder: '^第.+章.*$',
        },
      ).catch(() => null)
      return result ? (result.value || '') : null
    }

    const selected = ref(currentBook?.tocRule || 'spin+toc')
    const selector = h('select', {
      value: selected.value,
      style: 'width:100%;min-height:38px;padding:0 10px;border:1px solid var(--el-border-color);border-radius:4px;background:var(--el-bg-color);color:var(--el-text-color-primary)',
      onChange: event => {
        selected.value = event.target.value
      },
    }, epubTocRuleOptions.map(rule => h(
      'option',
      { value: rule.value },
      rule.label,
    )))
    const confirmed = await options.confirm(selector, '修改 EPUB 目录规则', {
      confirmButtonText: '刷新目录',
      cancelButtonText: '取消',
    }).catch(() => false)
    return confirmed ? selected.value : null
  }

  return { choose }
}
