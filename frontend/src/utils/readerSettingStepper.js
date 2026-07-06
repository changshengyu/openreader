export function steppedReaderSettingValue({
  value,
  direction,
  min,
  max,
  step,
}) {
  const increment = Number(step) || 1
  const lower = Number.isFinite(Number(min)) ? Number(min) : -Infinity
  const upper = Number.isFinite(Number(max)) ? Number(max) : Infinity
  const precision = decimalPlaces(increment)
  const current = Number.isFinite(Number(value)) ? Number(value) : 0
  const next = current + Math.sign(Number(direction) || 0) * increment
  return Number(Math.max(lower, Math.min(upper, next)).toFixed(precision))
}

export function readerSettingStepLabel(value, step = 1) {
  const number = Number(value)
  if (!Number.isFinite(number)) return ''
  return String(Number(number.toFixed(decimalPlaces(Number(step) || 1))))
}

function decimalPlaces(value) {
  const text = String(value)
  if (text.includes('e-')) return Number(text.split('e-')[1]) || 0
  return text.includes('.') ? text.split('.')[1].length : 0
}
