import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test, { after } from 'node:test'
import { createServer } from 'vite'

const storage = new Map()
globalThis.localStorage = {
  get length() { return storage.size },
  getItem: key => storage.get(String(key)) ?? null,
  key: index => [...storage.keys()][index] ?? null,
  removeItem: key => storage.delete(String(key)),
  setItem: (key, value) => storage.set(String(key), String(value)),
}
globalThis.window = {
  localStorage: globalThis.localStorage,
  sessionStorage: globalThis.localStorage,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {},
}

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
const { useBookshelfStore } = await vite.ssrLoadModule('/src/stores/bookshelf.js')
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')

const bookshelfSource = readFileSync(new URL('../src/stores/bookshelf.js', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')
const readerShelfSource = readFileSync(new URL('../src/composables/useReaderShelf.js', import.meta.url), 'utf8')

test('one Pinia action owns the upstream manual remote refresh transaction', () => {
  assert.match(bookshelfSource, /import\s*\{[^}]*checkBookUpdates[^}]*\}\s*from\s*['"]\.\.\/api\/books['"]/)
  assert.match(bookshelfSource, /let manualRefreshRequest = null/)
  assert.match(
    bookshelfSource,
    /async refreshFromSources\([^)]*\)[\s\S]*?checkBookUpdates\([\s\S]*?replacedBookIds[\s\S]*?clearBookBrowserChapterCache[\s\S]*?loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/,
  )
  assert.match(bookshelfSource, /if \(manualRefreshRequest\) return manualRefreshRequest/)
})

test('Home and Reader shelf reuse the store action instead of implementing two refresh paths', () => {
  assert.match(homeSource, /bookshelf\.refreshFromSources\(\)/)
  assert.match(readerShelfSource, /options\.bookshelf\.refreshFromSources\(\)/)
  assert.doesNotMatch(homeSource, /async function refreshShelf\(\)[\s\S]*?bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
  assert.doesNotMatch(readerShelfSource, /async function refresh\(\)[\s\S]*?bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
})

test('partial remote failures remain visible while the authoritative shelf still commits', () => {
  assert.match(homeSource, /failed[\s\S]*?书架已刷新，\$\{[^}]+\} 本书检查失败/)
  assert.match(readerShelfSource, /failed[\s\S]*?本书检查失败/)
  assert.match(bookshelfSource, /catch \(error\)\s*\{\s*checkError = error[\s\S]*?loadBooks\(\{ force: true, all: true, settleProgress: true \}\)[\s\S]*?throw checkError/)
})

test('ordinary initial, focus and sync shelf loads do not trigger remote source checks', () => {
  const directCalls = [...bookshelfSource.matchAll(/checkBookUpdates\(/g)]
  assert.equal(directCalls.length, 1)
  assert.doesNotMatch(homeSource, /checkBookUpdates\(/)
  assert.doesNotMatch(readerShelfSource, /checkBookUpdates\(/)
})

function axiosResponse(config, data) {
  return { data, status: 200, statusText: 'OK', headers: {}, config, request: {} }
}

test('simultaneous visible refreshes share one check and commit the following shelf snapshot', async () => {
  setActivePinia(createPinia())
  const bookshelf = useBookshelfStore()
  const originalAdapter = api.defaults.adapter
  const order = []
  let releaseCheck
  const checkGate = new Promise(resolve => { releaseCheck = resolve })
  api.defaults.adapter = async (config) => {
    order.push(`${config.method} ${config.url}`)
    if (config.url === '/books/check-updates') {
      await checkGate
      return axiosResponse(config, { checked: 2, updated: 1, failed: 1, newChapters: 0, replacedBookIds: [7] })
    }
    if (config.url === '/books') {
      return axiosResponse(config, [{ id: 7, title: '刷新后的书', chapterCount: 2, lastChapter: '新第二章' }])
    }
    throw new Error(`unexpected API ${config.method} ${config.url}`)
  }
  try {
    const first = bookshelf.refreshFromSources()
    const second = bookshelf.refreshFromSources()
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.equal(order.filter(item => item === 'post /books/check-updates').length, 1)
    releaseCheck()
    const [firstResult, secondResult] = await Promise.all([first, second])
    assert.deepEqual(firstResult, secondResult)
    assert.equal(firstResult.failed, 1)
    assert.deepEqual(order, ['post /books/check-updates', 'get /books'])
    assert.equal(bookshelf.books[0].lastChapter, '新第二章')
  } finally {
    api.defaults.adapter = originalAdapter
  }
})

test('a failed remote check still performs the authoritative shelf read before rejecting', async () => {
  setActivePinia(createPinia())
  const bookshelf = useBookshelfStore()
  const originalAdapter = api.defaults.adapter
  const order = []
  api.defaults.adapter = async (config) => {
    order.push(`${config.method} ${config.url}`)
    if (config.url === '/books/check-updates') throw new Error('remote check unavailable')
    if (config.url === '/books') return axiosResponse(config, [{ id: 8, title: '仍可用的权威书架' }])
    throw new Error(`unexpected API ${config.method} ${config.url}`)
  }
  try {
    await assert.rejects(bookshelf.refreshFromSources(), /remote check unavailable/)
    assert.deepEqual(order, ['post /books/check-updates', 'get /books'])
    assert.equal(bookshelf.books[0].title, '仍可用的权威书架')
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
