import { defineComponent, h, ref, unref } from 'vue'
import { ElInput, ElOption, ElSelect } from 'element-plus'
import { epubTocRuleOptions } from '../utils/localBookToc.js'

// reader-dev PopCatalog.vue#changeRule/renderComp (164–252): one $msgbox for TXT and EPUB local books with
// "请选择规则：", a filterable built-in rule select and a textarea sharing the same value, starting from the
// book's current rule; confirming an unchanged rule shows "未修改规则" and refreshes nothing
// (docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md R1–R4).
export function useReaderLocalTocRulePicker(options) {
  let txtRules = null

  async function loadTxtRules() {
    if (txtRules) return txtRules
    try {
      const rules = await options.loadTxtTocRules()
      txtRules = (Array.isArray(rules) ? rules : []).filter(rule => rule?.rule)
      return txtRules
    } catch (error) {
      options.onError?.(error, '加载目录规则失败')
      return []
    }
  }

  async function choose() {
    const currentRule = String(unref(options.book)?.tocRule || '')
    const rules = unref(options.isEPUBLocalBook)
      ? epubTocRuleOptions.map(option => ({ name: option.label, rule: option.value }))
      : await loadTxtRules()
    const selected = ref(currentRule)
    const update = value => {
      selected.value = String(value ?? '')
    }
    const RulePicker = defineComponent({
      name: 'ReaderTocRulePicker',
      setup: () => () => h('div', { class: 'reader-toc-rule-picker', style: { textAlign: 'center' } }, [
        h('span', '请选择规则：'),
        h(ElSelect, {
          modelValue: selected.value,
          'onUpdate:modelValue': update,
          size: 'small',
          filterable: true,
          placeholder: '内置规则',
          class: 'reader-toc-rule-select',
        }, {
          default: () => rules.map((rule, index) => h(ElOption, {
            key: `rule-${index}`,
            label: rule.name,
            value: rule.rule,
          })),
        }),
        h(ElInput, {
          modelValue: selected.value,
          'onUpdate:modelValue': update,
          type: 'textarea',
          rows: 3,
          size: 'small',
          style: { marginTop: '10px' },
        }),
      ]),
    })

    const action = await options.msgbox({
      title: '修改目录规则',
      message: h(RulePicker),
      showCancelButton: true,
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      customClass: 'reader-toc-rule-dialog',
    }).catch(() => null)
    if (action !== 'confirm') return null
    if (selected.value === currentRule) {
      options.notifyError?.('未修改规则')
      return null
    }
    return selected.value
  }

  return { choose }
}
