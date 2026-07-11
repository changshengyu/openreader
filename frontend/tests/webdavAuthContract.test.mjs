import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const clientSource = readFileSync(new URL('../src/api/client.js', import.meta.url), 'utf8')
const webdavSource = readFileSync(new URL('../src/api/webdav.js', import.meta.url), 'utf8')

test('uses the shared bearer-token interceptor for every raw WebDAV request', () => {
  assert.match(clientSource, /export const rootApi\s*=\s*addAuthInterceptors\(/, 'a root-scoped authenticated client must exist outside /api')
  assert.match(clientSource, /config\.headers\.Authorization\s*=\s*`Bearer \$\{token\}`/, 'root requests must receive the normal bearer token')
  assert.match(webdavSource, /import api, \{ rootApi \} from '\.\/client'/)
  assert.doesNotMatch(webdavSource, /import axios from 'axios'/, 'WebDAV must not bypass the authenticated clients')
  for (const operation of ['rootApi.get', 'rootApi.put', 'rootApi.delete', 'return rootApi({']) {
    assert.match(webdavSource, new RegExp(operation.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')), `${operation} must use the root authenticated client`)
  }
  assert.doesNotMatch(webdavSource, /accessToken|openreader_token/, 'the WebDAV URL must never contain a credential')
})
