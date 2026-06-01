<template>
  <section class="app-page sources-page">
    <header class="sources-head">
      <div>
        <h1 class="app-page-title">书源管理</h1>
      </div>
      <div class="head-actions">
        <el-button type="primary" :icon="Plus" @click="openEditor()">新增</el-button>
        <el-button :icon="Download" @click="exportSources">导出</el-button>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".json" @change="importFile">
          <el-button :icon="Upload">导入</el-button>
        </el-upload>
        <el-button :icon="Link" @click="showRemote = true">远程书源</el-button>
        <el-button type="danger" plain :disabled="!sources.length" @click="clearAllSources">清空</el-button>
      </div>
    </header>

    <section class="source-toolbar app-panel">
      <el-input v-model="keyword" placeholder="搜索书源名称、地址或分组" clearable>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="selectedGroup" placeholder="全部分组" clearable>
        <el-option v-for="group in groups" :key="group" :label="group" :value="group" />
      </el-select>
      <el-button :disabled="!selection.length" @click="batchUpdateSources('enable')">启用选中</el-button>
      <el-button :disabled="!selection.length" @click="batchUpdateSources('disable')">停用选中</el-button>
      <el-button :disabled="!selection.length" @click="setSelectedSourceGroup">设置分组</el-button>
      <el-button type="danger" plain :disabled="!selection.length" @click="batchUpdateSources('delete')">删除选中</el-button>
      <el-button :icon="CircleCheck" :loading="checking" @click="checkInvalidSources">失效检测</el-button>
      <el-button type="warning" plain :disabled="!failedHealthSourceIds.length" @click="disableFailedSources">
        停用失败 {{ failedHealthSourceIds.length || '' }}
      </el-button>
      <el-checkbox v-model="failedOnly" :disabled="!healthSummary.total">只看失败</el-checkbox>
      <span v-if="healthSummary.total" class="health-summary">
        已检 {{ healthSummary.total }} · 可用 {{ healthSummary.ok }} · 失败 {{ healthSummary.failed }}
      </span>
    </section>

    <el-table :data="shownSources" stripe class="source-table desktop-source-table" @selection-change="selection = $event">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="名称" min-width="150" show-overflow-tooltip />
      <el-table-column prop="group" label="分组" width="120">
        <template #default="{ row }">{{ row.group || '默认分组' }}</template>
      </el-table-column>
      <el-table-column prop="baseUrl" label="地址" min-width="220" show-overflow-tooltip />
      <el-table-column prop="charset" label="编码" width="90" />
      <el-table-column label="启用" width="76">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" size="small" @change="value => toggleSource(row, value)" />
        </template>
      </el-table-column>
      <el-table-column label="检测" min-width="160">
        <template #default="{ row }">
          <el-tag v-if="health[row.id]" :type="health[row.id].ok ? 'success' : 'danger'" effect="plain">
            {{ health[row.id].ok ? '可用' : health[row.id].message }}
          </el-tag>
          <span v-else class="muted">未检测</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="180" fixed="right">
        <template #default="{ row }">
          <el-button size="small" text type="primary" @click="openDebug(row)">调试</el-button>
          <el-button size="small" text @click="openEditor(row)">编辑</el-button>
          <el-button size="small" text type="danger" @click="deleteSource(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="shownSources.length" class="mobile-source-list">
      <div class="mobile-source-select-actions app-panel">
        <span>已选 {{ selection.length }} 个</span>
        <div>
          <el-button size="small" text @click="selectShownSources">全选当前</el-button>
          <el-button size="small" text @click="selection = []">清空</el-button>
        </div>
      </div>
      <article
        v-for="source in shownSources"
        :key="source.id"
        class="mobile-source-card app-panel"
        :class="{ selected: isSourceSelected(source) }"
      >
        <header>
          <el-checkbox :model-value="isSourceSelected(source)" @change="value => toggleMobileSourceSelection(source, value)" />
          <div>
            <strong>{{ source.name }}</strong>
            <span>{{ source.group || '默认分组' }}</span>
          </div>
          <el-switch :model-value="source.enabled" size="small" @change="value => toggleSource(source, value)" />
        </header>
        <p>{{ source.baseUrl || source.searchUrl || '未设置地址' }}</p>
        <div class="mobile-source-meta">
          <el-tag size="small" effect="plain">{{ source.charset || 'utf-8' }}</el-tag>
          <el-tag v-if="health[source.id]" size="small" :type="health[source.id].ok ? 'success' : 'danger'" effect="plain">
            {{ health[source.id].ok ? '可用' : health[source.id].message }}
          </el-tag>
          <el-tag v-else size="small" effect="plain">未检测</el-tag>
        </div>
        <footer>
          <el-button size="small" text type="primary" @click="openDebug(source)">调试</el-button>
          <el-button size="small" text @click="openEditor(source)">编辑</el-button>
          <el-button size="small" text type="danger" @click="deleteSource(source.id)">删除</el-button>
        </footer>
      </article>
    </div>

    <el-empty v-if="!sources.length" description="还没有书源，导入或新增书源开始使用" />

    <el-dialog v-model="showRemote" title="远程书源" width="460px" :fullscreen="isMobileDialog">
      <el-input v-model="remoteURL" placeholder="输入书源 JSON 订阅地址" />
      <template #footer>
        <el-button @click="showRemote = false">取消</el-button>
        <el-button type="primary" :loading="remoteLoading" @click="importRemote">导入</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="showEditor" :title="editingSourceId ? '编辑书源' : '新增书源'" :direction="drawerDirection" :size="editorDrawerSize">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="sourceForm.name" /></el-form-item>
        <el-form-item label="分组"><el-input v-model="sourceForm.group" placeholder="默认分组" /></el-form-item>
        <el-form-item label="基础地址"><el-input v-model="sourceForm.baseUrl" /></el-form-item>
        <el-form-item label="搜索地址"><el-input v-model="sourceForm.searchUrl" /></el-form-item>
        <el-form-item label="探索地址">
          <el-input v-model="ruleForm.exploreUrl" placeholder="用于书海/发现页的 exploreUrl，可包含 {page}" />
        </el-form-item>
        <el-form-item label="编码">
          <el-select v-model="sourceForm.charset">
            <el-option label="UTF-8" value="utf-8" />
            <el-option label="GBK" value="gbk" />
          </el-select>
        </el-form-item>
        <el-form-item label="常用规则">
          <el-collapse class="rule-collapse">
            <el-collapse-item title="搜索结果" name="search">
              <div class="rule-grid">
                <el-input v-model="ruleForm.bookListRule" placeholder="结果列表规则 bookListRule" />
                <el-input v-model="ruleForm.bookNameRule" placeholder="书名 bookNameRule" />
                <el-input v-model="ruleForm.bookAuthorRule" placeholder="作者 bookAuthorRule" />
                <el-input v-model="ruleForm.bookCoverRule" placeholder="封面 bookCoverRule" />
                <el-input v-model="ruleForm.bookIntroRule" placeholder="简介 bookIntroRule" />
                <el-input v-model="ruleForm.latestChapterRule" placeholder="最新章节 latestChapterRule" />
                <el-input v-model="ruleForm.bookUrlRule" placeholder="详情地址 bookUrlRule" />
                <el-input v-model="ruleForm.paginationRule" placeholder="下一页 paginationRule（可选）" />
              </div>
            </el-collapse-item>
            <el-collapse-item title="目录" name="toc">
              <div class="rule-grid">
                <el-input v-model="ruleForm.tocUrlRule" placeholder="目录地址 tocUrlRule" />
                <el-input v-model="ruleForm.chapterListRule" placeholder="章节列表 chapterListRule" />
                <el-input v-model="ruleForm.chapterNameRule" placeholder="章节名 chapterNameRule" />
                <el-input v-model="ruleForm.chapterUrlRule" placeholder="章节地址 chapterUrlRule" />
              </div>
            </el-collapse-item>
            <el-collapse-item title="正文" name="content">
              <div class="rule-grid">
                <el-input v-model="ruleForm.contentUrlRule" placeholder="正文地址 contentUrlRule" />
                <el-input v-model="ruleForm.contentRule" placeholder="正文内容 contentRule" />
              </div>
            </el-collapse-item>
          </el-collapse>
        </el-form-item>
        <el-form-item label="高级规则 JSON">
          <el-input v-model="sourceForm.rules" type="textarea" :rows="8" placeholder="保留 headers、分页、特殊规则等高级 JSON；上方常用规则会同步写入" />
        </el-form-item>
        <el-form-item label="正文替换规则">
          <div class="replace-rule-editor">
            <div v-for="(rule, index) in replaceRules" :key="index" class="replace-rule-row">
              <el-input v-model="rule.pattern" placeholder="正则或文本" />
              <el-input v-model="rule.replacement" placeholder="替换为" />
              <el-button text type="danger" @click="replaceRules.splice(index, 1)">删除</el-button>
            </div>
            <el-button size="small" plain :icon="Plus" @click="replaceRules.push({ pattern: '', replacement: '' })">添加替换规则</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-switch v-model="sourceForm.enabled" active-text="启用" inactive-text="停用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditor = false">取消</el-button>
        <el-button type="primary" :loading="savingSource" @click="saveSource">保存</el-button>
      </template>
    </el-drawer>

    <el-dialog v-model="showDebug" title="书源调试" width="720px" :fullscreen="isMobileDialog">
      <div class="debug-title">
        <strong>{{ debugSource?.name }}</strong>
        <span>{{ debugSource?.baseUrl }}</span>
      </div>
      <el-tabs v-model="debugTab">
        <el-tab-pane label="搜索" name="search">
          <div class="debug-row">
            <el-input v-model="debugKeyword" placeholder="搜索关键词" />
            <el-button :loading="testing" @click="testSearch">测试搜索</el-button>
          </div>
        </el-tab-pane>
        <el-tab-pane label="目录" name="toc">
          <div class="debug-row">
            <el-input v-model="debugBookURL" placeholder="书籍页 URL" />
            <el-button :loading="testing" @click="testChapter">测试目录</el-button>
          </div>
        </el-tab-pane>
        <el-tab-pane label="正文" name="content">
          <div class="debug-row">
            <el-input v-model="debugChapterURL" placeholder="章节页 URL" />
            <el-button :loading="testing" @click="testContent">测试正文</el-button>
          </div>
        </el-tab-pane>
      </el-tabs>
      <div v-if="debugSearchRows.length" class="debug-next-actions">
        <span>搜索返回 {{ debugSearchRows.length }} 条</span>
        <el-button size="small" type="primary" plain @click="useSearchResultForChapter(debugSearchRows[0])">用第一条测试目录</el-button>
      </div>
      <div v-if="debugChapterRows.length" class="debug-next-actions">
        <span>目录返回 {{ debugChapterRows.length }} 章</span>
        <el-button size="small" type="primary" plain @click="useChapterForContent(debugChapterRows[0])">用第一章测试正文</el-button>
      </div>
      <pre v-if="debugResult" class="debug-pre">{{ debugResultText }}</pre>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CircleCheck, Download, Link, Plus, Search, Upload } from '@element-plus/icons-vue'
