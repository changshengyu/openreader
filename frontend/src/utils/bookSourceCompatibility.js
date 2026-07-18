const SCRIPT_TEMPLATE_TOKEN = /\{\{[\s\S]*?\}\}/
const WEBVIEW_TOKEN = /webview\s*:/i
const RULE_CONTAINER_KEYS = new Set([
  'rule',
  'rules',
  'rulevalues',
  'rulesearch',
  'ruleexplore',
  'rulebookinfo',
  'ruletoc',
  'rulecontent',
])
const EXECUTABLE_SOURCE_FIELDS = new Set([
  'rule',
  'searchurl',
  'exploreurl',
  'bookurlpattern',
  'rulebookurlpattern',
  'loginurl',
  'login',
])
const DORMANT_RULE_FIELDS = new Set([
  'preupdatejs',
  'chapterpreupdatejsrule',
  'webjs',
  'contentwebjsrule',
  'sourceregex',
  'contentsourceregex',
])
const NON_EXECUTABLE_RULE_FIELDS = new Set([
  ...DORMANT_RULE_FIELDS,
  'textreplacerules',
  'contentreplaceregex',
  'contentimagestyle',
  'headers',
  'canrename',
  'bookinfocanrenamerule',
])
const URL_TEMPLATE_RULE_FIELDS = new Set(['searchurl', 'exploreurl'])

export function analyzeSourceCompatibility(source) {
  const reasons = new Set()
  const dormantFields = []
  const value = source && typeof source === 'object' ? source : {}

  const header = firstNonBlankString(value.header, value.headerMap)
  if (isDynamicHeader(header)) reasons.add('dynamic-header')
  if (firstNonBlankString(value.loginCheckJs, value.loginCheckJS)) reasons.add('login-check')

  inspectSourceEntry(value.loginUrl, reasons, { allowTemplate: true })
  inspectSourceEntry(value.login, reasons, { allowTemplate: true })

  for (const [key, fieldValue] of Object.entries(value)) {
    const normalizedKey = key.toLowerCase()
    if (RULE_CONTAINER_KEYS.has(normalizedKey)) {
      inspectRuleContainer(parseRuleContainer(fieldValue), key, reasons, dormantFields)
      continue
    }
    if (EXECUTABLE_SOURCE_FIELDS.has(normalizedKey)) {
      inspectSourceEntry(fieldValue, reasons, { allowTemplate: true })
    }
  }

  const tags = []
  if ([...reasons].some(reason => reason !== 'webview')) tags.push('@Javascript')
  if (reasons.has('webview')) tags.push('@WebView')
  const blocking = reasons.size > 0
  return {
    status: blocking
      ? (reasons.size === 1 && reasons.has('webview') ? 'unsupported-webview' : 'unsupported-script')
      : (dormantFields.length ? 'preserved-dormant' : 'supported'),
    blocking,
    tags,
    reasons: [...reasons],
    dormantFields: [...new Set(dormantFields)],
  }
}

export function importSourceTags(source) {
  return analyzeSourceCompatibility(source).tags.join(' ')
}

export function importSourceCompatibilityHint(source) {
  return sourceCompatibilityMessage(analyzeSourceCompatibility(source))
}

export function sourceCompatibilityMessage(analysis) {
  if (!analysis) return ''
  const labels = {
    'dynamic-header': '动态请求头依赖 JavaScript',
    'login-check': '登录检测依赖 JavaScript',
    'rule-script': '规则依赖 JavaScript',
    'rule-template': '规则依赖脚本模板',
    webview: '登录或请求依赖 WebView',
  }
  const blockers = (analysis.reasons || []).map(reason => labels[reason]).filter(Boolean)
  if (blockers.length) {
    return `${blockers.join('；')}；配置会保留，但当前服务不会执行`
  }
  if (analysis.dormantFields?.length) {
    return '包含固定基准普通 HTTP 流程未消费的兼容字段；配置仅无损保存'
  }
  return ''
}

function inspectRuleContainer(value, path, reasons, dormantFields) {
  if (typeof value === 'string') {
    inspectSourceEntry(value, reasons)
    return
  }
  if (!value || typeof value !== 'object') return
  for (const [key, child] of Object.entries(value)) {
    const normalizedKey = key.toLowerCase()
    const childPath = `${path}.${key}`
    if (DORMANT_RULE_FIELDS.has(normalizedKey)) {
      if (hasConfiguredValue(child)) dormantFields.push(childPath)
      continue
    }
    if (NON_EXECUTABLE_RULE_FIELDS.has(normalizedKey)) continue
    if (child && typeof child === 'object') {
      inspectRuleContainer(child, childPath, reasons, dormantFields)
      continue
    }
    inspectSourceEntry(child, reasons, { allowTemplate: URL_TEMPLATE_RULE_FIELDS.has(normalizedKey) })
  }
}

function inspectSourceEntry(value, reasons, { allowTemplate = false } = {}) {
  if (typeof value !== 'string') return
  if (WEBVIEW_TOKEN.test(value)) reasons.add('webview')
  if (/@js:|<js(?:\s|>)/i.test(value)) reasons.add('rule-script')
  if (!allowTemplate && SCRIPT_TEMPLATE_TOKEN.test(value)) reasons.add('rule-template')
}

function parseRuleContainer(value) {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return value
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

function isDynamicHeader(value) {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized.startsWith('@js:') || normalized.startsWith('<js>')
}

function firstNonBlankString(...values) {
  return values.find(value => typeof value === 'string' && value.trim())?.trim() || ''
}

function hasConfiguredValue(value) {
  if (typeof value === 'string') return Boolean(value.trim())
  return value != null
}
