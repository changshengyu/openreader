import assert from 'node:assert/strict'
import test from 'node:test'

import { planRSSSourceImport } from '../src/utils/rssSourceImport.js'

test('plans same-URL RSS imports as updates and new URLs as creates', () => {
  const result = planRSSSourceImport(
    [
      { title: '新版规则', url: ' https://rss.example/existing.xml ', ruleTitle: '.new-title' },
      { title: '新增源', url: 'https://rss.example/new.xml' },
    ],
    [
      { id: 7, title: '旧版规则', url: 'https://rss.example/existing.xml' },
    ],
  )

  assert.deepEqual(result.updates, [{
    id: 7,
    source: { title: '新版规则', url: 'https://rss.example/existing.xml', ruleTitle: '.new-title' },
  }])
  assert.deepEqual(result.creates, [{
    title: '新增源',
    url: 'https://rss.example/new.xml',
  }])
})

test('uses the last source when an import file repeats the same URL', () => {
  const result = planRSSSourceImport(
    [
      { title: '第一份', url: 'https://rss.example/feed.xml', enabled: true },
      { title: '最终配置', url: 'https://rss.example/feed.xml', enabled: false },
      { title: '无地址', url: '   ' },
    ],
    [{ id: 9, url: 'https://rss.example/feed.xml' }],
  )

  assert.equal(result.creates.length, 0)
  assert.deepEqual(result.updates, [{
    id: 9,
    source: { title: '最终配置', url: 'https://rss.example/feed.xml', enabled: false },
  }])
})
