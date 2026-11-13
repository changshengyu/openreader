import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

// Source contract for docs/compat/reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md R7:
// the OpenReader-owned "重新加载" buttons (main error block and continuous inline error block) must sit above
// the desktop tap zones and must not let a touch or click fall through to the reader page handlers.

const RETRY_BLOCKS = [
  { blockClass: 'chapter-load-error', clickHandler: "emit('reload')" },
  { blockClass: 'chapter-inline-error', clickHandler: "emit('retry-block', block.index)" },
]

function section(source, tag) {
  const match = source.match(new RegExp(`<${tag}\\b[^>]*>([\\s\\S]*)</${tag}>`))
  assert(match, `missing <${tag}> section`)
  return match[1]
}

function retryButtonAttributes(template, blockClass) {
  const start = template.indexOf(`class="${blockClass}"`)
  assert.notEqual(start, -1, `${blockClass} block is missing`)
  const rest = template.slice(start)
  const button = rest.match(/<button\b((?:[^>"']|"[^"]*"|'[^']*')*)>\s*重新加载\s*<\/button>/)
  assert(button, `${blockClass} has no 重新加载 button`)
  const between = rest.slice(0, button.index)
  for (const other of RETRY_BLOCKS.map(row => row.blockClass).filter(name => name !== blockClass)) {
    assert(!between.includes(`class="${other}"`), `${blockClass} 重新加载 button resolved inside ${other}`)
  }
  return button[1].trim()
}

function eventBinding(attributes, event) {
  const match = attributes.match(new RegExp(`(?:@|v-on:)${event}((?:\\.[a-z]+)*)(?:="([^"]*)")?`))
  return match ? { modifiers: match[1].split('.').filter(Boolean), handler: match[2] ?? null } : null
}

function cssRules(css) {
  const rules = []
  const source = css.replace(/\/\*[\s\S]*?\*\//g, '')
  const pattern = /([^{}]+)\{([^{}]*)\}/g
  let match
  while ((match = pattern.exec(source))) {
    const selectors = match[1].split(',').map(value => value.trim().replace(/\s+/g, ' ')).filter(Boolean)
    const declarations = Object.fromEntries(match[2].split(';')
      .map(value => value.split(':'))
      .filter(parts => parts.length >= 2)
      .map(([name, ...value]) => [name.trim(), value.join(':').trim()]))
    rules.push({ selectors, declarations })
  }
  return rules
}

function declarationsFor(css, selectorMatches) {
  return cssRules(css)
    .filter(rule => rule.selectors.some(selectorMatches))
    .reduce((merged, rule) => ({ ...merged, ...rule.declarations }), {})
}

function integerZIndex(value, label) {
  assert.match(String(value ?? ''), /^-?\d+$/, `${label} z-index must be an integer, got ${JSON.stringify(value)}`)
  return Number(value)
}

async function readSources() {
  const [contentSource, zonesSource, readerSource] = await Promise.all([
    readFile(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/reader/ReaderClickZones.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8'),
  ])
  return { contentSource, zonesSource, readerSource }
}

for (const { blockClass, clickHandler } of RETRY_BLOCKS) {
  test(`${blockClass} 重新加载 stops click and touch from reaching the reader page handlers`, async () => {
    const { contentSource } = await readSources()
    const attributes = retryButtonAttributes(section(contentSource, 'template'), blockClass)
    const click = eventBinding(attributes, 'click')
    assert(click, `${blockClass} 重新加载 has no click handler (${attributes})`)
    assert(click.modifiers.includes('stop'), `${blockClass} 重新加载 click is missing .stop (${attributes})`)
    assert.equal(click.handler, clickHandler, `${blockClass} 重新加载 click must still retry`)
    for (const event of ['touchstart', 'touchend']) {
      const touch = eventBinding(attributes, event)
      assert(touch?.modifiers.includes('stop'), `${blockClass} 重新加载 is missing @${event}.stop (${attributes})`)
      assert(!touch.modifiers.includes('prevent'), `${blockClass} 重新加载 @${event} must not cancel the tap's click`)
    }
  })

  test(`${blockClass} 重新加载 sits above the desktop tap zones and below the dimming layer`, async () => {
    const { contentSource, zonesSource, readerSource } = await readSources()
    const zones = declarationsFor(section(zonesSource, 'style'), selector => /^\.reader-tap-zones(?:\.[\w-]+)*$/.test(selector))
    const tapZoneZ = integerZIndex(zones['z-index'], '.reader-tap-zones')
    const dimming = declarationsFor(section(readerSource, 'style'), selector => selector === '.reader-page::after')
    const dimmingZ = integerZIndex(dimming['z-index'], '.reader-page::after')
    const selector = `.${blockClass} button`
    const button = declarationsFor(section(contentSource, 'style'), value => value === selector)
    assert.match(String(button.position ?? ''), /^(relative|absolute|fixed|sticky)$/, `${selector} needs a position, got ${JSON.stringify(button.position)}`)
    const buttonZ = integerZIndex(button['z-index'], selector)
    assert(buttonZ > tapZoneZ, `${selector} z-index ${buttonZ} must be above .reader-tap-zones ${tapZoneZ}`)
    assert(buttonZ < dimmingZ, `${selector} z-index ${buttonZ} must stay below the .reader-page::after dimming layer ${dimmingZ}`)
  })
}

// R1-R2 (sections 7 and 13): the main error block is reader-dev's error chapter page: the target chapter <h3>,
// the label line, the reason line and then 重新加载; the continuous inline block carries the same label line.
// Everything is text interpolation, never v-html.

function blockMarkup(template, blockClass) {
  const start = template.indexOf(`class="${blockClass}"`)
  assert.notEqual(start, -1, `${blockClass} block is missing`)
  const end = template.indexOf('</button>', start)
  assert.notEqual(end, -1, `${blockClass} block has no closing button`)
  return template.slice(start, end)
}

function propDefinition(script, name) {
  const match = script.match(new RegExp(`\\b${name}\\s*:\\s*\\{([^}]*)\\}`))
  return match ? match[1] : null
}

test('chapter-load-error renders the target chapter title, the label and the reason in reader-dev order', async () => {
  const { contentSource } = await readSources()
  const block = blockMarkup(section(contentSource, 'template'), 'chapter-load-error')
  const heading = block.match(/<h3\b([^>]*)>\s*\{\{\s*errorTitle\s*\}\}\s*<\/h3>/)
  assert(heading, `chapter-load-error must start with <h3 data-pos="0">{{ errorTitle }}</h3>: ${block}`)
  assert.match(heading[1], /\bdata-pos="0"/, 'the error page title is the chapter heading at position 0')
  const label = block.search(/<p\b[^>]*>\s*\{\{\s*errorLabel\s*\}\}\s*<\/p>/)
  const reason = block.search(/<p\b[^>]*>\s*\{\{\s*error\s*\}\}\s*<\/p>/)
  assert.notEqual(label, -1, `chapter-load-error has no {{ errorLabel }} line: ${block}`)
  assert.notEqual(reason, -1, `chapter-load-error has no {{ error }} reason line: ${block}`)
  assert(heading.index < label && label < reason, 'the title, the label and the reason must come in reader-dev order')
  assert(!/v-html/.test(block), 'error text must not be rendered with v-html')

  const script = section(contentSource, 'script')
  const titleProp = propDefinition(script, 'errorTitle')
  assert(titleProp, 'ReaderChapterContent needs an errorTitle prop')
  assert.match(titleProp, /default:\s*''/, 'an unknown catalog row shows no title text')
  const labelProp = propDefinition(script, 'errorLabel')
  assert(labelProp, 'ReaderChapterContent needs an errorLabel prop')
  assert.match(labelProp, /default:\s*'获取章节内容失败！'/, 'the label defaults to the content failure line')
})

test('chapter-inline-error renders the upstream label line between the title and the reason', async () => {
  const { contentSource } = await readSources()
  const block = blockMarkup(section(contentSource, 'template'), 'chapter-inline-error')
  const heading = block.search(/<h3\b[^>]*>/)
  const label = block.search(/<p\b[^>]*>\s*获取章节内容失败！\s*<\/p>/)
  const reason = block.search(/<p\b[^>]*>\s*\{\{\s*block\.error\s*\}\}\s*<\/p>/)
  assert.notEqual(label, -1, `chapter-inline-error has no 获取章节内容失败！ line: ${block}`)
  assert.notEqual(reason, -1, `chapter-inline-error has no {{ block.error }} reason line: ${block}`)
  assert(heading !== -1 && heading < label && label < reason, 'the title, the label and the reason must come in reader-dev order')
  assert(!/v-html/.test(block), 'error text must not be rendered with v-html')
})

test('Reader passes the catalog title and the failure label to the error page', async () => {
  const { readerSource } = await readSources()
  const element = section(readerSource, 'template').match(/<ReaderChapterContent\b((?:[^>"']|"[^"]*"|'[^']*')*)\/>/)
  assert(element, 'Reader.vue must render <ReaderChapterContent />')
  assert.match(element[1], /:error-title="[^"]+"/, 'Reader.vue must bind :error-title')
  assert.match(element[1], /:error-label="[^"]+"/, 'Reader.vue must bind :error-label')
})
