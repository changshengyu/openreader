export function useReaderSelectedTextActions(options) {
  async function createReplaceRuleFromText(text) {
    const prompt = await options.prompt(
      '替换为留空时表示直接过滤该文字。',
      '添加过滤规则',
      {
        confirmButtonText: '保存',
        cancelButtonText: '取消',
        inputValue: '',
        inputPlaceholder: '替换为',
      },
    ).catch(() => null)
    if (!prompt) return
    const cleanText = String(text || '').trim()
    if (!cleanText) return
    const name = cleanText.length > 24
      ? `${cleanText.slice(0, 24)}...`
      : cleanText
    const book = options.getBook()
    await options.createReplaceRule({
      name,
      pattern: cleanText,
      replacement: String(prompt.value || ''),
      scope: `${book?.title || ''};${book?.url || ''}`,
      isRegex: false,
      enabled: true,
    })
    options.dispatchRulesUpdated()
    options.onSuccess('过滤规则已添加')
  }

  async function operate(text) {
    const action = await options.confirm(
      '请选择对选中文字执行的操作。',
      '选择文字',
      {
        confirmButtonText: '添加过滤规则',
        cancelButtonText: '添加书签',
        distinguishCancelAndClose: true,
        closeOnClickModal: false,
        closeOnPressEscape: false,
        type: 'info',
      },
    ).catch(result => result)
    if (action === 'close') return
    if (action === 'cancel') {
      await options.createBookmark(text)
      return
    }
    await createReplaceRuleFromText(text)
  }

  return {
    createReplaceRuleFromText,
    operate,
  }
}
