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

function chapterText(prefix, count) {
  return Array.from({ length: count }, (_, index) => (
    `${prefix}第${index + 1}段。这里使用足够长的正文验证朗读段落、分页定位与跨章节事务不会因为加载较慢而中断。`
  )).join('\n')
}

async function installApiMocks(page, state = {}) {
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
          ttsVoiceURI: '',
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      state.settingsWrites ||= []
      state.settingsWrites.push(request.postDataJSON?.() || {})
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-07T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') {
      return route.fulfill(json({
        id: 1,
        title: '朗读契约测试',
        author: 'OpenReader',
        sourceId: 2,
        chapterCount: 2,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
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
    if (path === '/books/1/chapters/1/content') {
      state.secondChapterRequestCount = (state.secondChapterRequestCount || 0) + 1
      if (state.secondChapterRequestCount === 1) {
        // Reader preloads its neighbor after the first chapter. Reject that
        // background attempt so the TTS transition must exercise a fresh,
        // deliberately slow foreground chapter transaction.
        return route.fulfill(json({ error: 'fixture preload miss' }, 503))
      }
      state.secondChapterRequestedAt = Date.now()
      await new Promise(resolve => setTimeout(resolve, 4100))
      state.secondChapterFulfilledAt = Date.now()
      return route.fulfill(json({
        chapter: { id: 12, index: 1, title: '第二章' },
        content: chapterText('第二章', 14),
      }))
    }
    if (path === '/books/1/bookmarks') return route.fulfill(json([]))
    if (path === '/progress/1') return route.fulfill(json({}))
    if (path === '/sources') return route.fulfill(json([{ id: 2, name: '测试书源', enabled: true }]))
    if (path === '/categories') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function installSpeechMock(context) {
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
}

async function finishCurrentUtterance(page) {
  await page.evaluate(() => {
    window.speechSynthesis.speaking = false
    window.__openreaderTTS.lastUtterance.dispatchEvent(new Event('end'))
  })
}

async function verifyIncompleteSpeechAPI(browser) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
    Object.defineProperty(window, 'speechSynthesis', {
      configurable: true,
      value: {},
    })
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(error.message))
  await installApiMocks(page)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 10_000 })
  assert(await page.locator('button[title="朗读"]').count() === 0, `${viewport.width}: incomplete speech API must hide the TTS entry`)
  assert(failures.length === 0, `${viewport.width}: incomplete speech API crashed Reader:\n${failures.join('\n')}`)
  await context.close()
}

