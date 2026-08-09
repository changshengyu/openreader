export const BOOK_SOURCE_RULE_KEYS = [
  'exploreUrl', 'searchUrl', 'bookListRule', 'bookNameRule', 'bookAuthorRule', 'bookCoverRule',
  'bookIntroRule', 'bookKindRule', 'bookWordCountRule', 'latestChapterRule', 'bookUpdateTimeRule',
  'bookUrlRule', 'paginationRule', 'exploreBookListRule', 'exploreBookNameRule',
  'exploreBookAuthorRule', 'exploreBookCoverRule', 'exploreBookIntroRule', 'exploreBookKindRule',
  'exploreBookWordCountRule', 'exploreLatestChapterRule', 'exploreBookUpdateTimeRule',
  'exploreBookUrlRule', 'explorePaginationRule', 'bookInfoInitRule', 'bookInfoNameRule',
  'bookInfoAuthorRule', 'bookInfoCoverRule', 'bookInfoIntroRule', 'bookInfoKindRule',
  'bookInfoLatestChapterRule', 'bookInfoUpdateTimeRule', 'bookInfoWordCountRule',
  'bookInfoCanRenameRule', 'tocUrlRule', 'chapterPreUpdateJsRule', 'chapterListRule',
  'chapterNameRule', 'chapterUrlRule', 'chapterIsVolumeRule', 'chapterIsVipRule',
  'chapterUpdateTimeRule', 'nextTocUrlRule', 'contentUrlRule', 'contentRule',
  'nextContentUrlRule', 'contentWebJsRule', 'contentSourceRegex', 'contentReplaceRegex',
  'contentImageStyle',
]

export const BOOK_SOURCE_EXTRA_RULE_KEY = '__openreaderSourceExtra'

const BOOK_SOURCE_CANONICAL_KEYS = new Set([
  'bookSourceName', 'bookSourceGroup', 'bookSourceUrl', 'bookSourceType',
  'bookUrlPattern', 'ruleBookUrlPattern', 'bookSourceComment', 'enabled',
  'enabledExplore', 'searchUrl', 'exploreUrl', 'header', 'ruleSearch',
  'ruleExplore', 'ruleBookInfo', 'ruleToc', 'ruleTOC', 'ruleContent',
  'charset', 'concurrentRate', 'loginUrl', 'loginCheckJs', 'customOrder',
  'lastUpdateTime', 'weight', 'respondTime', 'rules',
])

const BOOK_SOURCE_API_KEYS = new Set([
  'id', 'userId', 'name', 'group', 'baseUrl', 'sourceType', 'comment',
  'createdAt', 'updatedAt', 'usedBookCount', 'usedBookNames',
])

const UNSAFE_EXTRA_KEYS = new Set(['__proto__', 'prototype', 'constructor'])

export function createBookSourceForm(source = {}) {
  return {
    id: source.id || null,
    name: source.name || source.bookSourceName || '',
    group: source.group || source.bookSourceGroup || '',
    baseUrl: source.baseUrl || source.bookSourceUrl || '',
    searchUrl: normalizeURLTemplate(source.searchUrl || ''),
    bookUrlPattern: source.bookUrlPattern || source.ruleBookUrlPattern || '',
    bookSourceType: Number(source.bookSourceType) || 0,
    bookSourceComment: source.bookSourceComment || '',
    charset: source.charset || 'utf-8',
    concurrentRate: source.concurrentRate || '',
    header: source.header || '',
    loginUrl: source.loginUrl || '',
    loginCheckJs: source.loginCheckJs || '',
    customOrder: Number(source.customOrder) || 0,
    lastUpdateTime: Number(source.lastUpdateTime) || 0,
    weight: Number(source.weight) || 0,
    respondTime: source.respondTime == null ? 180000 : Number(source.respondTime),
    rules: normalizeRulesText(source.rules),
    enabled: source.enabled ?? true,
    enabledExplore: source.enabledExplore ?? true,
  }
}

export function createBookSourceRuleForm(rules = {}) {
  return Object.fromEntries(BOOK_SOURCE_RULE_KEYS.map(key => [key, rules?.[key] || '']))
}

