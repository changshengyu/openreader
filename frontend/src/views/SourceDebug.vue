<template>
  <div class="source-debug-workspace">
    <header class="source-debug-header">
      <div>
        <strong>书源调试</strong>
        <span>保存当前配置后，按上游顺序自动执行完整链路</span>
      </div>
      <div class="source-debug-runbar">
        <el-select v-model="selectedSourceId" filterable placeholder="选择书源" @change="loadSelectedSource">
          <el-option v-for="item in sources" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
        <el-input v-model="debugKeyword" placeholder="输入关键词或快捷指令，格式见帮助" @keyup.enter="runDebug" />
        <el-button type="primary" :loading="debugging" @click="runDebug">开始调试</el-button>
        <el-button v-if="debugging" @click="stopDebug">停止</el-button>
      </div>
    </header>

    <main class="source-debug-layout">
      <section class="source-debug-rule-pane">
        <section class="source-debug-rule-group">
          <h2>基本</h2>
          <div class="source-debug-basic-grid">
            <label><span>名称</span><el-input v-model="sourceForm.name" /></label>
            <label><span>分组</span><el-input v-model="sourceForm.group" /></label>
            <label class="wide"><span>书源地址</span><el-input v-model="sourceForm.baseUrl" /></label>
            <label><span>字符集</span><el-input v-model="sourceForm.charset" /></label>
            <label><span>并发率</span><el-input v-model="sourceForm.concurrentRate" /></label>
            <label class="wide"><span>书籍 URL 正则</span><el-input v-model="sourceForm.bookUrlPattern" /></label>
            <label class="wide"><span>请求头</span><el-input v-model="sourceForm.header" type="textarea" :rows="2" /></label>
            <label class="wide"><span>登录地址</span><el-input v-model="sourceForm.loginUrl" /></label>
            <label class="wide"><span>登录检测脚本</span><el-input v-model="sourceForm.loginCheckJs" type="textarea" :rows="2" /></label>
            <label class="wide"><span>备注</span><el-input v-model="sourceForm.bookSourceComment" type="textarea" :rows="2" /></label>
            <div class="source-debug-switches wide">
              <el-switch v-model="sourceForm.enabled" active-text="启用书源" />
              <el-switch v-model="sourceForm.enabledExplore" active-text="启用探索功能" />
            </div>
          </div>
        </section>

        <section class="source-debug-rule-group">
          <h2>搜索</h2>
          <RuleFields v-model="rules" :fields="searchFields" />
        </section>

        <section class="source-debug-rule-group">
          <h2>发现</h2>
          <RuleFields v-model="rules" :fields="exploreFields" />
        </section>

        <section class="source-debug-rule-group">
          <h2>详情</h2>
          <RuleFields v-model="rules" :fields="bookInfoFields" />
        </section>

        <section class="source-debug-rule-group">
          <h2>目录</h2>
          <RuleFields v-model="rules" :fields="tocFields" />
        </section>

        <section class="source-debug-rule-group">
          <h2>正文</h2>
          <RuleFields v-model="rules" :fields="contentFields" />
        </section>

        <section class="source-debug-rule-group">
          <h2>其它规则</h2>
          <RuleFields v-model="rules" :fields="otherFields" />
        </section>
      </section>

      <aside class="source-debug-command-rail" aria-label="调试命令">
        <button type="button" @click="pushLocalSource">推送源</button>
        <button type="button" @click="pullLocalSource">拉取源</button>
        <button type="button" @click="showJSONEditor">编辑源</button>
        <button type="button" @click="generateSourceJSON">生成源</button>
        <button type="button" @click="clearForm">清空表单</button>
        <button type="button" :disabled="!canUndo" @click="undoForm">撤销操作</button>
        <button type="button" :disabled="!canRedo" @click="redoForm">重做操作</button>
        <button type="button" :disabled="debugging" @click="runDebug">调试源</button>
        <button type="button" :disabled="saving" @click="saveCurrentSource()">保存源</button>
      </aside>

      <section class="source-debug-output-pane">
        <el-tabs v-model="outputTab" stretch>
          <el-tab-pane label="编辑源" name="editor">
            <el-input v-model="sourceJSON" type="textarea" :rows="22" spellcheck="false" />
            <div class="source-debug-pane-actions">
              <el-button @click="generateSourceJSON">从表单生成</el-button>
              <el-button type="primary" @click="applySourceJSON">应用到表单</el-button>
            </div>
          </el-tab-pane>
          <el-tab-pane label="调试源" name="debugger">
            <p v-if="debugCompatibilityMessage" class="source-debug-compatibility-warning">{{ debugCompatibilityMessage }}</p>
            <div ref="debugConsoleElement" class="source-debug-console" role="log" aria-live="polite">
              <p v-if="!debugEvents.length">输入关键词后按回车，或点击“开始调试”。空关键词默认使用“我的”。</p>
              <article v-for="event in debugEvents" :key="`${event.data.seq || 0}:${event.type}`" :class="`event-${event.type}`">
                <span>{{ event.data.seq || '-' }}</span>
                <strong>{{ debugStageLabel(event.data.stage) }}</strong>
                <em>{{ event.data.status || event.type }}</em>
                <p>{{ event.data.message || '' }}<template v-if="event.data.count != null"> · {{ event.data.count }}</template></p>
              </article>
            </div>
          </el-tab-pane>
          <el-tab-pane label="源列表" name="sources">
            <div class="source-debug-list-actions">
              <el-button size="small" @click="rememberCurrentSource">记录当前</el-button>
              <el-button size="small" @click="openSourceListImport">导入</el-button>
              <el-button size="small" :disabled="!localSources.length" @click="exportLocalSources">导出</el-button>
              <el-button size="small" type="danger" plain :disabled="!selectedLocalKey" @click="deleteSelectedLocalSource">删除选中</el-button>
              <el-button size="small" type="danger" plain :disabled="!localSources.length" @click="clearLocalSources">清空列表</el-button>
              <input ref="sourceListInput" class="source-debug-file-input" type="file" accept=".json,application/json" @change="importLocalSources" />
            </div>
            <div class="source-debug-source-list">
              <button
                v-for="item in localSources"
                :key="localSourceKey(item)"
                type="button"
                :class="{ selected: selectedLocalKey === localSourceKey(item) }"
                @click="selectLocalSource(item)"
              >
                <strong>{{ item.name || '本地草稿' }}</strong><span>{{ item.baseUrl || item.searchUrl }}</span>
              </button>
            </div>
          </el-tab-pane>
          <el-tab-pane label="帮助信息" name="help">
            <div class="source-debug-help">
              <p><code>普通关键词</code>：搜索 → 第一条详情 → 目录 → 第一章正文。</p>
              <p><code>详情 URL</code>：详情 → 目录 → 第一章正文。</p>
              <p><code>分类::URL</code>：发现 → 第一条详情 → 目录 → 第一章正文。</p>
              <p><code>++URL</code>：从目录开始；<code>--URL</code>：只解析正文。</p>
              <p>服务端日志只返回阶段、数量和耗时，不返回请求头、Cookie、正文或变量值。</p>
            </div>
          </el-tab-pane>
        </el-tabs>
      </section>
    </main>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElInput, ElMessage, ElMessageBox } from 'element-plus'
