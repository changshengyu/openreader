#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import http from 'node:http'
import https from 'node:https'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const databasePath = String(process.env.OPENREADER_SMOKE_DB || '').trim()
const directSQLite = databasePath !== ''
const sourceLimit = 8 << 20
const articleLimit = 16 << 10
const importItemLimit = 5000

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(path, { method = 'GET', body = '', token = '', chunked = false } = {}) {
  const url = new URL(path, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const headers = {}
  if (body) {
    headers['Content-Type'] = 'application/json'
    if (chunked) headers['Transfer-Encoding'] = 'chunked'
    else headers['Content-Length'] = Buffer.byteLength(body)
  }
  if (token) headers.Authorization = `Bearer ${token}`

  return new Promise((resolve, reject) => {
    const outgoing = transport.request(url, { method, headers }, (incoming) => {
      const chunks = []
      incoming.on('data', chunk => chunks.push(chunk))
      incoming.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          reject(new Error(`${method} ${path}: non-JSON response ${JSON.stringify(text.slice(0, 512))}`))
          return
        }
        resolve({ status: incoming.statusCode || 0, data, text })
      })
    })
    outgoing.on('error', reject)
    if (body) {
      if (chunked) {
        const middle = Math.floor(body.length / 2)
        outgoing.write(body.slice(0, middle))
        outgoing.write(body.slice(middle))
      } else {
        outgoing.write(body)
      }
    }
    outgoing.end()
  })
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text.slice(0, 512)}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function paddedJSON(body, targetBytes) {
  let prefix
  let suffix
  if (body.startsWith('[') && body.endsWith('}]')) {
    prefix = `${body.slice(0, -2)},"padding":"`
    suffix = '"}]'
  } else {
    assert(body.startsWith('{') && body.endsWith('}'), `fixture is not one object/array item: ${body}`)
    prefix = `${body.slice(0, -1)},"padding":"`
    suffix = '"}'
  }
  const padding = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(padding >= 0, `fixture exceeds ${targetBytes} bytes`)
  const result = `${prefix}${'x'.repeat(padding)}${suffix}`
  assert(Buffer.byteLength(result) === targetBytes, `fixture is ${Buffer.byteLength(result)} bytes, want ${targetBytes}`)
  return result
}

function importPayload(count, prefix) {
  const rows = Array.from({ length: count }, (_, index) => ({
    title: `${prefix}-${index}`,
    url: `https://rss.example/${prefix}/${index}.xml`,
  }))
  return JSON.stringify(rows)
}

function sqlQuote(value) {
  return `'${String(value).replaceAll("'", "''")}'`
}

function sqlite(statement) {
  assert(databasePath, 'OPENREADER_SMOKE_DB is required')
  return execFileSync(process.env.SQLITE3_BIN || 'sqlite3', [databasePath, statement], { encoding: 'utf8' }).trim()
}

function sqliteNumber(statement) {
  const value = Number(sqlite(statement))
  assert(Number.isFinite(value), `non-numeric SQLite result for ${statement}`)
  return value
}