import {
  batchSources,
  batchTestSources,
  clearSources,
  createSource,
  deleteSource as deleteSourceApi,
  exportSources as exportSourcesApi,
  importRemoteSource,
  importSources,
  listSources,
  previewRemoteSource,
  testSourceChapter,
  testSourceContent,
  testSourceSearch,
  updateSource,
} from '../api/sources'
import { useReaderStore } from '../stores/reader'

const route = useRoute()
const reader = useReaderStore()
const sources = ref([])
const keyword = ref('')
const selectedGroup = ref('')
const selection = ref([])
const health = ref({})
const checking = ref(false)
const failedOnly = ref(false)

const showRemote = ref(false)
const remoteURL = ref('')
const remoteLoading = ref(false)

const showEditor = ref(false)
const editingSourceId = ref(null)
const savingSource = ref(false)
const sourceForm = reactive({ name: '', group: '', baseUrl: '', searchUrl: '', charset: 'utf-8', rules: '', enabled: true })
const ruleKeys = [
  'exploreUrl',
  'bookListRule',
  'bookNameRule',
  'bookAuthorRule',
  'bookCoverRule',
  'bookIntroRule',
  'latestChapterRule',
  'bookUrlRule',
  'paginationRule',
  'tocUrlRule',
  'chapterListRule',
  'chapterNameRule',
  'chapterUrlRule',
  'contentUrlRule',
  'contentRule',
]
const ruleForm = reactive(Object.fromEntries(ruleKeys.map(key => [key, ''])))
const replaceRules = ref([])