import { useRoute } from 'vue-router'
import { createSource, debugSourceStream, deleteSource, importSources, listSources, updateSource } from '../api/sources'
import { createSourceImportForm, parseImportSourceList } from '../composables/useSourceTransfer'
import { useUserStore } from '../stores/user'
import { currentUserScope } from '../utils/authScope'
import {
  buildBookSourcePayload,
  buildReaderDevBookSource,
  createBookSourceForm,
  createBookSourceRuleForm,
  sourceToEditorSnapshot,
} from '../utils/bookSourceEditor'
import {
  createSourceDebugHistory,
  loadSourceDebugSources,
  saveSourceDebugSources,
} from '../utils/sourceDebugState'

const RuleFields = defineComponent({
  name: 'RuleFields',
  props: {
    modelValue: { type: Object, required: true },
    fields: { type: Array, required: true },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('div', { class: 'source-debug-field-list' }, props.fields.map(field =>
      h('label', { key: field.key }, [
        h('span', field.label),
        h(ElInput, {
          modelValue: props.modelValue[field.key] || '',
          type: field.rows > 1 ? 'textarea' : 'text',
          rows: field.rows || 1,
          spellcheck: false,
          'onUpdate:modelValue': (value) => emit('update:modelValue', { ...props.modelValue, [field.key]: value }),
        }),
      ]),
    ))
  },
})