async function register(username) {
  const response = await request('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register: ${response.status} ${response.text}`)
  return response.data
}

async function createSource(token, payload) {
  const response = await request('/api/rss/sources', {
    method: 'POST',
    token,
    body: JSON.stringify(payload),
  })
  assert(response.status === 201 && response.data?.id, `create RSS source: ${response.status} ${response.text.slice(0, 512)}`)
  return response.data
}

async function listSources(token) {
  const response = await request('/api/rss/sources', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list RSS sources: ${response.status} ${response.text}`)
  return response.data
}

async function listArticles(token, sourceID) {
  const response = await request(`/api/rss/articles?sourceId=${sourceID}`, { token })
  assert(response.status === 200 && Array.isArray(response.data), `list RSS articles: ${response.status} ${response.text}`)
  return response.data
}

function seedArticle(userID, sourceID, { title, link, content = '', isRead = false, favorite = false }) {
  return sqliteNumber(`
    INSERT INTO rss_articles (user_id, source_id, sort, title, link, guid, author, image, summary, content, pub_date, is_read, favorite)
    VALUES (${Number(userID)}, ${Number(sourceID)}, '', ${sqlQuote(title)}, ${sqlQuote(link)}, '', '', '', '', ${sqlQuote(content)}, '', ${isRead ? 1 : 0}, ${favorite ? 1 : 0});
    SELECT last_insert_rowid();
  `)
}

function startFeedServer() {
  const state = {
    token: '',
    deleteSourceID: 0,
    deleteArticleID: 0,
    deleteArticleSourceID: 0,
    priorityFresh: false,
  }
  const server = http.createServer(async (incoming, response) => {
    try {
      response.setHeader('Content-Type', incoming.url?.startsWith('/detail') ? 'text/html; charset=utf-8' : 'application/xml; charset=utf-8')
      if (incoming.url === '/feed-state') {
        response.end(`<rss version="2.0"><channel><item><title>state before</title><link>${state.baseURL}/state-article</link></item></channel></rss>`)
        return
      }
      if (incoming.url === '/feed-detail') {
        response.end(`<rss version="2.0"><channel><item><title>detail before</title><link>${state.baseURL}/detail-preserve</link></item></channel></rss>`)
        return
      }
      if (incoming.url === '/feed-detail-delete') {
        response.end(`<rss version="2.0"><channel><item><title>late detail</title><link>${state.baseURL}/detail-delete</link></item></channel></rss>`)
        return
      }
      if (incoming.url === '/feed-priority') {
        const title = state.priorityFresh ? 'fresh feed title' : 'priority before'
        const summary = state.priorityFresh ? 'fresh summary' : 'initial summary'
        const content = state.priorityFresh ? '<p>feed candidate</p>' : ''
        response.end(`<?xml version="1.0"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><item><title>${title}</title><link>${state.baseURL}/detail-preserve</link><description>${summary}</description><content:encoded><![CDATA[${content}]]></content:encoded></item></channel></rss>`)
        return
      }
      if (incoming.url === '/feed-delete') {
        if (directSQLite) {
          sqlite(`DELETE FROM rss_sources WHERE id = ${Number(state.deleteSourceID)};`)
        } else {
          const deleted = await request(`/api/rss/sources/${state.deleteSourceID}`, { method: 'DELETE', token: state.token })
          assert(deleted.status === 204, `delete source during refresh: ${deleted.status} ${deleted.text}`)
        }
        response.end(`<rss version="2.0"><channel><item><title>late article</title><link>${state.baseURL}/late</link></item></channel></rss>`)
        return
      }
      if (incoming.url === '/detail-delete') {
        if (directSQLite) {
          sqlite(`DELETE FROM rss_articles WHERE id = ${Number(state.deleteArticleID)}; DELETE FROM rss_sources WHERE id = ${Number(state.deleteArticleSourceID)};`)
        } else {
          const deleted = await request(`/api/rss/sources/${state.deleteArticleSourceID}`, { method: 'DELETE', token: state.token })
          assert(deleted.status === 204, `delete source during content fetch: ${deleted.status} ${deleted.text}`)
        }
        response.end('<div class="content">late detail</div>')
        return
      }
      if (incoming.url === '/detail-preserve') {
        response.end('<div class="content"><p>authoritative detail</p></div>')
        return
      }
      response.statusCode = 404
      response.end('not found')
    } catch (error) {
      response.statusCode = 500
      response.end(String(error?.message || error))
    }
  })

  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, process.env.RSS_SMOKE_FEED_BIND || '127.0.0.1', () => {
      const address = server.address()
      const hostname = process.env.RSS_SMOKE_FEED_HOSTNAME || '127.0.0.1'
      state.baseURL = `http://${hostname}:${address.port}`
      resolve({ server, state })
    })
  })
}

