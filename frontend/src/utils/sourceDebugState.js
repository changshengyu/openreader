const SOURCE_DEBUG_SCHEMA = 'v1'
const HISTORY_LIMIT = 50

function clone(value) {
  if (value === undefined) return undefined
  return JSON.parse(JSON.stringify(value))
}

export function sourceDebugStorageKey(scope, kind) {
  const safeScope = encodeURIComponent(String(scope || 'anonymous'))
  const safeKind = encodeURIComponent(String(kind || 'state'))
  return `openreader:source-debug:${SOURCE_DEBUG_SCHEMA}:${safeScope}:${safeKind}`
}

export function loadSourceDebugSources(storage, scope) {
  const value = readJSON(storage, sourceDebugStorageKey(scope, 'sources'))
  return Array.isArray(value) ? clone(value) : []
}

export function saveSourceDebugSources(storage, scope, sources) {
  const value = Array.isArray(sources) ? clone(sources) : []
  storage?.setItem?.(sourceDebugStorageKey(scope, 'sources'), JSON.stringify(value))
  return value
}

export function createSourceDebugHistory({ storage, scope, initial }) {
  const key = sourceDebugStorageKey(scope, 'history')
  const stored = readJSON(storage, key)
  let state = validHistory(stored)
    ? clone(stored)
    : { old: [], now: clone(initial), new: [] }

  function persist() {
    storage?.setItem?.(key, JSON.stringify(state))
  }

  function current() {
    return clone(state.now)
  }

  function commit(value) {
    state.old.push(clone(state.now))
    if (state.old.length > HISTORY_LIMIT) {
      state.old.splice(0, state.old.length - HISTORY_LIMIT)
    }
    state.now = clone(value)
    state.new = []
    persist()
    return current()
  }

  function undo() {
    if (!state.old.length) return current()
    state.new.push(clone(state.now))
    state.now = state.old.pop()
    persist()
    return current()
  }

  function redo() {
    if (!state.new.length) return current()
    state.old.push(clone(state.now))
    state.now = state.new.pop()
    persist()
    return current()
  }

  function reset(value) {
    state = { old: [], now: clone(value), new: [] }
    persist()
    return current()
  }

  persist()
  return {
    current,
    commit,
    undo,
    redo,
    reset,
    snapshot: () => clone(state),
  }
}

function readJSON(storage, key) {
  try {
    const value = storage?.getItem?.(key)
    return value ? JSON.parse(value) : null
  } catch {
    return null
  }
}

function validHistory(value) {
  return Boolean(
    value &&
    Array.isArray(value.old) &&
    Array.isArray(value.new) &&
    Object.prototype.hasOwnProperty.call(value, 'now'),
  )
}