const showDebug = ref(false)
const debugSource = ref(null)
const debugTab = ref('search')
const debugKeyword = ref('')
const debugBookURL = ref('')
const debugChapterURL = ref('')
const debugResult = ref(null)
const testing = ref(false)
const handledRouteAction = ref('')
const MINI_INTERFACE_MAX_WIDTH = 750
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)

const groups = computed(() => [...new Set(sources.value.map(source => source.group || '默认分组'))].sort())
const healthSummary = computed(() => {
  const rows = Object.values(health.value)
  return {
    total: rows.length,
    ok: rows.filter(row => row.ok).length,
    failed: rows.filter(row => !row.ok).length,
  }
})
const failedHealthSourceIds = computed(() =>
  Object.entries(health.value)
    .filter(([, row]) => row && row.ok === false)
    .map(([id]) => Number(id))
    .filter(Boolean),
)
const shownSources = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return sources.value.filter(source => {
    const groupName = source.group || '默认分组'
    if (selectedGroup.value && groupName !== selectedGroup.value) return false
    if (failedOnly.value && health.value[source.id]?.ok !== false) return false
    if (!value) return true
    return `${source.name || ''} ${source.baseUrl || ''} ${source.searchUrl || ''} ${groupName}`.toLowerCase().includes(value)
  })
})
const debugResultText = computed(() => debugResult.value ? JSON.stringify(debugResult.value, null, 2) : '')
const debugSearchRows = computed(() => Array.isArray(debugResult.value?.results) ? debugResult.value.results : [])
const debugChapterRows = computed(() => Array.isArray(debugResult.value?.chapters) ? debugResult.value.chapters : [])
const isMobileDialog = computed(() => reader.pageMode === 'mobile' || windowWidth.value <= MINI_INTERFACE_MAX_WIDTH)
const drawerDirection = computed(() => isMobileDialog.value ? 'btt' : 'rtl')
const editorDrawerSize = computed(() => isMobileDialog.value ? '88%' : '520px')

