#!/usr/bin/env node

import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { execFile, spawn } from 'node:child_process'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const configuredTarget = String(process.env.TARGET_URL || '').trim().replace(/\/$/, '')
const backendPort = Number(process.env.OPENREADER_SOURCE_PARSER_PORT || 18088)

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function loadPlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch (error) {
    const bundled = '/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js'
    try {
      const module = await import(bundled)
      return module.chromium ? module : module.default
    } catch {
      throw new Error(`Playwright is required for the source parser workflow smoke: ${error.message}`)
    }
  }
}

async function fixtureText(name) {
  return readFile(join(backendDir, 'engine', 'testdata', 'source_compat', name), 'utf8')
}

async function startFixtureServer() {
  const fixtures = Object.fromEntries(await Promise.all([
    'books.html',
    'book_detail.html',
    'toc.html',
    'toc-2.html',
    'chapter.html',
    'content.html',
    'content-2.html',
    'books.json',
    'book_detail.json',
    'toc.json',
    'toc-2.json',
    'chapter.json',
    'content.json',
    'content-2.json',
  ].map(async name => [name, await fixtureText(name)])))

  const pages = new Map([
    ['/css/search', ['books.html', 'text/html; charset=utf-8']],
    ['/xpath/search', ['books.html', 'text/html; charset=utf-8']],
    ['/books/one', ['book_detail.html', 'text/html; charset=utf-8']],
    ['/books/two', ['book_detail.html', 'text/html; charset=utf-8']],
    ['/toc.html', ['toc.html', 'text/html; charset=utf-8']],
    ['/toc-2.html', ['toc-2.html', 'text/html; charset=utf-8']],
    ['/chapters/xpath-1', ['chapter.html', 'text/html; charset=utf-8']],
    ['/chapters/xpath-2', ['chapter.html', 'text/html; charset=utf-8']],
    ['/chapters/xpath-3', ['chapter.html', 'text/html; charset=utf-8']],
    ['/content.html', ['content.html', 'text/html; charset=utf-8']],
    ['/content-2.html', ['content-2.html', 'text/html; charset=utf-8']],
    ['/json/search', ['books.json', 'application/json; charset=utf-8']],
    ['/json/one', ['book_detail.json', 'application/json; charset=utf-8']],
    ['/json/two', ['book_detail.json', 'application/json; charset=utf-8']],
    ['/toc.json', ['toc.json', 'application/json; charset=utf-8']],
    ['/toc-2.json', ['toc-2.json', 'application/json; charset=utf-8']],
    ['/chapters/json-1', ['chapter.json', 'application/json; charset=utf-8']],
    ['/chapters/json-2', ['chapter.json', 'application/json; charset=utf-8']],
    ['/chapters/json-3', ['chapter.json', 'application/json; charset=utf-8']],
    ['/content.json', ['content.json', 'application/json; charset=utf-8']],
    ['/content-2.json', ['content-2.json', 'application/json; charset=utf-8']],
  ])
  const server = createServer((request, response) => {
    const path = new URL(request.url || '/', 'http://fixture.local').pathname
    if (path.startsWith('/covers/')) {
      response.writeHead(204)
      response.end()
      return
    }
    const page = pages.get(path)
    if (!page) {
      response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
      response.end(`missing source fixture: ${path}`)
      return
    }
    response.writeHead(200, { 'Content-Type': page[1], 'Cache-Control': 'no-store' })
    response.end(fixtures[page[0]])
  })
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'source fixture server did not bind an address')
  return {
    root: `http://127.0.0.1:${address.port}`,
    close: () => new Promise(resolve => server.close(resolve)),
  }
}

