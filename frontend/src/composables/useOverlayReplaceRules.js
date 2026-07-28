import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function normalizeOverlayReplaceRuleImport(input) {
  const rows = Array.isArray(input)
    ? input
    : Array.isArray(input?.rules)
      ? input.rules
      : []
  return rows.map(normalizeImportedReplaceRule)
}

export function normalizeOverlayReplaceRule(rule = {}) {
  const source = rule || {}
  return {
    ...source,
    scope: explicitReplaceRuleScope(source.scope),
    isRegex: source.isRegex === true,
    enabled: !(source.enabled === false || source.isEnabled === false),
  }
}

export function useOverlayReplaceRules(options) {
  const fallbackOperations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const managerOperations = options.managerOperationGuard || fallbackOperations
  const editorOperations = options.editorOperationGuard || fallbackOperations
  const rules = ref([])
  const loading = ref(false)
  const importing = ref(false)
  const selectedIds = ref([])
  const fileInput = ref(null)
  const dialogVisible = ref(false)
  const saving = ref(false)
  const editingId = ref(null)
  const draft = ref(emptyDraft())
  const scheduleTimeout = options.setTimeout || globalThis.setTimeout
  const cancelTimeout = options.clearTimeout || globalThis.clearTimeout
  let refreshTimer
  let managerRequest = 0

  async function load(parentOperation = null) {
    if (parentOperation && !managerOperations.canCommit(parentOperation)) return false
    const operation = managerOperations.begin('load')
    const request = ++managerRequest
    loading.value = true
    try {
      const { data } = await options.listReplaceRules()
      if (request !== managerRequest || !managerOperations.canCommit(operation)) return false
      rules.value = Array.isArray(data)
        ? data.map(normalizeOverlayReplaceRule)
        : []
      selectedIds.value = selectedIds.value.filter(id => (
        rules.value.some(rule => rule.id === id)
      ))
      return true
    } catch (error) {
      if (request !== managerRequest || !managerOperations.canCommit(operation)) return false
      options.onError(error, '加载替换规则失败')
      return false
    } finally {
      if (request === managerRequest && managerOperations.canCommit(operation)) {
        loading.value = false
      }
    }
  }

  function clearRefresh() {
    if (!refreshTimer) return
    cancelTimeout(refreshTimer)
    refreshTimer = undefined
  }

  function resetManager() {
    managerRequest += 1
    managerOperations.reset()
    clearRefresh()
    rules.value = []
    selectedIds.value = []
    loading.value = false
    importing.value = false
  }

  function scheduleRefresh() {
    clearRefresh()
    const operation = managerOperations.begin('scheduled-refresh')
    refreshTimer = scheduleTimeout(async () => {
      refreshTimer = undefined
      if (!managerOperations.canCommit(operation)) return
      await load(operation)
    }, 250)
  }

  function handleUpdated(event) {
    if (event?.detail?.local || !options.isActive()) return
    scheduleRefresh()
  }

  function changeSelection(rows) {
    selectedIds.value = rows.map(row => row.id)
  }

  function triggerImport() {
    fileInput.value?.click()
  }

  async function importFile(event) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    const operation = managerOperations.begin('import')
    importing.value = true
    try {
      const text = await file.text()
      if (!managerOperations.canCommit(operation)) return
      let parsed
      try {
        parsed = JSON.parse(text)
      } catch {
        options.onError(null, '替换规则文件错误')
        return
      }
      const ruleList = normalizeOverlayReplaceRuleImport(parsed)
      if (!ruleList.length) {
        options.onWarning('替换规则文件中没有可导入的规则')
        return
      }
      await options.confirm(
        `确认要导入文件中的${ruleList.length}条替换规则吗?`,
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!managerOperations.canCommit(operation)) return
      const { data } = await options.upsertReplaceRules(ruleList)
      if (!managerOperations.canCommit(operation)) return
      options.onSuccess(
        `导入替换规则成功：新增 ${data?.created || 0}，更新 ${data?.updated || 0}` +
          (data?.skipped ? `，跳过 ${data.skipped}` : ''),
      )
      await load(operation)
      if (!managerOperations.canCommit(operation)) return
      options.notifyUpdated()
    } catch (error) {
      if (!managerOperations.canCommit(operation)) return
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '导入替换规则失败')
    } finally {
      if (managerOperations.canCommit(operation)) importing.value = false
    }
  }

  function openEditor(rule = null) {
    if (!rule) {
      editingId.value = null
      draft.value = emptyDraft()
      dialogVisible.value = true
      return
    }
    const normalized = normalizeOverlayReplaceRule(rule)
    editingId.value = normalized.id || null
    draft.value = {
      name: normalized.name || '',
      group: normalized.group || '',
      pattern: normalized.pattern || '',
      replacement: normalized.replacement || '',
      scope: normalized.scope || '*',
      isRegex: normalized.isRegex,
      enabled: normalized.enabled,
      order: Number.isFinite(Number(normalized.order))
        ? Number(normalized.order)
        : 0,
    }
    dialogVisible.value = true
  }

  async function save() {
    const name = String(draft.value.name ?? '')
    const pattern = String(draft.value.pattern ?? '')
    const scope = String(draft.value.scope ?? '')
    if (!name) {
      options.onWarning('规则名不能为空')
      return
    }
    if (!pattern) {
      options.onWarning('规则不能为空')
      return
    }
    if (!scope) {
      options.onWarning('替换范围不能为空')
      return
    }
    if (!editingId.value && rules.value.some(rule => rule.name === name)) {
      options.onWarning('规则名不能重复')
      return
    }
    const ruleId = editingId.value
    const payload = normalizeOverlayReplaceRule({
      ...draft.value,
      name,
      pattern,
      scope,
    })
    const operation = editorOperations.begin('save')
    saving.value = true
    try {
      if (ruleId) {
        await options.updateReplaceRule(ruleId, payload)
        if (!editorOperations.canCommit(operation)) return
        options.onSuccess('编辑替换规则成功')
      } else {
        await options.createReplaceRule(payload)
        if (!editorOperations.canCommit(operation)) return
        options.onSuccess('新增替换规则成功')
      }
      dialogVisible.value = false
      if (options.isActive()) await load()
      if (!editorOperations.canCommit(operation)) return
      options.notifyUpdated()
    } catch (error) {
      if (editorOperations.canCommit(operation)) {
        options.onError(error, `${ruleId ? '编辑' : '新增'}替换规则失败`)
      }
    } finally {
      if (editorOperations.canCommit(operation)) saving.value = false
    }
  }

  async function toggle(rule, enabled) {
    const previous = normalizeOverlayReplaceRule(rule).enabled
    const normalized = normalizeOverlayReplaceRule({ ...rule, enabled })
    rule.enabled = normalized.enabled
    rule.isEnabled = normalized.enabled
    const operation = managerOperations.begin(`toggle:${normalized.id}`)
    try {
      await options.updateReplaceRule(normalized.id, {
        name: normalized.name,
        group: normalized.group || '',
        pattern: normalized.pattern,
        replacement: normalized.replacement,
        scope: normalized.scope,
        isRegex: normalized.isRegex,
        enabled: normalized.enabled,
        order: Number.isFinite(Number(normalized.order))
          ? Number(normalized.order)
          : 0,
      })
      if (!managerOperations.canCommit(operation)) return
      options.onSuccess('修改成功')
      options.notifyUpdated()
    } catch (error) {
      if (!managerOperations.canCommit(operation)) return
      rule.enabled = previous
      rule.isEnabled = previous
      options.onError(error, '更新替换规则失败')
      await load(operation)
    }
  }

  async function removeSelected() {
    const ids = [...selectedIds.value]
    if (!ids.length) {
      options.onWarning('请选择需要删除的替换规则')
      return
    }
    const operation = managerOperations.begin('remove-selected')
    try {
      await options.confirm(
        '确认要删除所选择的替换规则吗?',
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!managerOperations.canCommit(operation)) return
      const { data } = await options.deleteReplaceRules(ids)
      if (!managerOperations.canCommit(operation)) return
      const deletedIds = Array.isArray(data?.deletedIds)
        ? data.deletedIds
        : []
      rules.value = rules.value.filter(rule => !deletedIds.includes(rule.id))
      selectedIds.value = []
      options.onSuccess('删除替换规则成功')
      options.notifyUpdated()
    } catch (error) {
      if (!managerOperations.canCommit(operation)) return
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '删除替换规则失败')
    }
  }

  return {
    rules,
    loading,
    importing,
    selectedIds,
    fileInput,
    dialogVisible,
    saving,
    editingId,
    draft,
    load,
    resetManager,
    handleUpdated,
    clearRefresh,
    changeSelection,
    triggerImport,
    importFile,
    normalize: normalizeOverlayReplaceRule,
    openEditor,
    save,
    toggle,
    removeSelected,
    resetOperations() {
      managerOperations.reset()
      if (editorOperations !== managerOperations) editorOperations.reset()
    },
  }
}