onMounted(async () => {
  window.addEventListener('resize', handleResize)
  await loadSources()
  applyRouteAction()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
})

watch(
  () => [route.query.panel, route.query.action],
  () => applyRouteAction(),
)

async function loadSources() {
  const { data } = await listSources()
  sources.value = data
}

function applyRouteAction() {
  const signature = `${route.query.panel || ''}:${route.query.action || ''}`
  if (!signature || signature === handledRouteAction.value) return
  handledRouteAction.value = signature
  if (route.query.panel === 'remote') {
    showRemote.value = true
  }
  if (route.query.action === 'health') {
    failedOnly.value = true
    if (!healthSummary.value.total && !checking.value) checkInvalidSources()
  }
}

function handleResize() {
  windowWidth.value = window.innerWidth
}

async function toggleSource(source, enabled) {
  try {
    const { data } = await updateSource(source.id, { ...source, enabled })
    Object.assign(source, data)
  } catch (err) {
    ElMessage.error(readError(err, '操作失败'))
  }
}

function openEditor(source) {
  const parsedRules = parseRules(source?.rules || '')
  editingSourceId.value = source?.id || null
  resetRuleForm(parsedRules)
  Object.assign(sourceForm, {
    name: source?.name || '',
    group: source?.group || '',
    baseUrl: source?.baseUrl || '',
    searchUrl: source?.searchUrl || '',
    charset: source?.charset || 'utf-8',
    rules: source?.rules || '',
    enabled: source?.enabled ?? true,
  })
  replaceRules.value = Array.isArray(parsedRules.textReplaceRules)
    ? parsedRules.textReplaceRules.map(rule => ({ pattern: rule.pattern || '', replacement: rule.replacement || '' }))
    : []
  showEditor.value = true
}