const route = useRoute()
const userStore = useUserStore()
const sources = ref([])
const localSources = ref([])
const selectedSourceId = ref(null)
const sourceForm = reactive(createBookSourceForm())
const rules = ref(createBookSourceRuleForm())
const debugKeyword = ref('我的')
const outputTab = ref('debugger')
const sourceJSON = ref('')
const debugEvents = ref([])
const debugging = ref(false)
const saving = ref(false)
const debugConsoleElement = ref(null)
const scope = currentUserScope()
const history = createSourceDebugHistory({ storage: localStorage, scope, initial: snapshot() })
const historyVersion = ref(0)
const selectedLocalKey = ref('')
const sourceListInput = ref(null)
let debugRunGeneration = 0
let debugController = null
let historyTimer = 0
let applyingHistory = false

const searchFields = [
  { key: 'searchUrl', label: '搜索地址', rows: 2 },
  { key: 'bookListRule', label: '书籍列表' },
  { key: 'bookNameRule', label: '书名' },
  { key: 'bookAuthorRule', label: '作者' },
  { key: 'bookCoverRule', label: '封面' },
  { key: 'bookIntroRule', label: '简介' },
  { key: 'bookKindRule', label: '分类' },
  { key: 'bookWordCountRule', label: '字数' },
  { key: 'latestChapterRule', label: '最新章节' },
  { key: 'bookUpdateTimeRule', label: '更新时间' },
  { key: 'bookUrlRule', label: '详情地址' },
]
const exploreFields = [
  { key: 'exploreUrl', label: '发现地址', rows: 2 },
  { key: 'exploreBookListRule', label: '书籍列表' },
  { key: 'exploreBookNameRule', label: '书名' },
  { key: 'exploreBookAuthorRule', label: '作者' },
  { key: 'exploreBookCoverRule', label: '封面' },
  { key: 'exploreBookIntroRule', label: '简介' },
  { key: 'exploreBookKindRule', label: '分类' },
  { key: 'exploreBookWordCountRule', label: '字数' },
  { key: 'exploreLatestChapterRule', label: '最新章节' },
  { key: 'exploreBookUpdateTimeRule', label: '更新时间' },
  { key: 'exploreBookUrlRule', label: '详情地址' },
  { key: 'explorePaginationRule', label: '下一页' },
]
const bookInfoFields = [
  { key: 'bookInfoInitRule', label: '预处理' },
  { key: 'bookInfoNameRule', label: '书名' },
  { key: 'bookInfoAuthorRule', label: '作者' },
  { key: 'bookInfoCoverRule', label: '封面' },
  { key: 'bookInfoIntroRule', label: '简介' },
  { key: 'bookInfoKindRule', label: '分类' },
  { key: 'bookInfoLatestChapterRule', label: '最新章节' },
  { key: 'bookInfoUpdateTimeRule', label: '更新时间' },
  { key: 'bookInfoWordCountRule', label: '字数' },
  { key: 'bookInfoCanRenameRule', label: '允许改名' },
]
const tocFields = [
  { key: 'tocUrlRule', label: '目录地址', rows: 2 },
  { key: 'chapterPreUpdateJsRule', label: '预处理脚本' },
  { key: 'chapterListRule', label: '章节列表' },
  { key: 'chapterNameRule', label: '章节名' },
  { key: 'chapterUrlRule', label: '章节地址' },
  { key: 'chapterIsVolumeRule', label: '卷标识' },
  { key: 'chapterIsVipRule', label: 'VIP 标识' },
  { key: 'chapterUpdateTimeRule', label: '更新时间' },
  { key: 'nextTocUrlRule', label: '下一页' },
]
const contentFields = [
  { key: 'contentUrlRule', label: '正文地址', rows: 2 },
  { key: 'contentRule', label: '正文规则', rows: 3 },
  { key: 'nextContentUrlRule', label: '下一页' },
  { key: 'contentWebJsRule', label: 'WebView 脚本', rows: 2 },
  { key: 'contentSourceRegex', label: '源文本正则' },
  { key: 'contentReplaceRegex', label: '替换正则' },
  { key: 'contentImageStyle', label: '图片样式' },
]
const otherFields = [
  { key: 'paginationRule', label: '搜索下一页' },
]

