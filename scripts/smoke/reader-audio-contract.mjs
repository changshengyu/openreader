#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0&offset=37`

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

async function installMocks(page, requests) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/media\/audio-\d+\.mp3.*$/, route => route.fulfill({
    status: 200,
    contentType: 'audio/mpeg',
    body: Buffer.from([0x49, 0x44, 0x33, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]),
  }))
  await page.route(/^https?:\/\/[^/]+\/media\/cover\.svg.*$/, route => route.fulfill({
    status: 200,
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="320"><rect width="240" height="320" fill="#345"/><text x="36" y="170" font-size="32" fill="#fff">Audio</text></svg>',
  }))
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    requests.push(`${method} ${path}`)
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-07T00:00:00Z',
        value: {
          mode: 'scroll2',
          pageMode: 'normal',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          animateDuration: 0,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-07T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') {
      return route.fulfill(json({
        id: 1,
        title: '音频契约测试',
        author: 'OpenReader',
        sourceId: 2,
        type: 1,
        coverUrl: `${targetUrl.replace(/\/$/, '')}/media/cover.svg`,
        chapterCount: 2,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一集' },
        { id: 12, index: 1, title: '第二集' },
      ]))
    }
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '第一集' },
        content: `${targetUrl.replace(/\/$/, '')}/media/audio-0.mp3`,
        format: 'audio',
        resourceUrl: `${targetUrl.replace(/\/$/, '')}/media/audio-0.mp3`,
        resourceExpiresAt: '2026-07-07T12:00:00Z',
      }))
    }
    if (path === '/books/1/chapters/1/content') {
      return route.fulfill(json({
        chapter: { id: 12, index: 1, title: '第二集' },
        content: `${targetUrl.replace(/\/$/, '')}/media/audio-1.mp3`,
        format: 'audio',
        resourceUrl: `${targetUrl.replace(/\/$/, '')}/media/audio-1.mp3`,
        resourceExpiresAt: '2026-07-07T12:00:00Z',
      }))
    }
    if (path === '/books/1/bookmarks') return route.fulfill(json([]))
    if (path === '/progress/1') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') {
      const body = request.postDataJSON?.() || {}
      return route.fulfill(json({ ...body, bookId: 1 }))
    }
    if (path === '/sources') return route.fulfill(json([{ id: 2, name: '音频书源', enabled: true }]))
    if (path === '/categories') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
    window.localStorage.setItem('reader', JSON.stringify({
      mode: 'scroll2',
      pageMode: 'normal',
      settingsScope: 'user:1',
      progressScope: 'user:1',
    }))
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  const requests = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installMocks(page, requests)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-audio-content audio', { state: 'attached', timeout: 10_000 })

  const initial = await page.evaluate(() => ({
    shellClass: document.querySelector('.reader-shell')?.className || '',
    audioTitle: document.querySelector('.reader-audio-book-title')?.textContent || '',
    audioBookInfo: document.querySelector('.reader-audio-author')?.textContent || '',
    hasTextBlocks: Boolean(document.querySelector('.reader-body [data-reader-block]')),
    hasChapterContent: Boolean(document.querySelector('.chapter-content')),
    hasAutoReading: Boolean(document.querySelector('[title="自动阅读"]')),
    hasTTS: Boolean(document.querySelector('[title="朗读"]')),
    hasNativeControls: document.querySelector('.reader-audio-content audio')?.hasAttribute('controls') || false,
    hasCover: Boolean(document.querySelector('.reader-audio-cover img')),
    hasProgressSlider: Boolean(document.querySelector('input[aria-label="音频播放进度"]')),
    hasVolumeSlider: Boolean(document.querySelector('input[aria-label="音频音量"]')),
    hasMobilePageSlider: Boolean(document.querySelector('.reader-mobile-bottom .mobile-progress-slider')),
    mobileFooterHeight: Math.round(document.querySelector('.reader-mobile-bottom.visible')?.getBoundingClientRect().height || 0),
    hasPlayButton: Boolean([...document.querySelectorAll('.reader-audio-actions button')].find(button => button.textContent.includes('播放'))),
    hasMuteButton: Boolean([...document.querySelectorAll('.reader-audio-volume button')].find(button => button.textContent.includes('音量'))),
    mobileTopVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
  }))
  assert(initial.shellClass.includes('page'), `${viewport.width}: audio reader should force page mode, class=${initial.shellClass}`)
  assert(initial.audioTitle.includes('第一集'), `${viewport.width}: audio title missing: ${initial.audioTitle}`)
  assert(initial.audioBookInfo.includes('音频契约测试') && initial.audioBookInfo.includes('OpenReader'), `${viewport.width}: audio book/author info missing: ${initial.audioBookInfo}`)
  assert(!initial.hasTextBlocks, `${viewport.width}: audio should not render text reader blocks`)
  assert(!initial.hasChapterContent, `${viewport.width}: audio should not render ordinary chapter-content sections`)
  assert(!initial.hasAutoReading, `${viewport.width}: audio should hide auto-reading control`)
  assert(!initial.hasTTS, `${viewport.width}: audio should hide TTS control`)
  assert(!initial.hasNativeControls, `${viewport.width}: audio should not expose native controls as primary UI`)
  assert(initial.hasCover, `${viewport.width}: audio cover should render`)
  assert(initial.hasProgressSlider, `${viewport.width}: custom audio progress slider missing`)
  assert(initial.hasVolumeSlider, `${viewport.width}: custom audio volume slider missing`)
  if (viewport.width <= 750) {
    assert(!initial.hasMobilePageSlider, `${viewport.width}: audio reader must hide the text page slider`)
    assert(initial.mobileFooterHeight > 0 && initial.mobileFooterHeight <= 64, `${viewport.width}: audio footer should collapse to one tool row (${initial.mobileFooterHeight}px)`)
  }
  assert(initial.hasPlayButton, `${viewport.width}: custom play button missing`)
  assert(initial.hasMuteButton, `${viewport.width}: custom mute/volume button missing`)

  const firstChapterRequestsBeforeBoundary = requests.filter(item => item === 'GET /books/1/chapters/0/content').length
  await page.locator('.reader-audio-actions.primary').getByRole('button', { name: '上一章' }).click()
  await page.waitForFunction(() => document.body.innerText.includes('本章是第一章'))
  assert(
    requests.filter(item => item === 'GET /books/1/chapters/0/content').length === firstChapterRequestsBeforeBoundary,
    `${viewport.width}: first-chapter boundary must not issue another content request`,
  )

  await page.evaluate(() => {
    window.__openreaderAudio = { attemptCalls: 0, playCalls: 0, rejectNext: false }
    const audio = document.querySelector('.reader-audio-content audio')
    Object.defineProperty(audio, 'duration', { configurable: true, value: 120 })
    audio.play = () => {
      window.__openreaderAudio.attemptCalls += 1
      if (window.__openreaderAudio.rejectNext) {
        window.__openreaderAudio.rejectNext = false
        return Promise.reject(new DOMException('autoplay blocked', 'NotAllowedError'))
      }
      window.__openreaderAudio.playCalls += 1
      audio.dispatchEvent(new Event('play'))
      return Promise.resolve()
    }
    audio.pause = () => {
      audio.dispatchEvent(new Event('pause'))
    }
    audio.dispatchEvent(new Event('loadedmetadata'))
  })
  const restored = await page.evaluate(() => Math.round(document.querySelector('.reader-audio-content audio').currentTime || 0))
  assert(restored === 37, `${viewport.width}: saved playback second was not restored (${restored})`)
  await page.getByRole('button', { name: '播放' }).click()
  await page.waitForFunction(() => [...document.querySelectorAll('.reader-audio-actions button')].some(button => button.textContent.includes('暂停')))
  await page.getByRole('button', { name: '暂停' }).click()
  await page.waitForFunction(() => [...document.querySelectorAll('.reader-audio-actions button')].some(button => button.textContent.includes('播放')))
  await page.locator('input[aria-label="音频播放进度"]').evaluate((input) => {
    input.value = '45'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(new Event('change', { bubbles: true }))
  })
  const seekState = await page.evaluate(() => ({
    audioTime: Math.round(document.querySelector('.reader-audio-content audio').currentTime || 0),
    sliderValue: document.querySelector('input[aria-label="音频播放进度"]').value,
  }))
  assert(seekState.audioTime === 45, `${viewport.width}: seek did not update audio currentTime: ${JSON.stringify(seekState)}`)
  await page.getByRole('button', { name: '+15s' }).click()
  assert(await page.evaluate(() => Math.round(document.querySelector('.reader-audio-content audio').currentTime || 0)) === 60, `${viewport.width}: +15s did not seek audio`)
  await page.getByRole('button', { name: '-15s' }).click()
  assert(await page.evaluate(() => Math.round(document.querySelector('.reader-audio-content audio').currentTime || 0)) === 45, `${viewport.width}: -15s did not seek audio`)
  await page.locator('input[aria-label="音频音量"]').evaluate((input) => {
    input.value = '35'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  let volumeState = await page.evaluate(() => ({
    audioVolume: Math.round((document.querySelector('.reader-audio-content audio').volume || 0) * 100),
    label: document.querySelector('.reader-audio-volume span')?.textContent || '',
  }))
  assert(volumeState.audioVolume === 35 && volumeState.label.includes('35%'), `${viewport.width}: volume slider failed: ${JSON.stringify(volumeState)}`)
  await page.getByRole('button', { name: '音量' }).click()
  volumeState = await page.evaluate(() => ({
    audioMuted: document.querySelector('.reader-audio-content audio').muted,
    label: document.querySelector('.reader-audio-volume span')?.textContent || '',
  }))
  assert(volumeState.audioMuted && volumeState.label.includes('0%'), `${viewport.width}: mute failed: ${JSON.stringify(volumeState)}`)

  let playCallsBeforeTransition = await page.evaluate(() => window.__openreaderAudio.playCalls)
  if (viewport.width === 390) {
    await page.evaluate(() => {
      window.__openreaderAudio.rejectNext = true
    })
  }
  await page.locator('.reader-audio-actions.primary').getByRole('button', { name: '下一章' }).click()
  await page.waitForFunction(() => document.querySelector('.reader-audio-book-title')?.textContent?.includes('第二集'))
  let transition = await page.evaluate(() => ({
    autoplay: document.querySelector('.reader-audio-content audio')?.autoplay,
    src: document.querySelector('.reader-audio-content audio')?.src || '',
    playCalls: window.__openreaderAudio.playCalls,
  }))
  assert(transition.src.endsWith('/media/audio-1.mp3'), `${viewport.width}: manual next did not enter the destination audio ${JSON.stringify(transition)}`)
  if (viewport.width === 390) {
    await page.waitForFunction(() => document.body.innerText.includes('自动播放被浏览器阻止'))
    assert(!await page.locator('.reader-audio-content audio').evaluate(audio => audio.autoplay), '390: blocked autoplay intent must settle visibly')
    assert(await page.evaluate(() => window.__openreaderAudio.playCalls) === playCallsBeforeTransition, '390: a rejected autoplay attempt must not report a successful play')
    await page.getByRole('button', { name: '播放' }).click()
  } else if (transition.playCalls === playCallsBeforeTransition) {
    assert(transition.autoplay, `${viewport.width}: pending autoplay intent disappeared before play ${JSON.stringify(transition)}`)
    await page.evaluate(() => {
      const audio = document.querySelector('.reader-audio-content audio')
      Object.defineProperty(audio, 'duration', { configurable: true, value: 90 })
      audio.dispatchEvent(new Event('loadedmetadata'))
    })
  }
  await page.waitForFunction(calls => window.__openreaderAudio.playCalls > calls, playCallsBeforeTransition)
  assert(!await page.locator('.reader-audio-content audio').evaluate(audio => audio.autoplay), `${viewport.width}: autoplay intent must clear only after the destination really plays`)

  playCallsBeforeTransition = await page.evaluate(() => window.__openreaderAudio.playCalls)
  await page.locator('.reader-audio-actions.primary').getByRole('button', { name: '上一章' }).click()
  await page.waitForFunction(() => document.querySelector('.reader-audio-book-title')?.textContent?.includes('第一集'))
  transition = await page.evaluate(() => ({
    autoplay: document.querySelector('.reader-audio-content audio')?.autoplay,
    src: document.querySelector('.reader-audio-content audio')?.src || '',
    playCalls: window.__openreaderAudio.playCalls,
  }))
  assert(transition.src.endsWith('/media/audio-0.mp3') && (transition.autoplay || transition.playCalls > playCallsBeforeTransition), `${viewport.width}: manual previous must retain autoplay until a real play ${JSON.stringify(transition)}`)
  if (transition.playCalls === playCallsBeforeTransition) {
    await page.evaluate(() => document.querySelector('.reader-audio-content audio').dispatchEvent(new Event('loadedmetadata')))
  }
  await page.waitForFunction(calls => window.__openreaderAudio.playCalls > calls, playCallsBeforeTransition)

  playCallsBeforeTransition = await page.evaluate(() => window.__openreaderAudio.playCalls)
  await page.evaluate(() => document.querySelector('.reader-audio-content audio').dispatchEvent(new Event('ended')))
  await page.waitForFunction(() => document.querySelector('.reader-audio-book-title')?.textContent?.includes('第二集'))
  transition = await page.evaluate(() => ({
    autoplay: document.querySelector('.reader-audio-content audio')?.autoplay,
    src: document.querySelector('.reader-audio-content audio')?.src || '',
    playCalls: window.__openreaderAudio.playCalls,
  }))
  assert(transition.src.endsWith('/media/audio-1.mp3') && (transition.autoplay || transition.playCalls > playCallsBeforeTransition), `${viewport.width}: ended transition must retain autoplay until a real play ${JSON.stringify(transition)}`)
  if (transition.playCalls === playCallsBeforeTransition) {
    await page.evaluate(() => document.querySelector('.reader-audio-content audio').dispatchEvent(new Event('loadedmetadata')))
  }
  await page.waitForFunction(calls => window.__openreaderAudio.playCalls > calls, playCallsBeforeTransition)

  const finalChapterRequestsBeforeBoundary = requests.filter(item => item === 'GET /books/1/chapters/1/content').length
  await page.locator('.reader-audio-actions.primary').getByRole('button', { name: '下一章' }).click()
  await page.waitForFunction(() => document.body.innerText.includes('本章是最后一章'))
  await page.evaluate(() => document.querySelector('.reader-audio-content audio').dispatchEvent(new Event('ended')))
  await page.waitForTimeout(120)
  assert(
    requests.filter(item => item === 'GET /books/1/chapters/1/content').length === finalChapterRequestsBeforeBoundary,
    `${viewport.width}: final-chapter boundary and ended event must not issue another content request`,
  )

  const chapterOneRequestsBeforeKey = requests.filter(item => item === 'GET /books/1/chapters/1/content').length
  await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(160)
  assert(
    requests.filter(item => item === 'GET /books/1/chapters/1/content').length === chapterOneRequestsBeforeKey,
    `${viewport.width}: ArrowRight must not page or jump audio chapters`,
  )

  if (viewport.width <= 750) {
    assert(initial.mobileTopVisible, `${viewport.width}: mobile toolbar should be visible by default`)
    await page.mouse.click(viewport.width - 20, Math.round(viewport.height / 2))
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: side tap should not hide toolbar for audio`)
    const titleTapPoint = await page.locator('.reader-audio-book-title').evaluate((element) => {
      const rect = element.getBoundingClientRect()
      return {
        x: Math.round(rect.left + rect.width / 2),
        y: Math.round(rect.top + rect.height / 2),
      }
    })
    await page.mouse.click(titleTapPoint.x, titleTapPoint.y)
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, `${viewport.width}: center tap should hide toolbar for audio`)
    await page.mouse.click(titleTapPoint.x, titleTapPoint.y)
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: second center tap should show toolbar for audio`)
  }

  assert(failures.length === 0, `${viewport.width}: browser errors:\n${failures.join('\n')}`)
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    await runViewport(browser, { width: 390, height: 844 })
    await runViewport(browser, { width: 360, height: 800 })
    await runViewport(browser, { width: 1440, height: 900 })
    console.log('reader audio contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error)
  process.exit(1)
})
