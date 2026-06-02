export const readerFontOptions = [
  {
    label: '系统',
    value: 'system',
    customFamily: 'OpenReaderCustomSystem',
    stack: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif',
  },
  {
    label: '衬线',
    value: 'serif',
    customFamily: 'OpenReaderCustomSerif',
    stack: '"Noto Serif CJK SC", "Source Han Serif SC", "Songti SC", "STSong", "SimSun", serif',
  },
  {
    label: '楷体',
    value: 'kai',
    customFamily: 'OpenReaderCustomKai',
    stack: '"Kaiti SC", "STKaiti", "KaiTi", "AR PL UKai CN", cursive, serif',
  },
  {
    label: '等宽',
    value: 'mono',
    customFamily: 'OpenReaderCustomMono',
    stack: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
  },
]

export function readerFontStack(value, customFontsMap = {}) {
  const option = readerFontOptions.find(font => font.value === value) || readerFontOptions[0]
  if (customFontsMap?.[option.value]) return `"${option.customFamily}", ${option.stack}`
  return option.stack
}

export function syncReaderFontFaces(customFontsMap = {}) {
  if (typeof document === 'undefined') return
  const styleId = 'openreader-custom-fonts'
  let style = document.getElementById(styleId)
  if (!style) {
    style = document.createElement('style')
    style.id = styleId
    document.head.appendChild(style)
  }
  style.textContent = readerFontOptions
    .filter(font => customFontsMap?.[font.value])
    .map(font => {
      const url = String(customFontsMap[font.value]).replace(/'/g, "\\'")
      return `@font-face{font-family:"${font.customFamily}";src:url('${url}');font-display:swap;}`
    })
    .join('\n')
}
