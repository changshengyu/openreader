import { ref } from 'vue'

export function normalizeOverlayReplaceRuleImport(input) {
  const rows = Array.isArray(input)
    ? input
    : Array.isArray(input?.rules)
      ? input.rules
      : []
  return rows
    .map((rule, index) => ({
      name: String(rule.name || rule.title || `导入规则 ${index + 1}`).trim(),
      pattern: String(rule.pattern || rule.regex || rule.match || '').trim(),
      replacement: String(rule.replacement ?? rule.replace ?? ''),
      scope: String(rule.scope || '*').trim() || '*',
      isRegex: rule.isRegex === true,
      enabled: !(rule.enabled === false || rule.isEnabled === false),
    }))
    .filter(rule => rule.pattern)
}

export function normalizeOverlayReplaceRule(rule = {}) {
  const source = rule || {}
  return {
    ...source,
    scope: String(source.scope || '*').trim() || '*',
    isRegex: source.isRegex === true,
    enabled: !(source.enabled === false || source.isEnabled === false),
  }
}

export function useOverlayReplaceRules(options) {
  const rules = ref([])
  const loading = ref(false)
  const importing = ref(false)
  const selectedIds = ref([])
  const fileInput = ref(null)
  const dialogVisible = ref(false)
  const saving = ref(false)
  const testing = ref(false)
  const editingId = ref(null)
  const draft = ref(emptyDraft())
  const testText = ref('广告123\n正文内容')
  const testResult = ref(null)
  const scheduleTimeout = options.setTimeout || globalThis.setTimeout
  const cancelTimeout = options.clearTimeout || globalThis.clearTimeout
  let refreshTimer

  async function load() {
    loading.value = true
    try {
      const { data } = await options.listReplaceRules()
      rules.value = Array.isArray(data)
        ? data.map(normalizeOverlayReplaceRule)
        : []
      selectedIds.value = selectedIds.value.filter(id => (
        rules.value.some(rule => rule.id === id)
      ))
    } catch (error) {
      options.onError(error, '加载替换规则失败')
    } finally {
      loading.value = false
    }
  }

  function clearRefresh() {
    if (!refreshTimer) return
    cancelTimeout(refreshTimer)
    refreshTimer = undefined
  }

  function scheduleRefresh() {
    clearRefresh()
    refreshTimer = scheduleTimeout(async () => {
      refreshTimer = undefined
      await load()
    }, 250)
  }

  function handleUpdated(event) {
    if (event?.detail?.local || !options.isActive()) return
    scheduleRefresh()
  }

  function changeSelection(rows) {
    selectedIds.value = rows.map(row => row.id)
  }

  function toggleSelection(id, checked) {
    if (checked) {
      if (!selectedIds.value.includes(id)) selectedIds.value.push(id)
      return
    }
    selectedIds.value = selectedIds.value.filter(item => item !== id)
  }

  function triggerImport() {
    fileInput.value?.click()
  }

  async function importFile(event) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    importing.value = true
    try {
      const text = await file.text()
      const parsed = JSON.parse(text)
      const ruleList = normalizeOverlayReplaceRuleImport(parsed)
      if (!ruleList.length) {
        options.onWarning('替换规则文件中没有可导入的规则')
        return
      }
      await options.confirm(
        `确认要导入文件中的 ${ruleList.length} 条替换规则吗？`,
        '导入替换规则',
        { type: 'warning' },
      )
      const { data } = await options.upsertReplaceRules(ruleList)
      options.onSuccess(
        `导入替换规则成功：新增 ${data?.created || 0}，更新 ${data?.updated || 0}` +
        (data?.skipped ? `，跳过 ${data.skipped}` : ''),
      )
      await load()
      options.notifyUpdated()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '导入替换规则失败')
    } finally {
      importing.value = false
    }
  }

  function openEditor(rule = null) {
    if (!rule) {
      editingId.value = null
      draft.value = emptyDraft()
      testResult.value = null
      dialogVisible.value = true
      return
    }
    const normalized = normalizeOverlayReplaceRule(rule)
    editingId.value = normalized.id || null
    draft.value = {
      name: normalized.name || '',
      pattern: normalized.pattern || '',
      replacement: normalized.replacement || '',
      scope: normalized.scope || '*',
      isRegex: normalized.isRegex,
      enabled: normalized.enabled,
    }
    testResult.value = null
    dialogVisible.value = true
  }

  async function save() {
    const name = String(draft.value.name || '').trim()
    const pattern = String(draft.value.pattern || '').trim()
    const scope = String(draft.value.scope || '').trim()
    if (!name) {
      options.onWarning('规则名不能为空')
      return
    }
    if (!pattern) {
      options.onWarning('匹配规则不能为空')
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
    saving.value = true
    try {
      const payload = normalizeOverlayReplaceRule({
        ...draft.value,
        name,
        pattern,
        scope,
      })
      if (editingId.value) {
        await options.updateReplaceRule(editingId.value, payload)
        options.onSuccess('替换规则已更新')
      } else {
        await options.createReplaceRule(payload)
        options.onSuccess('替换规则已创建')
      }
      dialogVisible.value = false
      await load()
      options.notifyUpdated()
    } catch (error) {
      options.onError(error, '保存替换规则失败')
    } finally {
      saving.value = false
    }
  }

  async function toggle(rule, enabled) {
    const normalized = normalizeOverlayReplaceRule({ ...rule, enabled })
    try {
      await options.updateReplaceRule(normalized.id, {
        name: normalized.name,
        pattern: normalized.pattern,
        replacement: normalized.replacement,
        scope: normalized.scope,
        isRegex: normalized.isRegex,
        enabled: normalized.enabled,
      })
      rule.enabled = normalized.enabled
      rule.isEnabled = normalized.enabled
      options.onSuccess(normalized.enabled ? '规则已启用' : '规则已停用')
      options.notifyUpdated()
    } catch (error) {
      options.onError(error, '更新替换规则失败')
      await load()
    }
  }

  async function runTest() {
    if (!draft.value.pattern.trim() || !testText.value) {
      options.onWarning('请输入匹配规则和测试文本')
      return
    }
    testing.value = true
    try {
      const { data } = await options.testReplaceRule({
        pattern: draft.value.pattern,
        replacement: draft.value.replacement,
        isRegex: draft.value.isRegex,
        text: testText.value,
      })
      testResult.value = data
    } catch (error) {
      options.onError(error, '测试替换规则失败')
    } finally {
      testing.value = false
    }
  }

  async function remove(rule) {
    try {
      await options.confirm(
        `确定删除替换规则“${rule.name || rule.pattern}”吗？`,
        '删除替换规则',
        { type: 'warning' },
      )
      await options.deleteReplaceRule(rule.id)
      rules.value = rules.value.filter(item => item.id !== rule.id)
      selectedIds.value = selectedIds.value.filter(id => id !== rule.id)
      options.onSuccess('替换规则已删除')
      options.notifyUpdated()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '删除替换规则失败')
    }
  }

  async function removeSelected() {
    const ids = [...selectedIds.value]
    if (!ids.length) {
      options.onWarning('请选择需要删除的替换规则')
      return
    }
    try {
      await options.confirm(
        `确认要删除所选择的 ${ids.length} 条替换规则吗？`,
        '批量删除替换规则',
        { type: 'warning' },
      )
      const { data } = await options.deleteReplaceRules(ids)
      const deletedIds = Array.isArray(data?.deletedIds)
        ? data.deletedIds
        : []
      rules.value = rules.value.filter(rule => !deletedIds.includes(rule.id))
      selectedIds.value = []
      options.onSuccess('删除替换规则成功')
      options.notifyUpdated()
    } catch (error) {
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
    testing,
    editingId,
    draft,
    testText,
    testResult,
    load,
    handleUpdated,
    clearRefresh,
    changeSelection,
    toggleSelection,
    triggerImport,
    importFile,
    normalize: normalizeOverlayReplaceRule,
    openEditor,
    save,
    toggle,
    runTest,
    remove,
    removeSelected,
  }
}

function emptyDraft() {
  return {
    name: '',
    pattern: '',
    replacement: '',
    scope: '',
    isRegex: false,
    enabled: true,
  }
}