async function verifyTTSContract(browser) {
  const context = await browser.newContext({ viewport })
  await installSpeechMock(context)
  const page = await context.newPage()
  const failures = []
  const state = {}
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    if (text.includes('status of 503') && state.secondChapterRequestCount === 1) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installApiMocks(page, state)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 10_000 })
  const ttsTrigger = isMobileViewport
    ? '.reader-mobile-float-tools button[title="朗读"]'
    : '.reader-right-rail button[title="朗读"]'

  if (isMobileViewport) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, 'mobile reader tools should be visible by default')
    assert(await page.locator('.reader-shell.flip').count() === 1, 'mobile fixture must enter flip mode before opening TTS')
  } else {
    assert(await page.locator('.reader-left-rail').isVisible(), 'desktop reader rail should remain visible before opening TTS')
    assert(await page.locator('.reader-shell.page').count() === 1, 'desktop reader should normalize flip to page before opening TTS')
  }

  await page.locator(ttsTrigger).click()
  await page.waitForSelector('.tts-bar', { timeout: 10_000 })
  if (isMobileViewport) {
    assert(await page.locator('.reader-shell.page').count() === 1, 'opening TTS must leave the flip branch')
  }

  const geometry = await page.evaluate(() => {
    const bar = document.querySelector('.tts-bar')?.getBoundingClientRect()
    const frame = document.querySelector('.reader-page')?.getBoundingClientRect()
    return {
      bar: bar && { bottom: bar.bottom, left: bar.left, right: bar.right, width: bar.width },
      frame: frame && { right: frame.right },
      viewportHeight: window.innerHeight,
      viewportWidth: window.innerWidth,
    }
  })
  assert(Math.abs(geometry.bar.bottom - geometry.viewportHeight) <= 1, `${viewport.width}: TTS bar must attach to the bottom: ${JSON.stringify(geometry)}`)
  if (isMobileViewport) {
    assert(Math.abs(geometry.bar.left) <= 1 && Math.abs(geometry.bar.width - geometry.viewportWidth) <= 1, `${viewport.width}: mobile TTS bar must be full-width: ${JSON.stringify(geometry)}`)
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'opening TTS should hide mobile reader tools')
    await page.mouse.click(viewport.width / 2, Math.round(viewport.height / 2))
    await page.waitForTimeout(100)
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'content taps while TTS is open must not toggle mobile tools')
  } else {
    assert(Math.abs(geometry.bar.width - 500) <= 1, `desktop TTS bar must be 500px wide: ${JSON.stringify(geometry)}`)
    assert(Math.abs(geometry.bar.right - geometry.frame.right) <= 1, `desktop TTS bar must align to the Reader frame: ${JSON.stringify(geometry)}`)
    assert(await page.locator('.reader-left-rail').isVisible(), 'opening TTS must not hide desktop reader rails')
  }

  const readerPaddingTarget = isMobileViewport ? '.reader-body' : '.reader-content'
  const expandedPadding = await page.locator(readerPaddingTarget).evaluate(element => getComputedStyle(element).paddingBottom)
  assert(Number.parseFloat(expandedPadding) >= 280, `expanded TTS bar should reserve 280px, got ${expandedPadding}`)
  assert(await page.locator('.tts-voice-option').count() === 2, 'TTS voice radio list should expose both voices')
  assert(await page.locator('.tts-voice-option').filter({ hasText: '中文' }).count() === 1, 'TTS voice list should include Chinese')
  assert(await page.locator('.tts-config .reader-setting-stepper').count() === 3, 'rate, pitch, and timer must use numeric steppers')
  assert(await page.locator('.tts-config input[type="range"], .tts-config select').count() === 0, 'TTS config must not restore sliders or select controls')
  assert(await page.evaluate(() => window.__openreaderTTS.speakCalls) === 0, 'opening TTS must not start speech')

  await page.locator('.tts-play').click()
  await page.waitForFunction(() => document.body.innerText.includes('请先选择语音库'))
  assert(await page.evaluate(() => window.__openreaderTTS.speakCalls) === 0, 'TTS must not silently choose a browser voice')

  const rateRow = page.locator('.tts-row').filter({ hasText: '语速' })
  await rateRow.locator('.reader-setting-stepper-value').click()
  await rateRow.locator('.reader-setting-stepper-input').fill('1.4')
  await rateRow.locator('.reader-setting-stepper-input').press('Enter')
  await page.locator('.tts-voice-option').filter({ hasText: '中文' }).click()
  await page.locator('.tts-play').click()
  await page.waitForFunction(() => window.__openreaderTTS.speakCalls === 1)
  const firstSpeech = await page.evaluate(() => ({
    rate: window.__openreaderTTS.lastUtterance.rate,
    voice: window.__openreaderTTS.lastUtterance.voice?.voiceURI || '',
  }))
  assert(firstSpeech.rate === 1.4 && firstSpeech.voice === 'zh-cn', `selected TTS settings were not applied: ${JSON.stringify(firstSpeech)}`)

  let expectedSpeakCalls = 1
  for (let attempt = 0; attempt < 6 && !state.secondChapterRequestedAt; attempt += 1) {
    const before = await page.evaluate(() => window.__openreaderTTS.speakCalls)
    await finishCurrentUtterance(page)
    await page.waitForTimeout(160)
    if (state.secondChapterRequestedAt) break
    await page.waitForFunction(previous => window.__openreaderTTS.speakCalls > previous, before)
  }
  assert(state.secondChapterRequestedAt, `${viewport.width}: finishing the first chapter did not begin the adjacent chapter transaction`)
  await page.waitForFunction(() => window.__openreaderTTS.lastText.includes('第二章'), null, { timeout: 10_000 })
  expectedSpeakCalls = await page.evaluate(() => window.__openreaderTTS.speakCalls)
  assert(
    state.secondChapterFulfilledAt - state.secondChapterRequestedAt >= 4000,
    `${viewport.width}: delayed TTS fixture did not exceed the old 3.6s timeout`,
  )
  assert(Date.now() - state.secondChapterRequestedAt >= 4000, `${viewport.width}: TTS did not wait for the real delayed chapter transaction`)

  const nextButton = page.locator('.tts-main').getByRole('button', { name: '下一段' })
  for (let index = 0; index < 8; index += 1) {
    await nextButton.click()
    expectedSpeakCalls += 1
    await page.waitForFunction(expected => window.__openreaderTTS.speakCalls === expected, expectedSpeakCalls)
  }
  await page.waitForFunction(() => document.querySelector('.tts-active')?.innerText?.includes('第二章第8段'))
  const frozenParagraphText = await page.locator('.tts-active').innerText()

  await page.evaluate(() => {
    window.speechSynthesis.speaking = false
    const event = new Event('error')
    Object.defineProperty(event, 'error', { value: 'mock-error' })
    window.__openreaderTTS.lastUtterance.dispatchEvent(event)
  })
  await page.waitForFunction(() => document.body.innerText.includes('朗读错误'))

  await page.locator('button[title="展开/收起朗读设置"]').click()
  await page.waitForFunction(() => !document.querySelector('.tts-config'))
  const collapsedPadding = await page.locator(readerPaddingTarget).evaluate(element => getComputedStyle(element).paddingBottom)
  assert(Number.parseFloat(collapsedPadding) >= 80 && Number.parseFloat(collapsedPadding) < 100, `collapsed TTS bar should reserve 80px, got ${collapsedPadding}`)

  await page.locator('.tts-close').click()
  await page.waitForFunction(() => !document.querySelector('.tts-bar'))
  if (isMobileViewport) {
    await page.waitForFunction(() => document.querySelector('.reader-shell.flip'))
    await page.waitForFunction((text) => {
      const paragraph = [...document.querySelectorAll('.reader-body p')].find(element => element.innerText === text)
      const viewportRect = document.querySelector('.reader-content')?.getBoundingClientRect()
      const paragraphRect = paragraph?.getBoundingClientRect()
      if (!viewportRect || !paragraphRect) return false
      return paragraphRect.left >= viewportRect.left - 1 && paragraphRect.right <= viewportRect.right + 1
    }, frozenParagraphText, { timeout: 10_000 })
    const restored = await page.evaluate((text) => {
      const paragraph = [...document.querySelectorAll('.reader-body p')].find(element => element.innerText === text)
      const viewportRect = document.querySelector('.reader-content')?.getBoundingClientRect()
      const paragraphRect = paragraph?.getBoundingClientRect()
      const transform = getComputedStyle(document.querySelector('.reader-body')).transform
      return {
        paragraph: paragraphRect && { left: paragraphRect.left, right: paragraphRect.right },
        transform,
        transformX: transform === 'none' ? 0 : new DOMMatrix(transform).m41,
        viewport: viewportRect && { left: viewportRect.left, right: viewportRect.right },
      }
    }, frozenParagraphText)
    assert(Math.abs(restored.transformX) > 1, `mobile flip close should restore a later page: ${JSON.stringify(restored)}`)
    assert(
      restored.paragraph.left >= restored.viewport.left - 1 && restored.paragraph.right <= restored.viewport.right + 1,
      `mobile flip close did not restore the active TTS paragraph: ${JSON.stringify(restored)}`,
    )
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, 'closing TTS must not reopen mobile tools')
    await page.mouse.click(viewport.width / 2, Math.round(viewport.height / 2))
    await page.waitForFunction(() => document.querySelector('.reader-mobile-top.visible'))
  } else {
    assert(await page.locator('.reader-shell.page').count() === 1, 'desktop TTS close should retain the normalized page branch')
  }

  const afterClose = await page.evaluate(() => window.__openreaderTTS)
  assert(afterClose.cancelCalls >= 1, 'closing TTS should stop active speech')
  assert(state.settingsWrites?.length >= 1, 'voice and numeric TTS changes should use persisted reader settings')
  const savedSettings = state.settingsWrites.at(-1)?.value || {}
  assert(savedSettings.ttsVoiceURI === 'zh-cn' && savedSettings.ttsRate === 1.4, `persisted TTS settings are incomplete: ${JSON.stringify(savedSettings)}`)
  assert(failures.length === 0, `${viewport.width}: browser errors:\n${failures.join('\n')}`)
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    await verifyIncompleteSpeechAPI(browser)
    await verifyTTSContract(browser)
    console.log(`reader TTS contract smoke passed (${viewport.width}x${viewport.height})`)
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
