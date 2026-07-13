import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'

const root = resolve(import.meta.dirname, '..')
const read = relative => readFileSync(resolve(root, relative), 'utf8')

test('remote result cards preserve upstream cover-info and body-read entry split', () => {
  const component = read('src/components/RemoteBookResultGroups.vue')
  const search = read('src/views/Search.vue')
  const discover = read('src/views/Discover.vue')

  assert.match(component, /class="result-card app-panel"[\s\S]*@click="\$emit\('read', item\)"/)
  assert.match(component, /<BookCover[^>]*@click\.stop="\$emit\('preview', item\)"/)
  assert.match(component, /defineEmits\(\['preview', 'read'\]\)/)
  assert.doesNotMatch(component, /result-actions|查看信息/)
  assert.match(search, /@read="openRemoteReader"/)
  assert.match(discover, /@read="openRemoteReader"/)
  assert.match(search, /function openRemoteReader\(item\)/)
  assert.match(discover, /function openRemoteReader\(book\)/)
})

test('legacy BookInfo query is consumed when its shared overlay closes', () => {
  const layout = read('src/layouts/AppLayout.vue')
  assert.match(layout, /function clearRouteBookInfoOverlayIntent\(\)/)
  assert.match(layout, /bookInfo: _bookInfo/)
  assert.match(layout, /router\.replace\(\{ name: 'home', query \}\)/)
  assert.match(layout, /watch\(\s*\(\)\s*=>\s*overlay\.bookInfoVisible,[\s\S]*clearRouteBookInfoOverlayIntent\(\)/)
})
