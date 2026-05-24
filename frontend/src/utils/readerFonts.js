export const readerFontOptions = [
  {
    label: '系统',
    value: 'system',
    stack: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif',
  },
  {
    label: '衬线',
    value: 'serif',
    stack: '"Noto Serif CJK SC", "Source Han Serif SC", "Songti SC", "STSong", "SimSun", serif',
  },
  {
    label: '楷体',
    value: 'kai',
    stack: '"Kaiti SC", "STKaiti", "KaiTi", "AR PL UKai CN", cursive, serif',
  },
  {
    label: '等宽',
    value: 'mono',
    stack: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
  },
]

export function readerFontStack(value) {
  return readerFontOptions.find(font => font.value === value)?.stack || readerFontOptions[0].stack
}
