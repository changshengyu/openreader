import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

// Source contract for docs/compat/reader-desktop-panel-content-controls-fixed-baseline-second-audit-p0-contract.md
// P3/P4 (section 8.1): with a desktop workspace panel open, a press on an exposed content control (加载下一章, the
// main error block 重新加载, the continuous inline 重新加载) must run the control and close the panel, like every
// upstream control outside the click popover. The controls stop click/touch propagation (R7, mobile click
// contract), so no document-level outside listener can close the panel: each handler must go through
// runWithDesktopWorkspaceClosed, which leaves the mobile primary panels alone.

const ATTRIBUTES = String.raw`((?:[^>"']|"[^"]*"|'[^']*')*)`

const CONTROLS = [
  {
    label: '.reader-chapter-end 加载下一章',
    element: new RegExp(String.raw`<button\b(?=${ATTRIBUTES.slice(1, -1)}\bclass="reader-chapter-end")${ATTRIBUTES}>`),
    event: 'click',
    // Wrapped in the binding: the chapter-end handler itself, not its result.
    wrapped: () => /^\s*handleChapterEndNext\s*$|^\s*\(\s*\)\s*=>\s*handleChapterEndNext\(\s*\)\s*$/,
    // Wrapped in a handler: both outcomes of a press (last-chapter hint, next chapter) run with the panel closed.
    actions: [/showReaderToast\(\s*'本章是最后一章'\s*\)/, /\bgoChapter\(\s*currentIndex\.value\s*\+\s*1\s*\)/],
  },
  {
    label: '<ReaderChapterContent> @reload (main error block 重新加载)',
    element: new RegExp(String.raw`<ReaderChapterContent\b${ATTRIBUTES}\/?>`),
    event: 'reload',
    wrapped: () => /^\s*reloadChapter\s*$|^\s*\(\s*\)\s*=>\s*reloadChapter\(\s*\)\s*$/,
    actions: [/\breloadChapter\b/],
  },
  {
    label: '<ReaderChapterContent> @retry-block (continuous inline 重新加载)',
    element: new RegExp(String.raw`<ReaderChapterContent\b${ATTRIBUTES}\/?>`),
    event: 'retry-block',
    // The emitted chapter index ($event, or the binding's arrow parameter) must reach the retry.
    wrapped: index => new RegExp(String.raw`^\s*\(\s*\)\s*=>\s*retryContinuousChapter\(\s*${index.replace('$', '\\$')}\s*\)\s*$`),
    actions: [/\bretryContinuousChapter\(\s*[A-Za-z_$][\w$]*\s*\)/],
  },
]

function section(source, tag) {
  const match = source.match(new RegExp(`<${tag}\\b[^>]*>([\\s\\S]*)</${tag}>`))
  assert(match, `missing <${tag}> section`)
  return match[1]
}

function scriptSetup(source) {
  const match = source.match(/<script\b[^>]*\bsetup\b[^>]*>([\s\S]*?)<\/script>/)
  assert(match, 'missing <script setup> section')
  return match[1]
}

function oneLine(text) {
  return text.trim().replace(/\s+/g, ' ')
}

function eventBinding(attributes, event) {
  const name = event.replace(/[.*+?^${}()|[\]\\-]/g, '\\$&')
  const match = attributes.match(new RegExp(`(?:@|v-on:)${name}((?:\\.[a-z]+)*)(?:="([^"]*)")?(?=[\\s/>]|$)`))
  return match ? { modifiers: match[1].split('.').filter(Boolean), handler: match[2] ?? null } : null
}

// Index just past the bracket that closes the one at `open`, skipping strings, template literals and comments.
function matchBracket(script, open) {
  const pairs = { '(': ')', '{': '}' }
  const stack = []
  for (let index = open; index < script.length; index += 1) {
    const char = script[index]
    if (char === '/' && script[index + 1] === '/') {
      index = script.indexOf('\n', index)
      if (index < 0) return -1
      continue
    }
    if (char === '/' && script[index + 1] === '*') {
      index = script.indexOf('*/', index + 2) + 1
      if (index <= 0) return -1
      continue
    }
    if (char === '\'' || char === '"' || char === '`') {
      for (index += 1; index < script.length && script[index] !== char; index += 1) {
        if (script[index] === '\\') index += 1
      }
      continue
    }
    if (pairs[char]) stack.push(pairs[char])
    else if (char === ')' || char === '}') {
      if (stack.pop() !== char) return -1
      if (!stack.length) return index + 1
    }
  }
  return -1
}

// Source of `function name(...) { ... }` or `const name = (...) => ...` in the script, or null when the name is
// not a function declared in Reader.vue (for example a composable's destructured return value).
function functionSource(script, name) {
  const declared = script.match(new RegExp(String.raw`(?:^|[^\w$.])(function\s+${name}\s*\(|(?:const|let)\s+${name}\s*=\s*(?:async\s*)?(?:\(|[A-Za-z_$][\w$]*\s*=>))`))
  if (!declared) return null
  const start = declared.index + declared[0].length - declared[1].length
  let cursor = start + declared[1].length
  if (declared[1].endsWith('(')) {
    cursor = matchBracket(script, cursor - 1)
    if (cursor < 0) return null
  }
  const arrow = script.slice(cursor).match(/^\s*(?:=>\s*)?/)
  cursor += arrow[0].length
  if (script[cursor] === '{') {
    const end = matchBracket(script, cursor)
    return end < 0 ? null : script.slice(start, end)
  }
  const end = script.indexOf('\n', cursor)
  return script.slice(start, end < 0 ? script.length : end)
}