const canUndo = computed(() => {
  historyVersion.value
  return history.snapshot().old.length > 0
})
const canRedo = computed(() => {
  historyVersion.value
  return history.snapshot().new.length > 0
})
const debugCompatibilityMessage = computed(() => {
  const unsupported = debugEvents.value.find(event => event.data?.code === 'source_rule_unsupported')
  if (!unsupported) return ''
  return `当前服务不会执行此书源在${debugStageLabel(unsupported.data.stage)}阶段需要的 JavaScript 或 WebView；配置会保留。`
})

onMounted(async () => {
  localSources.value = loadSourceDebugSources(localStorage, scope)
  const tab = new URLSearchParams(window.location.hash.replace(/^#/, '')).get('tab')
  if (['editor', 'debugger', 'sources', 'help'].includes(tab)) outputTab.value = tab
  try {
    if (!userStore.profile) await userStore.loadMe()
    await loadRemoteSources()
    const requested = Number(route.query.sourceId)
    const initial = sources.value.find(item => item.id === requested) || sources.value[0]
    if (initial) loadSource(initial)
    else clearForm()
  } catch (error) {
    ElMessage.error(readError(error, '加载书源失败'))
  }
})

onBeforeUnmount(() => {
  if (historyTimer) window.clearTimeout(historyTimer)
  debugController?.abort()
})

watch([sourceForm, rules], () => {
  if (applyingHistory) return
  if (historyTimer) window.clearTimeout(historyTimer)
  historyTimer = window.setTimeout(() => {
    history.commit(snapshot())
    historyVersion.value += 1
  }, 450)
}, { deep: true })

watch(outputTab, (tab) => {
  const url = new URL(window.location.href)
  url.hash = `tab=${tab}`
  window.history.replaceState(window.history.state, '', url)
})

async function loadRemoteSources() {
  const { data } = await listSources()
  sources.value = Array.isArray(data) ? data : []
}

function loadSelectedSource() {
  const source = sources.value.find(item => Number(item.id) === Number(selectedSourceId.value))
  if (source) loadSource(source)
}

function loadSource(source) {
  applySnapshot(sourceSnapshot(source), true)
  selectedSourceId.value = source.id || null
}

function sourceSnapshot(source) {
  const editor = sourceToEditorSnapshot(source)
  return {
    ...editor.form,
    rules: editor.rules,
  }
}

function snapshot() {
  return {
    ...JSON.parse(JSON.stringify(sourceForm)),
    rules: JSON.parse(JSON.stringify(rules.value)),
  }
}

function applySnapshot(value, resetHistory = false) {
  applyingHistory = true
  const next = value?.rules && typeof value.rules === 'object' ? value : sourceSnapshot(value)
  Object.assign(sourceForm, createBookSourceForm(), next)
  delete sourceForm.rules
  rules.value = { ...createBookSourceRuleForm(), ...(next.rules || {}) }
  sourceJSON.value = JSON.stringify(readerDevPayload(), null, 2)
  if (resetHistory) {
    history.reset(snapshot())
    historyVersion.value += 1
  }
  nextTick(() => { applyingHistory = false })
}

function payload() {
  return buildBookSourcePayload(sourceForm, rules.value)
}

function readerDevPayload() {
  return buildReaderDevBookSource(sourceForm, rules.value)
}

async function saveCurrentSource(options = {}) {
  if (!String(sourceForm.name || '').trim()) {
    ElMessage.warning('书源名称不能为空')
    throw new Error('source name is required')
  }
  if (userStore.profile?.canEditSources === false) {
    ElMessage.warning('当前账号没有书源编辑权限')
    throw new Error('source editing is disabled')
  }
  const creating = !sourceForm.id
  saving.value = true
  try {
    const response = sourceForm.id
      ? await updateSource(sourceForm.id, payload())
      : await createSource(payload())
    loadSource(response.data)
    rememberCurrentSource(false)
    await loadRemoteSources()
    if (!options.quiet) ElMessage.success(creating ? '书源已创建' : '书源已保存')
    return response.data
  } finally {
    saving.value = false
  }
}

async function runDebug() {
  const runGeneration = ++debugRunGeneration
  debugController?.abort()
  try {
    const saved = await saveCurrentSource({ quiet: true })
    debugController = new AbortController()
    debugEvents.value = []
    outputTab.value = 'debugger'
    debugging.value = true
    await debugSourceStream(saved.id, debugKeyword.value, {
      signal: debugController.signal,
      onEvent: appendDebugEvent,
    })
  } catch (error) {
    if (error?.name !== 'AbortError') {
      ElMessage.error(readError(error, '调试失败'))
    }
  } finally {
    if (runGeneration === debugRunGeneration) debugging.value = false
  }
}

function stopDebug() {
  debugController?.abort()
}

function appendDebugEvent(event) {
  debugEvents.value = [...debugEvents.value.slice(-127), event]
  nextTick(() => {
    if (debugConsoleElement.value) debugConsoleElement.value.scrollTop = debugConsoleElement.value.scrollHeight
  })
}

function rememberCurrentSource(notify = true) {
  const current = snapshot()
  const key = localSourceKey(current)
  if (!current.name && !current.baseUrl) {
    if (notify) ElMessage.info('当前表单还是空的')
    return null
  }
  const index = localSources.value.findIndex(item => localSourceKey(item) === key)
  if (index >= 0) localSources.value.splice(index, 1, current)
  else localSources.value.unshift(current)
  localSources.value = localSources.value.slice(0, 50)
  saveSourceDebugSources(localStorage, scope, localSources.value)
  selectedLocalKey.value = key
  if (notify) ElMessage.success('已记录当前书源')
  return current
}

async function pushLocalSource() {
  try {
    rememberCurrentSource(false)
    if (!localSources.value.length) {
      ElMessage.info('源列表为空')
      return
    }
    const rows = localSources.value.map(item => buildReaderDevBookSource(item, item.rules || {}))
    const { data } = await importSources(createSourceImportForm(rows))
    await loadRemoteSources()
    ElMessage.success(`已推送 ${Number(data.imported || 0) + Number(data.updated || 0)} 条书源`)
  } catch (error) {
    ElMessage.error(readError(error, '推送书源失败'))
  }
}

async function pullLocalSource() {
  try {
    await loadRemoteSources()
    localSources.value = sources.value.map(sourceSnapshot)
    saveSourceDebugSources(localStorage, scope, localSources.value)
    selectedLocalKey.value = ''
    outputTab.value = 'sources'
    ElMessage.success(`已拉取 ${localSources.value.length} 条书源`)
  } catch (error) {
    ElMessage.error(readError(error, '拉取书源失败'))
  }
}

function localSourceKey(item) {
  return String(item?.baseUrl || item?.bookSourceUrl || item?.id || item?.name || item?.bookSourceName || '')
}

function selectLocalSource(item) {
  selectedLocalKey.value = localSourceKey(item)
  applySnapshot(item, true)
  selectedSourceId.value = item.id || null
}

function openSourceListImport() {
  sourceListInput.value?.click()
}

async function importLocalSources(event) {
  const input = event?.target
  const file = input?.files?.[0]
  if (!file) return
  try {
    const rows = parseImportSourceList(JSON.parse(await file.text())).map(sourceSnapshot)
    if (!rows.length) throw new Error('source list is empty')
    let replace = true
    if (localSources.value.length) {
      try {
        await ElMessageBox.confirm('选择“覆盖”将替换当前调试列表；选择“追加”会按书源地址去重。', '导入源列表', {
          confirmButtonText: '覆盖',
          cancelButtonText: '追加',
          distinguishCancelAndClose: true,
        })
      } catch (action) {
        if (action === 'close') return
        replace = false
      }
    }
    if (replace) {
      localSources.value = rows.slice(0, 5000)
    } else {
      const merged = [...localSources.value]
      const known = new Set(merged.map(localSourceKey))
      for (const row of rows) {
        const key = localSourceKey(row)
        if (!key || known.has(key)) continue
        known.add(key)
        merged.push(row)
      }
      localSources.value = merged.slice(0, 5000)
    }
    saveSourceDebugSources(localStorage, scope, localSources.value)
    ElMessage.success(`已导入 ${rows.length} 条书源`)
  } catch (error) {
    ElMessage.error(readError(error, '导入源文件失败'))
  } finally {
    if (input) input.value = ''
  }
}

function exportLocalSources() {
  const link = document.createElement('a')
  const blob = new Blob([JSON.stringify(localSources.value.map(item => buildReaderDevBookSource(item, item.rules || {})), null, 2)], {
    type: 'application/json',
  })
  link.href = URL.createObjectURL(blob)
  link.download = `OpenReader-source-debug-${new Date().toISOString().replace(/[:.]/g, '-')}.json`
  link.click()
  URL.revokeObjectURL(link.href)
}

async function deleteSelectedLocalSource() {
  const item = localSources.value.find(row => localSourceKey(row) === selectedLocalKey.value)
  if (!item) return
  try {
    await ElMessageBox.confirm('确定删除选中的书源吗？已保存到应用的同一书源也会删除。', '删除书源', { type: 'warning' })
    if (item.id) await deleteSource(item.id)
    localSources.value = localSources.value.filter(row => localSourceKey(row) !== selectedLocalKey.value)
    saveSourceDebugSources(localStorage, scope, localSources.value)
    selectedLocalKey.value = ''
    await loadRemoteSources()
    if (Number(sourceForm.id) === Number(item.id)) clearForm()
    ElMessage.success('书源已删除')
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(readError(error, '删除书源失败'))
  }
}

async function clearLocalSources() {
  try {
    await ElMessageBox.confirm('只清空当前账号的浏览器调试列表，不删除应用中的书源。', '清空源列表', { type: 'warning' })
    localSources.value = []
    selectedLocalKey.value = ''
    saveSourceDebugSources(localStorage, scope, [])
  } catch {
    // Canceled.
  }
}

function showJSONEditor() {
  generateSourceJSON()
  outputTab.value = 'editor'
}

function generateSourceJSON() {
  sourceJSON.value = JSON.stringify(readerDevPayload(), null, 2)
  outputTab.value = 'editor'
}

function applySourceJSON() {
  try {
    const value = JSON.parse(sourceJSON.value)
    applySnapshot(sourceSnapshot(value), true)
    ElMessage.success('JSON 已应用到表单')
  } catch {
    ElMessage.error('书源 JSON 格式不正确')
  }
}

function clearForm() {
  selectedSourceId.value = null
  selectedLocalKey.value = ''
  applySnapshot({ ...createBookSourceForm(), rules: createBookSourceRuleForm() }, true)
}

function undoForm() {
  applySnapshot(history.undo())
  historyVersion.value += 1
}

function redoForm() {
  applySnapshot(history.redo())
  historyVersion.value += 1
}

function debugStageLabel(stage) {
  return {
    search: '搜索',
    explore: '发现',
    book_info: '详情',
    toc: '目录',
    content: '正文',
  }[stage] || '链路'
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || error?.message || fallback
}
</script>

<style scoped>
.source-debug-workspace {
  min-height: 100vh;
  padding: 14px;
  color: var(--app-text);
  background: var(--app-bg);
}

.source-debug-header {
  position: sticky;
  z-index: 10;
  top: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: -14px -14px 14px;
  padding: 12px 14px;
  background: color-mix(in srgb, var(--app-bg) 94%, transparent);
  border-bottom: 1px solid var(--app-border);
  backdrop-filter: blur(12px);
}

.source-debug-header > div:first-child {
  display: grid;
  gap: 3px;
}

.source-debug-header strong {
  font-size: 18px;
}

.source-debug-header span,
.source-debug-source-list span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.source-debug-runbar {
  display: flex;
  min-width: min(720px, 65vw);
  gap: 8px;
}

.source-debug-runbar :deep(.el-select) {
  width: 210px;
}

.source-debug-layout {
  display: grid;
  grid-template-columns: minmax(420px, 1.2fr) 92px minmax(360px, 1fr);
  min-height: calc(100vh - 88px);
  gap: 12px;
}

.source-debug-rule-pane,
.source-debug-output-pane {
  min-width: 0;
  overflow: auto;
  padding: 14px;
  background: var(--app-panel);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius);
}

.source-debug-rule-pane {
  max-height: calc(100vh - 100px);
}

.source-debug-rule-group + .source-debug-rule-group {
  margin-top: 18px;
  padding-top: 15px;
  border-top: 1px solid var(--app-border);
}

.source-debug-rule-group h2 {
  margin: 0 0 10px;
  color: var(--app-primary);
  font-size: 15px;
}

.source-debug-basic-grid,
.source-debug-field-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.source-debug-basic-grid label,
.source-debug-field-list label {
  display: grid;
  min-width: 0;
  gap: 5px;
}

.source-debug-basic-grid label > span,
.source-debug-field-list label > span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.source-debug-basic-grid .wide,
.source-debug-field-list label:has(textarea) {
  grid-column: 1 / -1;
}

.source-debug-switches {
  display: flex;
  gap: 18px;
}

.source-debug-command-rail {
  position: sticky;
  top: 74px;
  align-self: start;
  display: grid;
  overflow: hidden;
  background: var(--app-panel);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius);
}

