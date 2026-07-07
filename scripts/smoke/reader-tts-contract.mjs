#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

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
      console.error('Playwright is required for reader TTS contract smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

async function installApiMocks(page) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') {
      return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    }
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-07T00:00:00Z',
        value: {
          mode: 'scroll',
          pageMode: 'normal',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          ttsRate: 1,
          ttsPitch: 1,
          ttsVoiceURI: 'zh-cn',
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-07T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') {
      return route.fulfill(json({
        id: 1,
        title: '朗读契约测试',
        author: 'OpenReader',
        sourceId: 2,
        chapterCount: 1,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json([{ id: 11, index: 0, title: '第一章' }]))
    }
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '第一章' },
        content: [
          '春风过处，纸页微明。',
          '朗读栏应先显示控制和语音配置。',
          '点击播放以后才调用 speechSynthesis。',
        ].join('\n'),
      }))
    }
    if (path === '/books/1/bookmarks') {
      return route.fulfill(json([]))
    }
    if (path === '/progress/1') {
      return route.fulfill(json({}))
    }
    if (path === '/sources') {
      return route.fulfill(json([{ id: 2, name: '测试书源', enabled: true }]))
    }
    if (path === '/categories') {
      return route.fulfill(json([]))
    }
    return route.fulfill(json({}))
  })
}

async function main() {
  const playwright = await loadPlaywright()
  const browser = await playwright.chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
  })
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
    window.__openreaderTTS = {
      speakCalls: 0,
      cancelCalls: 0,
      pauseCalls: 0,
      resumeCalls: 0,
    }
    class MockSpeechSynthesisUtterance extends EventTarget {
      constructor(text) {
        super()
        this.text = text
        this.rate = 1
        this.pitch = 1
        this.voice = null
      }
    }
    Object.defineProperty(window, 'SpeechSynthesisUtterance', {
      configurable: true,
      value: MockSpeechSynthesisUtterance,
    })
    const mockSpeechSynthesis = {
      speaking: false,
      paused: false,
      getVoices() {
        return [
          { name: 'English', lang: 'en-US', voiceURI: 'en-us' },
          { name: '中文', lang: 'zh-CN', voiceURI: 'zh-cn' },
        ]
      },
      addEventListener() {},
      removeEventListener() {},
      speak(utterance) {
        window.__openreaderTTS.speakCalls += 1
        this.speaking = true
        this.paused = false
        setTimeout(() => utterance.dispatchEvent(new Event('start')), 0)
      },
      cancel() {
        window.__openreaderTTS.cancelCalls += 1
        this.speaking = false
        this.paused = false
      },
      pause() {
        window.__openreaderTTS.pauseCalls += 1
        this.paused = true
      },
      resume() {
        window.__openreaderTTS.resumeCalls += 1
        this.paused = false
      },
    }
    Object.defineProperty(window, 'speechSynthesis', {
      configurable: true,
      value: mockSpeechSynthesis,
    })
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installApiMocks(page)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 10000 })
  await page.locator('.reader-mobile-float-tools button[title="朗读"]').click()
  await page.waitForSelector('.tts-bar', { timeout: 10000 })

  const afterOpen = await page.evaluate(() => ({
    speakCalls: window.__openreaderTTS.speakCalls,
    barText: document.querySelector('.tts-bar')?.innerText || '',
    voiceOptions: Array.from(document.querySelectorAll('.tts-select option')).map(option => option.textContent.trim()),
  }))
  assert(afterOpen.speakCalls === 0, `opening TTS bar must not start speech, got ${afterOpen.speakCalls}`)
  assert(afterOpen.barText.includes('语音库'), 'TTS bar should expose voice library config')
  assert(afterOpen.barText.includes('上一段') && afterOpen.barText.includes('下一段'), 'TTS bar should expose paragraph controls')
  assert(afterOpen.voiceOptions.some(label => label.includes('中文')), 'TTS bar should list Chinese voice')

  await page.locator('.tts-play').click()
  await page.waitForFunction(() => window.__openreaderTTS.speakCalls === 1, null, { timeout: 10000 })
  await page.locator('.tts-close').click()
  await page.waitForFunction(() => !document.querySelector('.tts-bar'), null, { timeout: 10000 })
  const afterClose = await page.evaluate(() => window.__openreaderTTS)
  assert(afterClose.cancelCalls >= 1, 'closing TTS bar should stop active speech')
  assert(failures.length === 0, `browser errors:\n${failures.join('\n')}`)
  await browser.close()
  console.log('reader TTS contract smoke passed')
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
