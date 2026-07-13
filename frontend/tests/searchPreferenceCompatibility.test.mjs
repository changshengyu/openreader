import assert from 'node:assert/strict'
import test from 'node:test'
import {
  DEFAULT_SEARCH,
  SEARCH_CONCURRENT_OPTIONS,
  searchConcurrentLabel,
  searchConcurrentOptions,
  sanitizeSearchPreference,
} from '../src/utils/searchPreference.js'

test('restores the upstream multi-source concurrency default for new search preferences', () => {
  assert.deepEqual(DEFAULT_SEARCH, {
    searchType: 'all',
    group: '',
    sourceId: '',
    concurrent: 24,
  })
  assert.equal(sanitizeSearchPreference({}).concurrent, 24)
  assert.equal(sanitizeSearchPreference({ concurrent: 0 }).concurrent, 24)
  assert.deepEqual(SEARCH_CONCURRENT_OPTIONS, [12, 18, 24, 30, 36, 42, 48, 54, 60])
})

test('preserves canonical and historical OpenReader concurrency values without a silent preference reset', () => {
  for (const value of SEARCH_CONCURRENT_OPTIONS) {
    assert.equal(sanitizeSearchPreference({ concurrent: value }).concurrent, value)
  }
  for (const value of [8, 16, 32]) {
    assert.equal(sanitizeSearchPreference({ concurrent: value }).concurrent, value)
    assert.ok(searchConcurrentOptions(value).includes(value), `legacy ${value} must remain selectable`)
    assert.match(searchConcurrentLabel(value), /旧配置/)
  }
  assert.equal(sanitizeSearchPreference({ concurrent: 999 }).concurrent, 24)
})

test('keeps the deployed all/group/single source-id mapping while normalizing concurrency only', () => {
  assert.deepEqual(
    sanitizeSearchPreference({ searchType: 'group', group: '玄幻', sourceId: 9, concurrent: 32 }),
    { searchType: 'group', group: '玄幻', sourceId: 9, concurrent: 32 },
  )
  assert.deepEqual(
    sanitizeSearchPreference({ searchType: 'multi', group: '不应保留', concurrent: 24 }),
    { searchType: 'all', group: '不应保留', sourceId: '', concurrent: 24 },
  )
})