async function saveSource() {
  if (!sourceForm.name.trim()) {
    ElMessage.warning('书源名称不能为空')
    return
  }
  savingSource.value = true
  try {
    const rules = parseRules(sourceForm.rules)
    syncRuleFormToRules(rules)
    const cleanedReplacements = replaceRules.value
      .map(rule => ({ pattern: rule.pattern.trim(), replacement: rule.replacement }))
      .filter(rule => rule.pattern)
    if (cleanedReplacements.length) {
      rules.textReplaceRules = cleanedReplacements
    } else {
      delete rules.textReplaceRules
    }
    const payload = { ...sourceForm, rules: Object.keys(rules).length ? JSON.stringify(rules, null, 2) : '' }
    if (editingSourceId.value) {
      await updateSource(editingSourceId.value, payload)
      ElMessage.success('书源已更新')
    } else {
      await createSource(payload)
      ElMessage.success('书源已新增')
    }
    showEditor.value = false
    await loadSources()
  } catch (err) {
    ElMessage.error(err instanceof SyntaxError ? '规则 JSON 格式不正确' : readError(err, '保存失败'))
  } finally {
    savingSource.value = false
  }
}

async function deleteSource(id) {
  try {
    await ElMessageBox.confirm('确定删除这个书源吗？', '提示', { type: 'warning' })
    await deleteSourceApi(id)
    sources.value = sources.value.filter(source => source.id !== id)
    ElMessage.success('已删除')
  } catch {
    // canceled
  }
}

async function batchUpdateSources(action) {
  if (!selection.value.length) return
  const sourceIds = selection.value.map(source => source.id)
  const actionName = action === 'enable' ? '启用' : action === 'disable' ? '停用' : '删除'
  try {
    if (action === 'delete') {
      await ElMessageBox.confirm(`确定删除选中的 ${sourceIds.length} 个书源吗？`, '批量删除书源', { type: 'warning' })
    }
    const { data } = await batchSources({ action, sourceIds })
    ElMessage.success(`已${actionName} ${data.affected || 0} 个书源`)
    selection.value = []
    await loadSources()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, `批量${actionName}失败`))
  }
}

async function setSelectedSourceGroup() {
  if (!selection.value.length) return
  try {
    const currentGroups = [...new Set(selection.value.map(source => source.group || '').filter(Boolean))]
    const res = await ElMessageBox.prompt(
      '留空将移回默认分组',
      `设置 ${selection.value.length} 个书源的分组`,
      {
        inputValue: currentGroups.length === 1 ? currentGroups[0] : '',
        confirmButtonText: '确定',
        cancelButtonText: '取消',
      },
    )
    const sourceIds = selection.value.map(source => source.id)
    const group = String(res.value || '').trim()
    const { data } = await batchSources({ action: 'group', sourceIds, group })
    ElMessage.success(`已设置 ${data.affected || 0} 个书源分组`)
    selection.value = []
    await loadSources()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '设置分组失败'))
  }
}

async function disableFailedSources() {
  const sourceIds = failedHealthSourceIds.value
  if (!sourceIds.length) return
  try {
    await ElMessageBox.confirm(`确定停用检测失败的 ${sourceIds.length} 个书源吗？`, '停用失败书源', { type: 'warning' })
    const { data } = await batchSources({ action: 'disable', sourceIds })
    ElMessage.success(`已停用 ${data.affected || 0} 个失败书源`)
    for (const id of sourceIds) {
      if (health.value[id]) health.value[id].enabled = false
    }
    selection.value = []
    await loadSources()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '停用失败书源失败'))
  }
}

function isSourceSelected(source) {
  return selection.value.some(item => item.id === source.id)
}

function toggleMobileSourceSelection(source, checked) {
  if (checked) {
    if (!isSourceSelected(source)) selection.value = [...selection.value, source]
    return
  }
  selection.value = selection.value.filter(item => item.id !== source.id)
}

function selectShownSources() {
  selection.value = [...shownSources.value]
}

