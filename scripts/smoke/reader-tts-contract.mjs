#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const viewport = {
  width: Number(process.env.SMOKE_VIEWPORT_WIDTH || 390),
  height: Number(process.env.SMOKE_VIEWPORT_HEIGHT || 844),
}
const isMobileViewport = viewport.width <= 750

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
          mode: 'flip',
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
  const browser = await openSmokeBrowser()
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
    window.__openreaderTTS = {
      speakCalls: 0,
      cancelCalls: 0,
      pauseCalls: 0,
      resumeCalls: 0,
      lastText: '',
      lastUtterance: null,
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
        window.__openreaderTTS.lastText = utterance.text
        window.__openreaderTTS.lastUtterance = utterance
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
  const ttsTrigger = isMobileViewport
    ? '.reader-mobile-float-tools button[title="朗读"]'
    : '.reader-right-rail button[title="朗读"]'
  if (isMobileViewport) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, 'mobile reader tools should be visible by default')
  } else {
    assert(await page.locator('.reader-left-rail').isVisible(), 'desktop reader rail should remain visible before opening TTS')
  }
  if (isMobileViewport) {
    assert(await page.locator('.reader-shell.flip').count() === 1, 'mobile fixture must enter the flip-reading branch before opening TTS')
  } else {
    assert(await page.locator('.reader-shell.page').count() === 1, 'desktop reader should retain its normalized page branch before opening TTS')
  }
  await page.locator(ttsTrigger).click()
  await page.waitForSelector('.tts-bar', { timeout: 10000 })
  if (isMobileViewport) {
    assert(await page.locator('.reader-shell.page').count() === 1, 'opening the TTS bar must leave the upstream slide-reading branch')
  }
  const readerPaddingTarget = isMobileViewport ? '.reader-body' : '.reader-content'
  const expandedPadding = await page.locator(readerPaddingTarget).evaluate(element => getComputedStyle(element).paddingBottom)
  assert(Number.parseFloat(expandedPadding) >= 280, `expanded TTS bar should reserve 280px, got ${expandedPadding}`)
  if (isMobileViewport) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'opening the upstream TTS bar should hide mobile reader tools')
    await page.mouse.click(viewport.width / 2, viewport.height / 2)
    await page.waitForTimeout(100)
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'content taps while the TTS bar is open must not toggle mobile reader tools')
  } else {
    assert(await page.locator('.reader-left-rail').isVisible(), 'opening the TTS bar must not hide desktop reader rails')
  }

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
  await page.getByText('下一段').click()
  await page.waitForFunction(() => window.__openreaderTTS.speakCalls === 2, null, { timeout: 10000 })
  await page.waitForFunction(() => (
    document.querySelector('.tts-active')?.innerText || ''
  ).includes('春风过处'), null, { timeout: 10000 })
  const afterNext = await page.evaluate(() => ({
    lastText: window.__openreaderTTS.lastText,
    activeText: document.querySelector('.tts-active')?.innerText || '',
  }))
  assert(afterNext.lastText.includes('春风过处'), `next paragraph should speak the first body paragraph, got ${afterNext.lastText}`)
  assert(afterNext.activeText.includes('春风过处'), `next paragraph should highlight active DOM paragraph, got ${afterNext.activeText}`)

  await page.evaluate(() => {
    const event = new Event('error')
    Object.defineProperty(event, 'error', { value: 'mock-error' })
    window.__openreaderTTS.lastUtterance.dispatchEvent(event)
  })
  await page.waitForFunction(() => document.body.innerText.includes('朗读错误'), null, { timeout: 10000 })

  await page.locator('button[title="展开/收起朗读设置"]').click()
  await page.waitForFunction(() => !document.querySelector('.tts-config'), null, { timeout: 10000 })
  const collapsedPadding = await page.locator(readerPaddingTarget).evaluate(element => getComputedStyle(element).paddingBottom)
  assert(Number.parseFloat(collapsedPadding) >= 80 && Number.parseFloat(collapsedPadding) < 100, `collapsed TTS bar should reserve 80px, got ${collapsedPadding}`)

  await page.locator('.tts-close').click()
  await page.waitForFunction(() => !document.querySelector('.tts-bar'), null, { timeout: 10000 })
  if (isMobileViewport) {
    assert(await page.locator('.reader-shell.flip').count() === 1, 'closing TTS should restore the configured flip-reading branch')
  } else {
    assert(await page.locator('.reader-shell.page').count() === 1, 'closing TTS should retain the desktop page branch')
  }
  if (isMobileViewport) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'closing the TTS bar should not reopen mobile reader tools')
    await page.mouse.click(viewport.width / 2, viewport.height / 2)
    await page.waitForFunction(() => document.querySelector('.reader-mobile-top.visible'), null, { timeout: 10000 })
  }
  const afterClose = await page.evaluate(() => window.__openreaderTTS)
  assert(afterClose.cancelCalls >= 1, 'closing TTS bar should stop active speech')
  assert(failures.length === 0, `browser errors:\n${failures.join('\n')}`)
  await browser.close()
  console.log(`reader TTS contract smoke passed (${viewport.width}x${viewport.height})`)
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