async function main() {
  const feed = await startFeedServer()
  try {
    const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
    const health = await request('/api/health')
    assert(health.status === 200, `health status ${health.status}: ${health.text}`)
    const owner = await register(`rss${suffix}`)
    const token = owner.token
    const userID = owner.user.id
    feed.state.token = token

    const createBody = JSON.stringify({ title: 'wire create', url: `https://rss.example/wire-create-${suffix}.xml` })
    const importBody = JSON.stringify([{ title: 'wire import', url: `https://rss.example/wire-import-${suffix}.xml` }])
    const updateTarget = await createSource(token, { title: 'wire update before', url: `https://rss.example/wire-update-${suffix}.xml` })

    for (const route of [
      { name: 'create', method: 'POST', path: '/api/rss/sources', body: createBody, message: 'request body too large', overflowStatus: 413, success: 201 },
      { name: 'import', method: 'POST', path: '/api/rss/sources/import', body: importBody, message: 'invalid RSS source import', overflowStatus: 400, success: 200 },
      { name: 'update', method: 'PUT', path: `/api/rss/sources/${updateTarget.id}`, body: JSON.stringify({ title: 'wire update after', url: updateTarget.url }), message: 'request body too large', overflowStatus: 413, success: 200 },
    ]) {
      for (const chunked of [false, true]) {
        const overflow = await request(route.path, {
          method: route.method,
          token,
          body: paddedJSON(route.body, sourceLimit + 1),
          chunked,
        })
        expectError(overflow, route.overflowStatus, route.message)
      }
      const malformed = route.name === 'import' ? 'invalid RSS source import' : 'url is required'
      const second = await request(route.path, { method: route.method, token, body: `${route.body}${route.name === 'import' ? '[]' : '{}'}` })
      expectError(second, 400, malformed)
      const exact = await request(route.path, { method: route.method, token, body: paddedJSON(route.body, sourceLimit) })
      assert(exact.status === route.success, `${route.name} exact limit: ${exact.status} ${exact.text.slice(0, 512)}`)
    }

    const exactImport = await request('/api/rss/sources/import', {
      method: 'POST', token, body: importPayload(importItemLimit, `exact-${suffix}`),
    })
    assert(exactImport.status === 200 && exactImport.data?.created === importItemLimit, `exact import count: ${exactImport.status} ${exactImport.text.slice(0, 512)}`)
    const sourceCountBeforeOverflow = (await listSources(token)).length
    const overflowImport = await request('/api/rss/sources/import', {
      method: 'POST', token, body: importPayload(importItemLimit + 1, `overflow-${suffix}`),
    })
    expectError(overflowImport, 400, 'invalid RSS source import')
    assert((await listSources(token)).length === sourceCountBeforeOverflow, '5,001-item import persisted a prefix')

    const stateSource = await createSource(token, {
      title: 'state source',
      url: directSQLite ? `https://rss.example/state-${suffix}.xml` : `${feed.state.baseURL}/feed-state`,
    })
    let stateArticleID
    if (directSQLite) {
      stateArticleID = seedArticle(userID, stateSource.id, {
        title: 'state before', link: `https://rss.example/state/${suffix}`, favorite: true,
      })
    } else {
      const stateRefresh = await request(`/api/rss/sources/${stateSource.id}/refresh`, { method: 'POST', token })
      assert(stateRefresh.status === 200 && stateRefresh.data?.items?.length === 1, `seed state article: ${stateRefresh.status} ${stateRefresh.text}`)
      stateArticleID = stateRefresh.data.items[0].id
      const favoriteState = await request(`/api/rss/articles/${stateArticleID}`, {
        method: 'PUT', token, body: '{"favorite":true}',
      })
      assert(favoriteState.status === 200 && favoriteState.data?.favorite === true, `seed favorite state: ${favoriteState.status} ${favoriteState.text}`)
    }
    const statePath = `/api/rss/articles/${stateArticleID}`
    for (const chunked of [false, true]) {
      const overflow = await request(statePath, {
        method: 'PUT', token, body: paddedJSON('{"isRead":true}', articleLimit + 1), chunked,
      })
      expectError(overflow, 413, 'request body too large')
    }
    for (const body of ['{}', '{"ignored":true}', '{"isRead":null}', '{"favorite":1}', '{"isRead":true}{}']) {
      const invalid = await request(statePath, { method: 'PUT', token, body })
      expectError(invalid, 400, 'invalid RSS article payload')
    }
    const exactState = await request(statePath, {
      method: 'PUT', token, body: paddedJSON('{"isRead":true}', articleLimit),
    })
    assert(exactState.status === 200 && exactState.data?.isRead === true && exactState.data?.favorite === true, `exact state patch: ${exactState.status} ${exactState.text}`)

    const concurrentURL = `https://rss.example/concurrent-${suffix}.xml`
    const concurrent = await Promise.all(Array.from({ length: 8 }, (_, index) => {
      const useImport = index % 2 === 1
      return request(useImport ? '/api/rss/sources/import' : '/api/rss/sources', {
        method: 'POST',
        token,
        body: JSON.stringify(useImport
          ? [{ title: `concurrent ${index}`, url: concurrentURL }]
          : { title: `concurrent ${index}`, url: concurrentURL }),
      })
    }))
    assert(concurrent.every(result => result.status === 200 || result.status === 201), `concurrent create/import statuses: ${concurrent.map(result => result.status)}`)
    assert((await listSources(token)).filter(source => source.url === concurrentURL).length === 1, 'concurrent create/import left duplicate URLs')

    if (directSQLite) {
      const deletedSource = await createSource(token, { title: 'delete source', url: `https://rss.example/delete-${suffix}.xml` })
      sqlite(`
        CREATE TRIGGER smoke_rss_source_update_delete BEFORE UPDATE OF title ON rss_sources
        WHEN OLD.id = ${Number(deletedSource.id)} BEGIN DELETE FROM rss_sources WHERE id = OLD.id; END;
      `)
      const deletedSourceUpdate = await request(`/api/rss/sources/${deletedSource.id}`, {
        method: 'PUT', token, body: JSON.stringify({ title: 'must not revive', url: deletedSource.url }),
      })
      sqlite('DROP TRIGGER smoke_rss_source_update_delete;')
      expectError(deletedSourceUpdate, 404, 'RSS source not found')
      assert(sqliteNumber(`SELECT COUNT(*) FROM rss_sources WHERE id = ${Number(deletedSource.id)};`) === 0, 'source update revived a deleted row')

      const metadataArticleID = seedArticle(userID, stateSource.id, {
        title: 'metadata before', link: `https://rss.example/metadata/${suffix}`,
      })
      sqlite(`
        CREATE TRIGGER smoke_rss_state_metadata BEFORE UPDATE OF is_read ON rss_articles
        WHEN OLD.id = ${metadataArticleID} BEGIN
          UPDATE rss_articles SET title = 'metadata after', summary = 'summary after' WHERE id = OLD.id;
        END;
      `)
      const metadataUpdate = await request(`/api/rss/articles/${metadataArticleID}`, {
        method: 'PUT', token, body: '{"isRead":true}',
      })
      sqlite('DROP TRIGGER smoke_rss_state_metadata;')
      assert(metadataUpdate.status === 200 && metadataUpdate.data?.title === 'metadata after' && metadataUpdate.data?.summary === 'summary after', `state patch returned stale metadata: ${metadataUpdate.status} ${metadataUpdate.text}`)

      const deletedArticleID = seedArticle(userID, stateSource.id, {
        title: 'article delete before', link: `https://rss.example/article-delete/${suffix}`,
      })
      sqlite(`
        CREATE TRIGGER smoke_rss_state_delete BEFORE UPDATE OF favorite ON rss_articles
        WHEN OLD.id = ${deletedArticleID} BEGIN DELETE FROM rss_articles WHERE id = OLD.id; END;
      `)
      const deletedArticleUpdate = await request(`/api/rss/articles/${deletedArticleID}`, {
        method: 'PUT', token, body: '{"favorite":true}',
      })
      sqlite('DROP TRIGGER smoke_rss_state_delete;')
      expectError(deletedArticleUpdate, 404, 'RSS article not found')
      assert(sqliteNumber(`SELECT COUNT(*) FROM rss_articles WHERE id = ${deletedArticleID};`) === 0, 'state patch revived a deleted article')
    }

    const detailSource = await createSource(token, {
      title: 'detail source',
      url: `${feed.state.baseURL}/${directSQLite ? 'unused' : 'feed-detail'}`,
      ruleContent: '.content|html',
    })
    let detailArticleID
    if (directSQLite) {
      detailArticleID = seedArticle(userID, detailSource.id, {
        title: 'detail before', link: `${feed.state.baseURL}/detail-preserve`,
      })
      sqlite(`
        CREATE TRIGGER smoke_rss_content_columns BEFORE UPDATE OF content ON rss_articles
        WHEN OLD.id = ${detailArticleID} BEGIN
          UPDATE rss_articles SET title = 'detail concurrent', favorite = 1 WHERE id = OLD.id;
        END;
      `)
    } else {
      const detailRefresh = await request(`/api/rss/sources/${detailSource.id}/refresh`, { method: 'POST', token })
      assert(detailRefresh.status === 200 && detailRefresh.data?.items?.length === 1, `seed detail article: ${detailRefresh.status} ${detailRefresh.text}`)
      detailArticleID = detailRefresh.data.items[0].id
      const favoriteDetail = await request(`/api/rss/articles/${detailArticleID}`, {
        method: 'PUT', token, body: '{"favorite":true}',
      })
      assert(favoriteDetail.status === 200 && favoriteDetail.data?.favorite === true, `seed detail favorite: ${favoriteDetail.status} ${favoriteDetail.text}`)
    }
    const detailResponse = await request(`/api/rss/articles/${detailArticleID}/content`, { token })
    if (directSQLite) sqlite('DROP TRIGGER smoke_rss_content_columns;')
    const expectedDetailTitle = directSQLite ? 'detail concurrent' : 'detail before'
    assert(detailResponse.status === 200 && detailResponse.data?.title === expectedDetailTitle && detailResponse.data?.favorite === true && String(detailResponse.data?.content).includes('authoritative detail'), `content cache overwrote owned columns: ${detailResponse.status} ${detailResponse.text}`)

    const prioritySource = await createSource(token, {
      title: 'priority source', url: `${feed.state.baseURL}/feed-priority`, ruleContent: '.content|html',
    })
    let priorityArticleID
    let expectedPriorityContent
    if (directSQLite) {
      priorityArticleID = seedArticle(userID, prioritySource.id, {
        title: 'priority before', link: `${feed.state.baseURL}/detail-preserve`, content: '<p>stored detail</p>', favorite: true,
      })
      expectedPriorityContent = '<p>stored detail</p>'
    } else {
      const initialPriority = await request(`/api/rss/sources/${prioritySource.id}/refresh`, { method: 'POST', token })
      assert(initialPriority.status === 200 && initialPriority.data?.items?.length === 1, `seed priority article: ${initialPriority.status} ${initialPriority.text}`)
      priorityArticleID = initialPriority.data.items[0].id
      const cachedPriority = await request(`/api/rss/articles/${priorityArticleID}/content`, { token })
      assert(cachedPriority.status === 200 && String(cachedPriority.data?.content).includes('authoritative detail'), `cache priority detail: ${cachedPriority.status} ${cachedPriority.text}`)
      expectedPriorityContent = cachedPriority.data.content
      const favoritePriority = await request(`/api/rss/articles/${priorityArticleID}`, {
        method: 'PUT', token, body: '{"favorite":true}',
      })
      assert(favoritePriority.status === 200 && favoritePriority.data?.favorite === true, `seed priority favorite: ${favoritePriority.status} ${favoritePriority.text}`)
    }
    feed.state.priorityFresh = true
    const priorityRefresh = await request(`/api/rss/sources/${prioritySource.id}/refresh`, { method: 'POST', token })
    assert(priorityRefresh.status === 200, `priority refresh: ${priorityRefresh.status} ${priorityRefresh.text}`)
    const priorityArticle = (await listArticles(token, prioritySource.id)).find(article => article.id === priorityArticleID)
    assert(priorityArticle?.title === 'fresh feed title' && priorityArticle?.content === expectedPriorityContent && priorityArticle?.favorite === true, `refresh violated content/state priority: ${JSON.stringify(priorityArticle)}`)

    const lateSource = await createSource(token, {
      title: 'late source', url: `${feed.state.baseURL}/feed-delete`,
    })
    feed.state.deleteSourceID = lateSource.id
    const lateRefresh = await request(`/api/rss/sources/${lateSource.id}/refresh`, { method: 'POST', token })
    expectError(lateRefresh, 404, 'RSS source not found')
    if (directSQLite) {
      assert(sqliteNumber(`SELECT COUNT(*) FROM rss_articles WHERE source_id = ${Number(lateSource.id)};`) === 0, 'refresh persisted orphan articles')
    } else {
      assert((await listArticles(token, lateSource.id)).length === 0, 'refresh persisted orphan articles')
    }

    const lateDetailSource = await createSource(token, {
      title: 'late detail source',
      url: `${feed.state.baseURL}/${directSQLite ? 'unused-delete' : 'feed-detail-delete'}`,
      ruleContent: '.content|html',
    })
    let lateDetailArticleID
    if (directSQLite) {
      lateDetailArticleID = seedArticle(userID, lateDetailSource.id, {
        title: 'late detail', link: `${feed.state.baseURL}/detail-delete`,
      })
    } else {
      const lateDetailRefresh = await request(`/api/rss/sources/${lateDetailSource.id}/refresh`, { method: 'POST', token })
      assert(lateDetailRefresh.status === 200 && lateDetailRefresh.data?.items?.length === 1, `seed late detail article: ${lateDetailRefresh.status} ${lateDetailRefresh.text}`)
      lateDetailArticleID = lateDetailRefresh.data.items[0].id
    }
    feed.state.deleteArticleID = lateDetailArticleID
    feed.state.deleteArticleSourceID = lateDetailSource.id
    const lateDetail = await request(`/api/rss/articles/${lateDetailArticleID}/content`, { token })
    expectError(lateDetail, 404, 'RSS source not found')
    if (directSQLite) {
      assert(sqliteNumber(`SELECT COUNT(*) FROM rss_articles WHERE id = ${lateDetailArticleID};`) === 0, 'content fetch revived a deleted article')
    } else {
      const missingArticle = await request(`/api/rss/articles/${lateDetailArticleID}`, {
        method: 'PUT', token, body: '{"isRead":true}',
      })
      expectError(missingArticle, 404, 'RSS article not found')
    }

    console.log(JSON.stringify({
      status: 'ok',
      wire: { sourceBytes: sourceLimit, articleBytes: articleLimit, singleJSON: true },
      importCardinality: { exact: importItemLimit, overflow: importItemLimit + 1 },
      sourceIdentity: 'serialized-per-user',
      explicitColumns: 'preserved',
      deletedRows: 'not-revived',
      remoteLiveness: 'rechecked',
      mode: directSQLite ? 'http-plus-sqlite-triggers' : 'public-api-container',
    }))
  } finally {
    await new Promise(resolve => feed.server.close(resolve))
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