async function clearAllSources() {
  if (!sources.value.length) return
  try {
    await ElMessageBox.confirm(`确定清空全部 ${sources.value.length} 个书源吗？这个操作不可撤销。`, '清空书源', { type: 'warning' })
    const { data } = await clearSources()
    sources.value = []
    selection.value = []
    health.value = {}
    failedOnly.value = false
    ElMessage.success(`已清空 ${data.affected || 0} 个书源`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清空书源失败'))
  }
}

async function importFile(data) {
  const file = data.raw
  if (!file) return
  try {
    const names = previewSourceNames(JSON.parse(await file.text()))
    await ElMessageBox.confirm(
      names.length
        ? `将导入 ${names.length} 个书源：${names.slice(0, 8).join('、')}${names.length > 8 ? '...' : ''}`
        : '未识别到书源名称，仍要尝试导入吗？',
      '导入书源预览',
      { type: 'info' },
    )
    const form = new FormData()
    form.append('file', file)
    const { data: result } = await importSources(form)
    ElMessage.success(sourceImportMessage(result))
    await loadSources()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '导入失败'))
  }
}

function previewSourceNames(value) {
  const list = Array.isArray(value)
    ? value
    : Array.isArray(value?.bookSources)
      ? value.bookSources
      : Array.isArray(value?.sources)
        ? value.sources
        : value?.name
          ? [value]
          : []
  return list.map(item => item?.name).filter(Boolean)
}

async function importRemote() {
  if (!remoteURL.value.trim()) return
  remoteLoading.value = true
  try {
    const url = remoteURL.value.trim()
    const { data: preview } = await previewRemoteSource(url)
    const names = preview.names || []
    await ElMessageBox.confirm(
      preview.count
        ? `远程订阅包含 ${preview.count} 个书源：${names.slice(0, 8).join('、')}${names.length > 8 ? '...' : ''}`
        : '远程订阅未识别到书源名称，仍要尝试导入吗？',
      '远程书源预览',
      { type: 'info' },
    )
    const { data } = await importRemoteSource(url)
    ElMessage.success(sourceImportMessage(data))
    showRemote.value = false
    remoteURL.value = ''
    await loadSources()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '远程导入失败'))
  } finally {
    remoteLoading.value = false
  }
}

function sourceImportMessage(result = {}) {
  const imported = result.imported || 0
  const updated = result.updated || 0
  const skipped = result.skipped || 0
  return `新增 ${imported} 个，更新 ${updated} 个${skipped ? `，跳过 ${skipped} 个` : ''}`
}

async function exportSources() {
  try {
    const resp = await exportSourcesApi()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([resp.data], { type: 'application/json' }))
    a.download = 'bookSources.json'
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (err) {
    ElMessage.error(readError(err, '导出失败'))
  }
}

async function checkInvalidSources() {
  const list = selection.value.length ? selection.value : shownSources.value
  if (!list.length) return
  checking.value = true
  try {
    const { data } = await batchTestSources({
      sourceIds: list.map(source => source.id),
      keyword: '测试',
    })
    for (const item of data.results || []) {
      health.value[item.sourceId] = {
        ok: item.ok,
        message: item.ok ? `可用，${item.count} 条` : item.message || '失败',
        name: item.name,
        group: item.group || '默认分组',
        enabled: item.enabled,
        count: item.count || 0,
      }
    }
    const failed = (data.results || []).filter(item => !item.ok).length
    ElMessage.success(`已检测 ${data.results?.length || 0} 个书源，失败 ${failed} 个`)
  } catch (err) {
    ElMessage.error(readError(err, '批量检测失败'))
  } finally {
    checking.value = false
  }
}

function openDebug(source) {
  debugSource.value = source
  debugKeyword.value = ''
  debugBookURL.value = ''
  debugChapterURL.value = ''
  debugResult.value = null
  showDebug.value = true
}

function parseRules(value) {
  const raw = String(value || '').trim()
  if (!raw) return {}
  return JSON.parse(raw)
}

function resetRuleForm(rules = {}) {
  for (const key of ruleKeys) {
    ruleForm[key] = rules[key] || ''
  }
}

function syncRuleFormToRules(rules) {
  for (const key of ruleKeys) {
    const value = String(ruleForm[key] || '').trim()
    if (value) {
      rules[key] = value
    } else {
      delete rules[key]
    }
  }
}

