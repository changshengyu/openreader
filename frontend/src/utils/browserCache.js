const DB_NAME = 'openreader-cache'
const DB_VERSION = 1
const STORE_NAME = 'responses'
const CACHE_PREFIX = 'localCache@'

let dbPromise

function openDB() {
  if (typeof window === 'undefined' || !window.indexedDB) {
    return Promise.reject(new Error('IndexedDB unavailable'))
  }
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      const request = window.indexedDB.open(DB_NAME, DB_VERSION)
      request.onerror = () => reject(request.error || new Error('failed to open cache db'))
      request.onsuccess = () => resolve(request.result)
      request.onupgradeneeded = () => {
        const db = request.result
        if (!db.objectStoreNames.contains(STORE_NAME)) {
          db.createObjectStore(STORE_NAME, { keyPath: 'key' })
        }
      }
    })
  }
  return dbPromise
}

async function idbGet(key) {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const request = store.get(key)
    request.onerror = () => reject(request.error || new Error('failed to read cache'))
    request.onsuccess = () => resolve(request.result)
  })
}

async function idbSet(key, value, canWrite, chapterRequest) {
  const db = await openDB()
  if (canWrite?.() === false) return
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    if (!chapterRequest) {
      const request = store.put({ key, value, updatedAt: Date.now() })
      request.onerror = () => reject(request.error || new Error('failed to write cache'))
      request.onsuccess = () => resolve()
      return
    }
    tx.oncomplete = () => resolve()
    tx.onabort = () => reject(tx.error || new Error('failed to write cache'))
    // Read and conditionally replace the chapter within one transaction across tabs.
    const existing = store.get(key)
    existing.onsuccess = () => {
      const lastReload = updateChapterReload(existing.result?.chapterReload, chapterRequest)
      if (lastReload === false || canWrite?.() === false) return
      const record = { key, value, updatedAt: Date.now() }
      if (lastReload) record.chapterReload = lastReload
      store.put(record)
    }
  })
}

async function idbRemove(key) {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const request = store.delete(key)
    request.onerror = () => reject(request.error || new Error('failed to remove cache'))
    request.onsuccess = () => resolve()
  })
}

function updateChapterReload(lastReload, request) {
  if (request.refreshRemote) {
    if (lastReload >= request.startedAt) return false
    return request.startedAt
  }
  if (lastReload !== request.chapterReload) return false
  return lastReload
}

function prefixedKey(key) {
  return key.startsWith(CACHE_PREFIX) ? key : `${CACHE_PREFIX}${key}`
}

function readLegacyCache(key) {
  try {
    const raw = window.localStorage?.getItem(key)
    if (!raw) return null
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function writeLegacyCache(key, value) {
  try {
    window.localStorage?.setItem(key, JSON.stringify(value))
  } catch {
    // localStorage may be full on mobile browsers; IndexedDB is the primary path.
  }
}

export async function getBrowserCache(key) {
  return (await readBrowserCacheRecord(key)).value
}

export async function readBrowserCacheRecord(key) {
  const cacheKey = prefixedKey(key)
  try {
    const record = await idbGet(cacheKey)
    if (record?.value) return record
  } catch {
    // Fall through to the old localStorage-compatible path.
  }
  const cached = readLegacyCache(cacheKey)
  if (cached?.openReaderChapterReload) {
    const { openReaderChapterReload, ...value } = cached
    return { value, chapterReload: openReaderChapterReload }
  }
  return { value: cached }
}

export async function setBrowserCache(key, value, canWrite, chapterRequest) {
  if (canWrite?.() === false) return
  const cacheKey = prefixedKey(key)
  try {
    await idbSet(cacheKey, value, canWrite, chapterRequest)
  } catch {
    if (!chapterRequest) {
      if (canWrite?.() !== false) writeLegacyCache(cacheKey, value)
      return
    }
    const writeChapter = () => {
      if (canWrite?.() === false) return
      const existing = readLegacyCache(cacheKey)
      const lastReload = updateChapterReload(existing?.openReaderChapterReload, chapterRequest)
      if (lastReload === false) return
      writeLegacyCache(cacheKey, lastReload ? { ...value, openReaderChapterReload: lastReload } : value)
    }
    const locks = globalThis.navigator?.locks
    if (!locks) {
      writeChapter()
      return
    }
    try {
      await locks.request(cacheKey, writeChapter)
    } catch {
      writeChapter()
    }
  }
}

export async function removeBrowserCache(key) {
  const cacheKey = prefixedKey(key)
  try {
    await idbRemove(cacheKey)
  } catch {
    // Ignore missing IndexedDB entries; the legacy path is handled below.
  }
  try {
    window.localStorage?.removeItem(cacheKey)
  } catch {
    // Ignore private-mode storage errors.
  }
}

async function idbKeys(prefix) {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const keys = []
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const request = store.openCursor()
    request.onerror = () => reject(request.error || new Error('failed to iterate cache'))
    request.onsuccess = () => {
      const cursor = request.result
      if (!cursor) {
        resolve(keys)
        return
      }
      if (String(cursor.key).startsWith(prefix)) {
        keys.push(String(cursor.key))
      }
      cursor.continue()
    }
  })
}

function legacyKeys(prefix) {
  const keys = []
  try {
    const storage = window.localStorage
    if (!storage) return keys
    for (let index = 0; index < storage.length; index += 1) {
      const key = storage.key(index)
      if (key?.startsWith(prefix)) keys.push(key)
    }
  } catch {
    // Ignore private-mode storage errors.
  }
  return keys
}

export async function listBrowserCacheKeys(prefix = '') {
  const cachePrefix = prefixedKey(prefix)
  const keys = new Set(legacyKeys(cachePrefix))
  try {
    const indexedKeys = await idbKeys(cachePrefix)
    indexedKeys.forEach(key => keys.add(key))
  } catch {
    // The localStorage fallback above is enough for unsupported browsers.
  }
  return [...keys]
}

export async function removeBrowserCacheKeys(prefix = '') {
  const keys = await listBrowserCacheKeys(prefix)
  await Promise.all(keys.map(key => removeBrowserCache(key)))
  return keys.length
}

export async function cacheFirstRequest(requestFunc, cacheKey, options = {}) {
  if (!options.refresh) {
    const cached = await getBrowserCache(cacheKey)
    if (cached && (!options.validate || options.validate(cached))) {
      return { data: cached, fromCache: true }
    }
  }
  const response = await requestFunc()
  if (response?.data && (!options.validate || options.validate(response.data))) {
    await setBrowserCache(cacheKey, response.data, options.canWrite)
  }
  return response
}

export async function networkFirstRequest(requestFunc, cacheKey, options = {}) {
  try {
    const response = await requestFunc()
    if (response?.data && (!options.validate || options.validate(response.data))) {
      await setBrowserCache(cacheKey, response.data, options.canWrite)
    }
    return response
  } catch (err) {
    const cached = await getBrowserCache(cacheKey)
    if (cached && (!options.validate || options.validate(cached))) {
      return { data: cached, fromCache: true }
    }
    throw err
  }
}
