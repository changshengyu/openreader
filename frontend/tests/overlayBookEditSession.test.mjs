import assert from 'node:assert/strict'
import test from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useOverlayStore } from '../src/stores/overlay.js'

test('opens the one shared book editor only for persisted shelf-shaped records', () => {
  setActivePinia(createPinia())
  const overlay = useOverlayStore()

  assert.equal(overlay.openBookEdit({ title: '临时搜索结果' }), false)
  assert.equal(overlay.bookEditVisible, false)
  assert.equal(overlay.bookEditBook, null)

  const shelfBook = { id: 7, title: '书架书' }
  assert.equal(overlay.openBookEdit(shelfBook), true)
  assert.equal(overlay.bookEditVisible, true)
  assert.deepEqual(overlay.bookEditBook, shelfBook)

  overlay.closeBookEdit()
  assert.equal(overlay.bookEditVisible, false)
})