export function parseBookSourceRules(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return JSON.parse(JSON.stringify(value))
  }
  const raw = String(value || '').trim()
  if (!raw) return {}
  const parsed = JSON.parse(raw)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new SyntaxError('rules must be a JSON object')
  }
  return parsed
}

export function sourceToEditorSnapshot(source = {}) {
  const form = createBookSourceForm(source)
  const parsedRules = form.rules.trim()
    ? parseBookSourceRules(form.rules)
    : legacyBookSourceRules(source)
  const preservedExtras = normalizeSourceExtras(parsedRules[BOOK_SOURCE_EXTRA_RULE_KEY])
  delete parsedRules[BOOK_SOURCE_EXTRA_RULE_KEY]
  const sourceExtras = sourceTopLevelExtras(source)
  const extras = { ...preservedExtras, ...sourceExtras }
  const rules = Object.keys(extras).length
    ? { ...parsedRules, [BOOK_SOURCE_EXTRA_RULE_KEY]: extras }
    : parsedRules
  return { form, rules: { ...createBookSourceRuleForm(), ...rules } }
}

export function buildBookSourcePayload(form, rules) {
  const cleanRules = Object.fromEntries(Object.entries(rules || {}).filter(([, value]) => {
    if (Array.isArray(value)) return value.length > 0
    if (value && typeof value === 'object') return Object.keys(value).length > 0
    return String(value || '').trim() !== ''
  }))
  const normalized = createBookSourceForm(form)
  normalized.rules = Object.keys(cleanRules).length ? JSON.stringify(cleanRules, null, 2) : ''
  return normalized
}

export function buildReaderDevBookSource(form, rules) {
  const payload = buildBookSourcePayload(form, rules)
  const cleanRules = parseBookSourceRules(payload.rules)
  const sourceExtras = normalizeSourceExtras(cleanRules[BOOK_SOURCE_EXTRA_RULE_KEY])
  delete cleanRules[BOOK_SOURCE_EXTRA_RULE_KEY]
  const exportedRules = Object.keys(cleanRules).length
    ? JSON.stringify(cleanRules, null, 2)
    : ''

  return {
    ...sourceExtras,
    bookSourceName: payload.name,
    bookSourceGroup: payload.group,
    bookSourceUrl: payload.baseUrl,
    bookSourceType: payload.bookSourceType,
    bookUrlPattern: payload.bookUrlPattern,
    bookSourceComment: payload.bookSourceComment,
    enabled: payload.enabled,
    enabledExplore: payload.enabledExplore,
    searchUrl: exportURLTemplate(cleanRules.searchUrl || payload.searchUrl),
    exploreUrl: exportURLTemplate(cleanRules.exploreUrl),
    header: payload.header,
    ruleSearch: buildReaderDevSearchRule(cleanRules, ''),
    ruleExplore: buildReaderDevSearchRule(cleanRules, 'explore'),
    ruleBookInfo: {
      init: exportSelectorRule(cleanRules.bookInfoInitRule),
      name: exportSelectorRule(cleanRules.bookInfoNameRule),
      author: exportSelectorRule(cleanRules.bookInfoAuthorRule),
      coverUrl: exportSelectorRule(cleanRules.bookInfoCoverRule),
      intro: exportSelectorRule(cleanRules.bookInfoIntroRule),
      kind: exportSelectorRule(cleanRules.bookInfoKindRule),
      lastChapter: exportSelectorRule(cleanRules.bookInfoLatestChapterRule),
      updateTime: exportSelectorRule(cleanRules.bookInfoUpdateTimeRule),
      wordCount: exportSelectorRule(cleanRules.bookInfoWordCountRule),
      tocUrl: exportSelectorRule(cleanRules.tocUrlRule),
      canReName: exportSelectorRule(cleanRules.bookInfoCanRenameRule),
    },
    ruleToc: {
      preUpdateJs: cleanRules.chapterPreUpdateJsRule || '',
      chapterList: exportSelectorRule(cleanRules.chapterListRule),
      chapterName: exportSelectorRule(cleanRules.chapterNameRule),
      chapterUrl: exportSelectorRule(cleanRules.chapterUrlRule),
      isVolume: exportSelectorRule(cleanRules.chapterIsVolumeRule),
      isVip: exportSelectorRule(cleanRules.chapterIsVipRule),
      updateTime: exportSelectorRule(cleanRules.chapterUpdateTimeRule),
      nextTocUrl: exportSelectorRule(cleanRules.nextTocUrlRule),
    },
    ruleContent: {
      content: exportSelectorRule(cleanRules.contentRule),
      nextContentUrl: exportSelectorRule(cleanRules.nextContentUrlRule),
      webJs: cleanRules.contentWebJsRule || '',
      sourceRegex: cleanRules.contentSourceRegex || '',
      replaceRegex: cleanRules.contentReplaceRegex || '',
      imageStyle: cleanRules.contentImageStyle || '',
    },
    charset: payload.charset,
    concurrentRate: payload.concurrentRate,
    loginUrl: payload.loginUrl,
    loginCheckJs: payload.loginCheckJs,
    customOrder: payload.customOrder,
    lastUpdateTime: payload.lastUpdateTime,
    weight: payload.weight,
    respondTime: payload.respondTime,
    // Preserve OpenReader-only parser fields while exposing the complete
    // reader-dev shape, matching the canonical backend exporter.
    rules: exportedRules,
  }
}