.source-debug-command-rail button {
  min-height: 43px;
  padding: 6px;
  color: var(--app-text);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
}

.source-debug-command-rail button:last-child {
  border-bottom: 0;
}

.source-debug-command-rail button:hover:not(:disabled) {
  color: var(--app-primary);
  background: var(--app-primary-soft);
}

.source-debug-command-rail button:disabled {
  opacity: 0.45;
  cursor: default;
}

.source-debug-output-pane {
  max-height: calc(100vh - 100px);
}

.source-debug-pane-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
  gap: 8px;
}

.source-debug-console {
  display: grid;
  max-height: calc(100vh - 190px);
  overflow: auto;
  gap: 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.source-debug-console > p,
.source-debug-console article {
  margin: 0;
  padding: 9px 10px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.source-debug-console article {
  display: grid;
  grid-template-columns: 28px 58px 58px minmax(0, 1fr);
  align-items: center;
  gap: 7px;
}

.source-debug-console article p {
  margin: 0;
}

.source-debug-console article em {
  color: var(--app-text-muted);
  font-style: normal;
}

.source-debug-console .event-error {
  color: var(--el-color-danger);
  border-color: color-mix(in srgb, var(--el-color-danger) 40%, var(--app-border));
}

.source-debug-compatibility-warning {
  padding: 10px;
  color: var(--el-color-warning-dark-2);
  background: var(--el-color-warning-light-9);
  border: 1px solid var(--el-color-warning-light-5);
  border-radius: var(--app-radius-sm);
}

.source-debug-source-list {
  display: grid;
  gap: 8px;
}

.source-debug-list-actions {
  display: flex;
  flex-wrap: wrap;
  margin-bottom: 10px;
  gap: 6px;
}

.source-debug-file-input {
  display: none;
}

.source-debug-source-list button {
  display: grid;
  justify-items: start;
  gap: 3px;
  padding: 10px;
  color: var(--app-text);
  text-align: left;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  cursor: pointer;
}

.source-debug-source-list button.selected {
  color: var(--app-primary);
  background: var(--app-primary-soft);
  border-color: var(--app-primary);
}

.source-debug-help {
  line-height: 1.7;
}

@media (max-width: 980px) {
  .source-debug-header {
    position: static;
    display: grid;
  }

  .source-debug-runbar {
    display: grid;
    grid-template-columns: 1fr auto auto;
    min-width: 0;
  }

  .source-debug-runbar :deep(.el-select) {
    grid-column: 1 / -1;
    width: 100%;
  }

  .source-debug-layout {
    display: flex;
    flex-direction: column;
  }

  .source-debug-rule-pane,
  .source-debug-output-pane {
    max-height: none;
    overflow: visible;
  }

  .source-debug-command-rail {
    position: static;
    grid-template-columns: repeat(3, 1fr);
  }

  .source-debug-command-rail button {
    border-right: 1px solid var(--app-border);
  }
}

@media (max-width: 560px) {
  .source-debug-workspace {
    padding: 8px;
  }

  .source-debug-header {
    margin: -8px -8px 8px;
    padding: 10px 8px;
  }

  .source-debug-runbar {
    grid-template-columns: 1fr 1fr;
  }

  .source-debug-runbar :deep(.el-input) {
    grid-column: 1 / -1;
  }

  .source-debug-basic-grid,
  .source-debug-field-list {
    grid-template-columns: 1fr;
  }

  .source-debug-basic-grid .wide,
  .source-debug-field-list label:has(textarea) {
    grid-column: auto;
  }

  .source-debug-console article {
    grid-template-columns: 24px 52px 52px minmax(0, 1fr);
  }
}
</style>