async function testSearch() {
  if (!debugKeyword.value.trim()) return
  await runDebug(() => testSourceSearch(debugSource.value.id, debugKeyword.value.trim()))
}

async function testChapter() {
  if (!debugBookURL.value.trim()) return
  await runDebug(() => testSourceChapter(debugSource.value.id, debugBookURL.value.trim()))
}

async function testContent() {
  if (!debugChapterURL.value.trim()) return
  await runDebug(() => testSourceContent(debugSource.value.id, debugChapterURL.value.trim()))
}

async function runDebug(fn) {
  testing.value = true
  try {
    const { data } = await fn()
    debugResult.value = data
  } catch (err) {
    debugResult.value = { error: readError(err, '失败') }
  } finally {
    testing.value = false
  }
}

async function useSearchResultForChapter(row) {
  const bookUrl = row?.bookUrl || row?.url || row?.bookURL
  if (!bookUrl) {
    ElMessage.warning('搜索结果没有书籍地址')
    return
  }
  debugBookURL.value = bookUrl
  debugTab.value = 'toc'
  await testChapter()
}

async function useChapterForContent(row) {
  const chapterUrl = row?.url || row?.chapterUrl || row?.chapterURL
  if (!chapterUrl) {
    ElMessage.warning('目录结果没有章节地址')
    return
  }
  debugChapterURL.value = chapterUrl
  debugTab.value = 'content'
  await testContent()
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.sources-page {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.sources-head,
.head-actions,
.source-toolbar,
.debug-row,
.debug-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.sources-head {
  justify-content: space-between;
}

.head-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.muted,
.debug-title span {
  color: var(--app-text-muted);
}

.source-toolbar {
  min-width: 0;
  flex-wrap: wrap;
  padding: 12px;
}

.source-toolbar .el-input {
  min-width: min(260px, 100%);
  flex: 1;
}

.health-summary {
  color: var(--app-text-muted);
  font-size: 13px;
  white-space: nowrap;
}

.source-table {
  width: 100%;
}

.mobile-source-list {
  display: none;
}

.mobile-source-select-actions {
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 9px 10px;
  color: var(--app-text-muted);
  font-size: 13px;
}

.mobile-source-select-actions,
.mobile-source-select-actions div {
  display: flex;
}

.mobile-source-card {
  display: grid;
  gap: 9px;
  padding: 12px;
}

.mobile-source-card.selected {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.mobile-source-card header,
.mobile-source-card footer,
.mobile-source-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-source-card header {
  justify-content: space-between;
}

.mobile-source-card header > div {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
}

.mobile-source-card strong,
.mobile-source-card span,
.mobile-source-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-source-card strong {
  color: var(--app-text);
  font-size: 15px;
}

.mobile-source-card span,
.mobile-source-card p {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-source-card p {
  margin: 0;
}

.mobile-source-card footer {
  justify-content: flex-end;
}

.debug-title {
  justify-content: space-between;
  margin-bottom: 10px;
}

.debug-row .el-input {
  flex: 1;
}

.debug-next-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 10px;
  padding: 9px 10px;
  color: var(--app-text-muted);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  font-size: 13px;
}

.debug-pre {
  max-height: 320px;
  margin: 12px 0 0;
  overflow: auto;
  padding: 12px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  font-size: 12px;
  white-space: pre-wrap;
}

.replace-rule-editor {
  display: grid;
  width: 100%;
  gap: 8px;
}

.rule-collapse {
  width: 100%;
}

.rule-grid {
  display: grid;
  gap: 8px;
}

.replace-rule-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
}

@media (max-width: 750px) {
  .sources-head,
  .debug-row,
  .source-toolbar,
  .replace-rule-row {
    display: grid;
  }

  .head-actions,
  .source-toolbar :deep(.el-input),
  .source-toolbar :deep(.el-select),
  .source-toolbar :deep(.el-button),
  .debug-row :deep(.el-input),
  .debug-row :deep(.el-button) {
    width: 100%;
  }

  .health-summary {
    white-space: normal;
  }

  .debug-next-actions {
    display: grid;
  }

  .desktop-source-table {
    display: none;
  }

  .mobile-source-list {
    display: grid;
    gap: 10px;
  }
}
</style>