function bodyOf(source) {
  const open = source.indexOf('{')
  return open < 0 ? source : source.slice(open)
}

// A press closes the desktop workspace before the control acts when the binding itself is
// runWithDesktopWorkspaceClosed(<the control's action>), or when it names a Reader.vue function whose body runs
// every outcome of the control through runWithDesktopWorkspaceClosed (or calls closeDesktopWorkspace() first).
function closingRoute(script, control, handler) {
  const arrow = handler.match(/^\s*(?:\(\s*([A-Za-z_$][\w$]*)?\s*\)|([A-Za-z_$][\w$]*))\s*=>\s*([\s\S]*)$/)
  const expression = arrow ? arrow[3] : handler
  const wrappedBinding = expression.match(/^\s*runWithDesktopWorkspaceClosed\(([\s\S]*)\)\s*$/)
  if (wrappedBinding) {
    return control.wrapped(arrow ? (arrow[1] || arrow[2] || '') : '$event').test(wrappedBinding[1])
      ? { ok: true }
      : { ok: false, why: `the binding wraps ${oneLine(wrappedBinding[1])}, which is not the control's action (with its arguments)` }
  }
  const name = handler.trim()
  if (!/^[A-Za-z_$][\w$]*$/.test(name)) {
    return { ok: false, why: 'the binding neither is runWithDesktopWorkspaceClosed(...) nor names a handler' }
  }
  const source = functionSource(script, name)
  if (!source) return { ok: false, why: `${name} is bound directly and is not a Reader.vue function, so nothing closes the panel` }
  const body = bodyOf(source)
  const missing = control.actions.filter(action => !action.test(body))
  if (missing.length) return { ok: false, why: `${name} no longer runs ${missing.map(String).join(', ')}` }

  const routed = body.match(/\brunWithDesktopWorkspaceClosed\(/)
  if (routed) {
    const argumentEnd = matchBracket(body, routed.index + routed[0].length - 1)
    if (argumentEnd < 0) return { ok: false, why: `${name} has an unbalanced runWithDesktopWorkspaceClosed(...) call` }
    const argument = body.slice(routed.index + routed[0].length, argumentEnd - 1)
    const inner = /^\s*[A-Za-z_$][\w$]*\s*$/.test(argument) ? bodyOf(functionSource(script, argument.trim()) || '') : argument
    const outside = control.actions.filter(action => !action.test(inner))
    return outside.length
      ? { ok: false, why: `${name} runs ${outside.map(String).join(', ')} outside runWithDesktopWorkspaceClosed` }
      : { ok: true }
  }
  const closeAt = body.search(/\bcloseDesktopWorkspace\(\)/)
  if (closeAt >= 0 && control.actions.every(action => closeAt < body.search(action))) return { ok: true }
  return { ok: false, why: `${name} never closes the desktop workspace before it acts` }
}

async function readReader() {
  const source = await readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
  return { template: section(source, 'template'), script: scriptSetup(source) }
}

for (const control of CONTROLS) {
  test(`P3/P4 ${control.label} closes the desktop workspace and still runs its action`, async () => {
    const { template, script } = await readReader()
    const element = template.match(control.element)
    assert(element, `${control.label} element is missing from Reader.vue`)
    const binding = eventBinding(element[1], control.event)
    assert(binding?.handler, `${control.label} has no @${control.event} handler (${oneLine(element[1])})`)
    const route = closingRoute(script, control, binding.handler)
    const written = `@${[control.event, ...binding.modifiers].join('.')}="${binding.handler}"`
    assert(route.ok, `${control.label} ${written} does not close the desktop workspace: ${route.why}`)
  })
}

test('runWithDesktopWorkspaceClosed closes every desktop panel first, never the mobile ones, then runs the action', async () => {
  const { script } = await readReader()
  const runner = functionSource(script, 'runWithDesktopWorkspaceClosed')
  assert(runner, 'runWithDesktopWorkspaceClosed is missing from Reader.vue')
  assert.match(
    runner,
    /^function runWithDesktopWorkspaceClosed\((\w+)\)\s*\{\s*if\s*\(\s*!isMobileReader\.value\s*\)\s*closeDesktopWorkspace\(\)\s*;?\s*return\s+\1(?:\?\.)?\(\)\s*;?\s*\}$/,
    `runWithDesktopWorkspaceClosed must close the desktop workspace only when !isMobileReader, before the action:\n${runner}`,
  )
  const close = functionSource(script, 'closeDesktopWorkspace')
  assert(close, 'closeDesktopWorkspace is missing from Reader.vue')
  for (const drawer of ['showShelfDrawer', 'showSourceDrawer', 'showTocDrawer', 'showSettingsDrawer']) {
    assert.match(close, new RegExp(`\\b${drawer}\\.value\\s*=\\s*false\\b`), `closeDesktopWorkspace must close ${drawer}`)
  }
})

test('加载下一章 keeps stopping click and touch from reaching the reader page', async () => {
  const { template } = await readReader()
  const element = template.match(CONTROLS[0].element)
  assert(element, '.reader-chapter-end button is missing from Reader.vue')
  for (const event of ['click', 'touchstart', 'touchend']) {
    const binding = eventBinding(element[1], event)
    assert(binding?.modifiers.includes('stop'), `.reader-chapter-end is missing @${event}.stop (${oneLine(element[1])})`)
    assert(!binding.modifiers.includes('prevent'), `.reader-chapter-end @${event} must not cancel the tap's click`)
  }
})