function sourceTopLevelExtras(source) {
  if (!source || typeof source !== 'object' || Array.isArray(source)) return {}
  const extras = {}
  for (const [key, value] of Object.entries(source)) {
    if (BOOK_SOURCE_CANONICAL_KEYS.has(key) || BOOK_SOURCE_API_KEYS.has(key)) continue
    if (UNSAFE_EXTRA_KEYS.has(key) || value === undefined) continue
    extras[key] = cloneJSONValue(value)
  }
  return extras
}

function normalizeSourceExtras(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const extras = {}
  for (const [key, item] of Object.entries(value)) {
    if (BOOK_SOURCE_CANONICAL_KEYS.has(key) || UNSAFE_EXTRA_KEYS.has(key)) continue
    if (item === undefined) continue
    extras[key] = cloneJSONValue(item)
  }
  return extras
}

function cloneJSONValue(value) {
  return JSON.parse(JSON.stringify(value))
}

function normalizeRulesText(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return JSON.stringify(value, null, 2)
  }
  return String(value || '')
}

function legacyBookSourceRules(source) {
  const search = source.ruleSearch || {}
  const explore = source.ruleExplore || {}
  const info = source.ruleBookInfo || {}
  const toc = source.ruleToc || source.ruleTOC || {}
  const content = source.ruleContent || {}
  return {
    searchUrl: normalizeURLTemplate(source.searchUrl || ''),
    exploreUrl: normalizeURLTemplate(source.exploreUrl || ''),
    bookListRule: normalizeSelectorRule(search.bookList),
    bookNameRule: normalizeSelectorRule(search.name),
    bookAuthorRule: normalizeSelectorRule(search.author),
    bookCoverRule: normalizeSelectorRule(search.coverUrl),
    bookIntroRule: normalizeSelectorRule(search.intro),
    bookKindRule: normalizeSelectorRule(search.kind),
    bookWordCountRule: normalizeSelectorRule(search.wordCount),
    latestChapterRule: normalizeSelectorRule(search.lastChapter),
    bookUpdateTimeRule: normalizeSelectorRule(search.updateTime),
    bookUrlRule: normalizeSelectorRule(search.bookUrl),
    exploreBookListRule: normalizeSelectorRule(explore.bookList),
    exploreBookNameRule: normalizeSelectorRule(explore.name),
    exploreBookAuthorRule: normalizeSelectorRule(explore.author),
    exploreBookCoverRule: normalizeSelectorRule(explore.coverUrl),
    exploreBookIntroRule: normalizeSelectorRule(explore.intro),
    exploreBookKindRule: normalizeSelectorRule(explore.kind),
    exploreBookWordCountRule: normalizeSelectorRule(explore.wordCount),
    exploreLatestChapterRule: normalizeSelectorRule(explore.lastChapter),
    exploreBookUpdateTimeRule: normalizeSelectorRule(explore.updateTime),
    exploreBookUrlRule: normalizeSelectorRule(explore.bookUrl),
    bookInfoInitRule: normalizeSelectorRule(info.init),
    bookInfoNameRule: normalizeSelectorRule(info.name),
    bookInfoAuthorRule: normalizeSelectorRule(info.author),
    bookInfoCoverRule: normalizeSelectorRule(info.coverUrl),
    bookInfoIntroRule: normalizeSelectorRule(info.intro),
    bookInfoKindRule: normalizeSelectorRule(info.kind),
    bookInfoLatestChapterRule: normalizeSelectorRule(info.lastChapter),
    bookInfoUpdateTimeRule: normalizeSelectorRule(info.updateTime),
    bookInfoWordCountRule: normalizeSelectorRule(info.wordCount),
    bookInfoCanRenameRule: normalizeSelectorRule(info.canReName),
    tocUrlRule: normalizeSelectorRule(info.tocUrl),
    chapterPreUpdateJsRule: toc.preUpdateJs || '',
    chapterListRule: normalizeSelectorRule(toc.chapterList),
    chapterNameRule: normalizeSelectorRule(toc.chapterName),
    chapterUrlRule: normalizeSelectorRule(toc.chapterUrl),
    chapterIsVolumeRule: normalizeSelectorRule(toc.isVolume),
    chapterIsVipRule: normalizeSelectorRule(toc.isVip),
    chapterUpdateTimeRule: normalizeSelectorRule(toc.updateTime),
    nextTocUrlRule: normalizeSelectorRule(toc.nextTocUrl),
    contentRule: normalizeSelectorRule(content.content),
    nextContentUrlRule: normalizeSelectorRule(content.nextContentUrl),
    contentWebJsRule: content.webJs || '',
    contentSourceRegex: content.sourceRegex || '',
    contentReplaceRegex: content.replaceRegex || '',
    contentImageStyle: content.imageStyle || '',
  }
}

