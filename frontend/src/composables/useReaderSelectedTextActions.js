export function createReaderSelectedTextReplaceRuleDraft({ text, book, now = new Date() } = {}) {
  const pattern = String(text || '')
  if (!pattern.trim()) return null
  const timestamp = now instanceof Date && !Number.isNaN(now.getTime())
    ? now
    : new Date()
  return {
    name: `文本替换 ${formatReaderSelectedTextRuleTime(timestamp)}`,
    pattern,
    replacement: '',
    scope: `${book?.title || ''};${book?.url || ''}`,
    isRegex: false,
    enabled: true,
  }
}

export function useReaderSelectedTextActions(options) {
  function createReplaceRuleFromText(text) {
    const draft = createReaderSelectedTextReplaceRuleDraft({
      text,
      book: options.getBook?.(),
      now: options.now?.(),
    })
    if (draft) options.openReplaceRuleEditor?.(draft)
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

function formatReaderSelectedTextRuleTime(value) {
  const pad = number => String(number).padStart(2, '0')
  return [
    value.getFullYear(),
    pad(value.getMonth() + 1),
    pad(value.getDate()),
  ].join('-') + ` ${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}`
}