async function waitForHealth(root, processOutput) {
  const deadline = Date.now() + 60_000
  let lastError = null
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
      lastError = new Error(`health returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  throw new Error(`OpenReader test server did not start: ${lastError?.message || 'unknown error'}\n${processOutput()}`)
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) {
    child.kill('SIGKILL')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 2_000))])
  }
}

async function startOpenReader() {
  if (configuredTarget) return { root: configuredTarget, close: async () => {} }
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-source-parser-'))
  const binary = join(tempRoot, 'openreader')
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: process.env,
    maxBuffer: 4 * 1024 * 1024,
  })
  let output = ''
  const child = spawn(binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${backendPort}`,
      OPENREADER_DATA_DIR: join(tempRoot, 'data'),
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(tempRoot, 'data', 'openreader.db'),
      OPENREADER_PUBLIC_DIR: join(rootDir, 'frontend', 'dist'),
      OPENREADER_JWT_SECRET: 'source-parser-workflow-smoke-secret',
      OPENREADER_CORS_ORIGIN: `http://127.0.0.1:${backendPort}`,
      OPENREADER_CHECK_INTERVAL: '24h',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', chunk => { output += chunk.toString() })
  child.stderr.on('data', chunk => { output += chunk.toString() })
  const root = `http://127.0.0.1:${backendPort}`
  try {
    await waitForHealth(root, () => output)
  } catch (error) {
    await stopProcess(child)
    await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
  return {
    root,
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function api(root, path, { token = '', method = 'GET', body } = {}) {
  const response = await fetch(`${root}/api${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  if (!response.ok) {
    throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  }
  return data
}

function cssRules(fixtureRoot) {
  return {
    searchUrl: `${fixtureRoot}/css/search?q={keyword}`,
    bookListRule: '@CSS:article.book',
    bookNameRule: '@CSS:.name@text',
    bookURLRule: '@CSS:.detail@href',
    bookKindRule: '@CSS:.kind@text',
    bookInfoInitRule: '@CSS:#detail',
    bookInfoNameRule: '@CSS:h1@text',
    bookInfoAuthorRule: '@CSS:.author@text',
    bookInfoCoverRule: '@CSS:.cover@data-src',
    bookInfoIntroRule: '@CSS:.intro@text',
    bookInfoKindRule: '@CSS:.kind@text',
    bookInfoLatestChapterRule: '@CSS:.latest@text',
    bookInfoUpdateTimeRule: '@CSS:.updated@text',
    bookInfoWordCountRule: '@CSS:.words@text',
    tocUrlRule: '@CSS:#toc@href',
    chapterListRule: '@CSS:li.chapter',
    chapterNameRule: '@CSS:a@text',
    chapterURLRule: '@CSS:a@href',
    chapterIsVIPRule: '@CSS:.vip@text',
    chapterUpdateTimeRule: '@CSS:.updated@text',
    nextTocUrlRule: '@CSS:#next@href',
    contentUrlRule: '@CSS:#content@href',
    contentRule: '@CSS:main#content@text',
    nextContentUrlRule: '@CSS:#next@href',
  }
}

function jsonPathRules(fixtureRoot) {
  return {
    searchUrl: `${fixtureRoot}/json/search?q={keyword}`,
    bookListRule: '$.data.books[*]',
    bookNameRule: '$.name',
    bookURLRule: '$.url',
    bookKindRule: '$.kinds[*]',
    bookInfoInitRule: '$.book',
    bookInfoNameRule: '$.title',
    bookInfoAuthorRule: '$.author',
    bookInfoCoverRule: '$.cover',
    bookInfoIntroRule: '$.intro',
    bookInfoKindRule: '$.kinds[*]',
    bookInfoLatestChapterRule: '$.latest',
    bookInfoUpdateTimeRule: '$.updated',
    bookInfoWordCountRule: '$.words',
    tocUrlRule: '$.book.toc',
    chapterListRule: '$.chapters[*]',
    chapterNameRule: '$.title',
    chapterURLRule: '$.url',
    chapterIsVIPRule: '$.vip',
    chapterUpdateTimeRule: '$.updated',
    nextTocUrlRule: '$.next',
    contentUrlRule: '$.contentUrl',
    contentRule: '$.payload.body',
    nextContentUrlRule: '$.next',
  }
}

function xpathRules(fixtureRoot) {
  return {
    searchUrl: `${fixtureRoot}/xpath/search?q={keyword}`,
    bookListRule: "@XPath://article[@class='book']",
    bookNameRule: '@XPath:.//h2/text()',
    bookURLRule: '@XPath:.//a/@href',
    bookKindRule: '@XPath:.//span[@class=\'kind\']/text()',
    bookInfoInitRule: "@XPath://section[@id='detail']",
    bookInfoNameRule: '@XPath:.//h1/text()',
    bookInfoAuthorRule: "@XPath:.//span[@class='author']/text()",
    bookInfoCoverRule: "@XPath:.//img[@class='cover']/@data-src",
    bookInfoIntroRule: "@XPath:.//p[@class='intro']/text()",
    bookInfoKindRule: "@XPath:.//span[@class='kind']/text()",
    bookInfoLatestChapterRule: "@XPath:.//span[@class='latest']/text()",
    bookInfoUpdateTimeRule: "@XPath:.//span[@class='updated']/text()",
    bookInfoWordCountRule: "@XPath:.//span[@class='words']/text()",
    tocUrlRule: "@XPath://a[@id='toc']/@href",
    chapterListRule: "@XPath://li[@class='chapter']",
    chapterNameRule: '@XPath:.//a/text()',
    chapterURLRule: '@XPath:.//a/@href',
    chapterIsVIPRule: "@XPath:.//span[@class='vip']/text()",
    chapterUpdateTimeRule: "@XPath:.//span[@class='updated']/text()",
    nextTocUrlRule: "@XPath://a[@id='next']/@href",
    contentUrlRule: "@XPath://a[@id='content']/@href",
    contentRule: "@XPath://main[@id='content']/text()",
    nextContentUrlRule: "@XPath://a[@id='next']/@href",
  }
}

async function createSources(root, token, fixtureRoot) {
  const definitions = [
    {
      mode: 'CSS',
      searchTitle: '第一本书',
      detailAuthor: 'XPath 作者',
      chapterTitle: 'XPath 第一章',
      content: ['XPath 正文第一页', 'XPath 正文第二页'],
      rules: cssRules(fixtureRoot),
    },
    {
      mode: 'JSONPath',
      searchTitle: 'JSON 第一书',
      detailAuthor: 'JSON 作者',
      chapterTitle: 'JSON 第一章',
      content: ['JSON 正文第一页', 'JSON 正文第二页'],
      rules: jsonPathRules(fixtureRoot),
    },
    {
      mode: 'XPath',
      searchTitle: '第一本书',
      detailAuthor: 'XPath 作者',
      chapterTitle: 'XPath 第一章',
      content: ['XPath 正文第一页', 'XPath 正文第二页'],
      rules: xpathRules(fixtureRoot),
    },
  ]
  for (const definition of definitions) {
    const source = await api(root, '/sources', {
      token,
      method: 'POST',
      body: {
        name: `浏览器解析夹具-${definition.mode}`,
        baseUrl: fixtureRoot,
        searchUrl: definition.rules.searchUrl,
        charset: 'utf-8',
        enabled: true,
        enabledExplore: false,
        rules: JSON.stringify(definition.rules),
      },
    })
    assert(Number.isInteger(source?.id) && source.id > 0, `${definition.mode}: source creation did not return an id`)
    definition.sourceId = source.id
  }
  return definitions
}

async function assertWorkflow(browser, root, token, definition) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  await page.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  const searchURL = `${root}/?workspace=search&q=${encodeURIComponent('解析验收')}&searchType=single&sourceId=${definition.sourceId}&concurrent=1`
  await page.goto(searchURL, { waitUntil: 'networkidle' })
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 15_000 })
  const result = page.locator('.workspace-result-page .result-card').first()
  await result.getByText(definition.searchTitle, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })

  await result.locator('.book-cover-shared').click()
  const bookInfo = page.locator('.book-info-dialog')
  await bookInfo.waitFor({ state: 'visible', timeout: 10_000 })
  await bookInfo.getByText(definition.searchTitle, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  await bookInfo.locator('.el-dialog__headerbtn').click()
  await bookInfo.waitFor({ state: 'hidden', timeout: 10_000 })

  await result.locator('.result-main').click()
  await page.waitForURL(/\/reader\/remote\/[a-f0-9]+\?chapter=0/, { timeout: 15_000 })
  await page.locator('.reader-body').waitFor({ state: 'visible', timeout: 15_000 })
  await page.getByText(definition.content[0], { exact: false }).waitFor({ state: 'visible', timeout: 15_000 })
  await page.getByText(definition.content[1], { exact: false }).waitFor({ state: 'visible', timeout: 15_000 })

  const sessionID = new URL(page.url()).pathname.split('/').filter(Boolean).at(-1)
  assert(sessionID, `${definition.mode}: missing remote reader session id`)
  const session = await api(root, `/reader/remote-sessions/${sessionID}`, { token })
  // reader-dev only lets detail data rename a search title when canReName says so.
  // The fixtures intentionally omit that optional rule, so title must retain the
  // search result while author proves the real detail document was parsed.
  assert(session?.book?.title === definition.searchTitle, `${definition.mode}: remote title = ${session?.book?.title}, want ${definition.searchTitle}`)
  assert(session?.book?.author === definition.detailAuthor, `${definition.mode}: detail author = ${session?.book?.author}, want ${definition.detailAuthor}`)
  assert(Array.isArray(session?.chapters) && session.chapters.length === 3, `${definition.mode}: expected three parsed chapters`)
  assert(session.chapters[0]?.title === definition.chapterTitle, `${definition.mode}: first chapter = ${session.chapters[0]?.title}, want ${definition.chapterTitle}`)

  await page.locator('.reader-left-rail button[title="目录"]').click()
  const toc = page.locator('.reader-desktop-workspace.workspace-panel-toc')
  await toc.waitFor({ state: 'visible', timeout: 10_000 })
  assert(await toc.locator('.toc-item').count() === 3, `${definition.mode}: reader directory did not render three chapters`)
  await toc.getByText(definition.chapterTitle, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  assert(failures.length === 0, `${definition.mode}: ${failures.join('\n')}`)
  await context.close()
  return definition.mode
}

async function run() {
  const fixture = await startFixtureServer()
  const app = await startOpenReader()
  try {
    const registered = await api(app.root, '/auth/register', {
      method: 'POST',
      body: { username: 'parser-browser-smoke', password: 'source-parser-browser' },
    })
    const token = registered?.token
    assert(token, 'source parser browser smoke registration did not return a token')
    const definitions = await createSources(app.root, token, fixture.root)
    const { chromium } = await loadPlaywright()
    const browser = await chromium.launch({
      headless: true,
      executablePath: process.env.CHROME_PATH || defaultChromePath,
    })
    try {
      const completed = []
      for (const definition of definitions) completed.push(await assertWorkflow(browser, app.root, token, definition))
      console.log(`source-parser-workflow: ok ${completed.join(', ')} realApi=true fixtureOnly=true searchBookInfoTocContent=true`)
    } finally {
      await browser.close()
    }
  } finally {
    await app.close()
    await fixture.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