function normalizeURLTemplate(value) {
  return String(value || '')
    .trim()
    .replaceAll('{{key}}', '{keyword}')
    .replaceAll('{{keyword}}', '{keyword}')
    .replaceAll('{{page}}', '{page}')
}

function exportURLTemplate(value) {
  return normalizeURLTemplate(value)
    .replaceAll('{keyword}', '{{key}}')
    .replaceAll('{page}', '{{page}}')
}

function buildReaderDevSearchRule(rules, prefix) {
  const key = suffix => prefix ? `${prefix}${suffix[0].toUpperCase()}${suffix.slice(1)}` : suffix
  return {
    bookList: exportSelectorRule(rules[key('bookListRule')]),
    name: exportSelectorRule(rules[key('bookNameRule')]),
    author: exportSelectorRule(rules[key('bookAuthorRule')]),
    coverUrl: exportSelectorRule(rules[key('bookCoverRule')]),
    intro: exportSelectorRule(rules[key('bookIntroRule')]),
    kind: exportSelectorRule(rules[key('bookKindRule')]),
    wordCount: exportSelectorRule(rules[key('bookWordCountRule')]),
    lastChapter: exportSelectorRule(rules[key('latestChapterRule')]),
    updateTime: exportSelectorRule(rules[key('bookUpdateTimeRule')]),
    bookUrl: exportSelectorRule(rules[key('bookUrlRule')]),
  }
}

function exportSelectorRule(value) {
  const rule = String(value || '').trim()
  const index = rule.indexOf('|')
  if (index <= 0 || index === rule.length - 1) return rule
  const selector = rule.slice(0, index).trim()
  const operation = rule.slice(index + 1).trim()
  if (operation === 'text' || operation === 'html') return `${selector}@${operation}`
  if (operation.startsWith('attr:')) {
    const attribute = operation.slice('attr:'.length).trim()
    if (attribute) return `${selector}@${attribute}`
  }
  return rule
}

function normalizeSelectorRule(value) {
  const rule = String(value || '').trim()
  if (!rule || rule.startsWith('/') || rule.toLowerCase().startsWith('@js:')) return rule
  const index = rule.lastIndexOf('@')
  if (index <= 0 || index === rule.length - 1) return rule
  const selector = rule.slice(0, index).trim()
  const operation = rule.slice(index + 1).trim()
  if (!selector || !operation || /[ /|@[\](){}]/.test(operation)) return rule
  if (['text', 'html'].includes(operation.toLowerCase())) return `${selector}|${operation.toLowerCase()}`
  return `${selector}|attr:${operation}`
}
