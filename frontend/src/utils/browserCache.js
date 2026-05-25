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
    request.onsuccess = () => resolve(request.result?.value)
  })
}

async function idbSet(key, value) {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const request = store.put({ key, value, updatedAt: Date.now() })
    request.onerror = () => reject(request.error || new Error('failed to write cache'))
    request.onsuccess = () => resolve()
  })
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
  const cacheKey = prefixedKey(key)
  try {
    const value = await idbGet(cacheKey)
    if (value) return value
  } catch {
    // Fall through to the old localStorage-compatible path.
  }
  return readLegacyCache(cacheKey)
}

export async function setBrowserCache(key, value) {
  const cacheKey = prefixedKey(key)
  try {
    await idbSet(cacheKey, value)
    return
  } catch {
    writeLegacyCache(cacheKey, value)
  }
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
    setBrowserCache(cacheKey, response.data)
  }
  return response
}