function emptyDraft() {
  return {
    name: '',
    group: '',
    pattern: '',
    replacement: '',
    scope: '',
    isRegex: false,
    enabled: true,
    order: 0,
  }
}

function normalizeImportedReplaceRule(rule) {
  const source = rule && typeof rule === 'object' ? rule : {}
  return {
    name: String(firstOwnedValue(source, ['name', 'title']) ?? ''),
    pattern: String(firstOwnedValue(source, ['pattern', 'regex', 'match']) ?? ''),
    replacement: String(source.replacement ?? source.replace ?? ''),
    scope: explicitReplaceRuleScope(source.scope),
    isRegex: source.isRegex === true,
    enabled: !(source.enabled === false || source.isEnabled === false),
    ...(Object.prototype.hasOwnProperty.call(source, 'group')
      ? { group: String(source.group ?? '') }
      : {}),
    ...(Object.prototype.hasOwnProperty.call(source, 'order')
      ? { order: Number.isFinite(Number(source.order)) ? Number(source.order) : 0 }
      : {}),
  }
}

function firstOwnedValue(source, keys) {
  if (!source || typeof source !== 'object') return undefined
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(source, key)) return source[key]
  }
  return undefined
}

function explicitReplaceRuleScope(value) {
  if (value === null || value === undefined || value === '') return '*'
  return String(value)
}
