<template>
  <section class="app-page sources-page">
    <header class="sources-head">
      <div>
        <h1 class="app-page-title">书源管理</h1>
      </div>
      <div class="head-actions">
        <el-button type="primary" :icon="Plus" @click="openEditor()">新增</el-button>
        <el-button :icon="Download" @click="exportSources">
          {{ selection.length ? `导出选中 ${selection.length}` : '导出' }}
        </el-button>
        <el-upload ref="sourceUploadRef" :show-file-list="false" :auto-upload="false" accept=".json" @change="importFile">
          <el-button :icon="Upload">导入</el-button>
        </el-upload>
        <el-button :icon="Link" @click="showRemote = true">远程书源</el-button>
        <el-button plain :disabled="!sources.length" :loading="defaultSaving" @click="setCurrentAsDefault">设为默认</el-button>
        <el-button plain :disabled="!defaultSource.configured" :loading="defaultRestoring" @click="restoreDefaults">
          恢复默认{{ defaultSource.count ? ` ${defaultSource.count}` : '' }}
        </el-button>
        <el-button type="danger" plain :disabled="!sources.length" @click="clearAllSources">清空</el-button>
      </div>
    </header>

    <section class="source-toolbar app-panel">
      <el-input v-model="keyword" placeholder="搜索书源名称、地址或分组" clearable>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="selectedGroup" placeholder="全部分组" clearable>
        <el-option v-for="group in sourceGroupOptions" :key="group.value" :label="group.label" :value="group.value" />
      </el-select>
      <div class="source-check-config">
        <span>搜索词</span>
        <el-input v-model="checkConfig.keyword" placeholder="检测关键词" />
        <span>超时(ms)</span>
        <el-input-number v-model="checkConfig.timeout" :min="1000" :max="15000" :step="500" controls-position="right" />
        <span>并发数</span>
        <el-input-number v-model="checkConfig.concurrent" :min="3" :max="15" :step="1" controls-position="right" />
      </div>
      <el-button class="source-batch-command" :disabled="!selection.length" @click="batchUpdateSources('enable')">启用选中</el-button>
      <el-button class="source-batch-command" :disabled="!selection.length" @click="batchUpdateSources('disable')">停用选中</el-button>
      <el-button class="source-batch-command" :disabled="!selection.length" @click="setSelectedSourceGroup">设置分组</el-button>
      <el-button class="source-batch-command" type="danger" plain :disabled="!selection.length" @click="batchUpdateSources('delete')">删除选中</el-button>
      <el-button class="source-batch-command" :icon="CircleCheck" :loading="checking" @click="checkInvalidSources">失效检测</el-button>
      <el-button class="source-batch-command" type="warning" plain :disabled="!failedHealthSourceIds.length" @click="disableFailedSources">
        停用失败 {{ failedHealthSourceIds.length || '' }}
      </el-button>
      <el-checkbox v-model="failedOnly" :disabled="!healthSummary.total">只看失败</el-checkbox>
      <span v-if="healthSummary.total" class="health-summary">
        已检 {{ healthSummary.total }} · 可用 {{ healthSummary.ok }} · 失败 {{ healthSummary.failed }}
      </span>
    </section>

    <div v-if="sourceShowGroups.length" class="source-group-wrapper">
      <el-tag
        v-for="group in sourceShowGroups"
        :key="group.value"
        type="info"
        class="source-group-btn"
        :effect="isSourceGroupChipSelected(group.value) ? 'dark' : 'light'"
        :class="{ selected: isSourceGroupChipSelected(group.value) }"
        @click="setSourceGroupChip(group.value)"
      >
        {{ group.label }}
      </el-tag>
    </div>

    <el-table :data="pagedSources" stripe class="source-table desktop-source-table" @selection-change="selection = $event">
      <el-table-column type="selection" width="42" :selectable="isSourceSelectable" />
      <el-table-column prop="name" label="名称" min-width="150" show-overflow-tooltip />
      <el-table-column prop="group" label="分组" width="120">
        <template #default="{ row }">{{ row.group || '默认分组' }}</template>
      </el-table-column>
      <el-table-column prop="baseUrl" label="地址" min-width="220" show-overflow-tooltip />
      <el-table-column prop="charset" label="编码" width="90" />
      <el-table-column label="书架书籍" width="100">
        <template #default="{ row }">
          <span :class="row.usedBookCount ? 'source-used' : 'muted'">{{ row.usedBookCount || 0 }}</span>
        </template>
      </el-table-column>
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
          <el-button size="small" text type="danger" :disabled="!isSourceSelectable(row)" @click="deleteSource(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="pagedSources.length" class="mobile-source-list">
      <div class="mobile-source-select-actions app-panel">
        <span>已选 {{ selection.length }} 个</span>
        <div>
          <el-button size="small" text @click="selectShownSources">全选当前</el-button>
          <el-button size="small" text @click="selection = []">清空</el-button>
        </div>
      </div>
      <article
        v-for="source in pagedSources"
        :key="source.id"
        class="mobile-source-card app-panel"
        :class="{ selected: isSourceSelected(source) }"
      >
        <header>
          <el-checkbox :model-value="isSourceSelected(source)" :disabled="!isSourceSelectable(source)" @change="value => toggleMobileSourceSelection(source, value)" />
          <div>
            <strong>{{ source.name }}</strong>
            <span>{{ source.group || '默认分组' }}</span>
          </div>
          <el-switch :model-value="source.enabled" size="small" @change="value => toggleSource(source, value)" />
        </header>
        <p>{{ source.baseUrl || source.searchUrl || '未设置地址' }}</p>
        <div class="mobile-source-meta">
          <el-tag size="small" effect="plain">{{ source.charset || 'utf-8' }}</el-tag>
          <el-tag v-if="source.usedBookCount" size="small" type="warning" effect="plain">书架 {{ source.usedBookCount }} 本</el-tag>
          <el-tag v-if="health[source.id]" size="small" :type="health[source.id].ok ? 'success' : 'danger'" effect="plain">
            {{ health[source.id].ok ? '可用' : health[source.id].message }}
          </el-tag>
          <el-tag v-else size="small" effect="plain">未检测</el-tag>
        </div>
        <footer>
          <el-button size="small" text type="primary" @click="openDebug(source)">调试</el-button>
          <el-button size="small" text @click="openEditor(source)">编辑</el-button>
          <el-button size="small" text type="danger" :disabled="!isSourceSelectable(source)" @click="deleteSource(source.id)">删除</el-button>
        </footer>
      </article>
    </div>

    <div v-if="shownSources.length" class="source-pagination">
      <el-pagination
        v-model:current-page="sourcePage"
        v-model:page-size="sourcePageSize"
        :page-sizes="sourcePageSizes"
        layout="total, sizes, prev, pager, next"
        :pager-count="isMobileDialog ? 5 : 7"
        :total="shownSources.length"
      />
    </div>

    <el-empty v-if="!sources.length" description="还没有书源，导入或新增书源开始使用" />

    <div v-if="sources.length" class="source-batch-footer app-panel">
      <span class="check-tip">已选择 {{ selection.length }} 个</span>
      <el-button type="primary" plain :disabled="!selection.length" @click="batchUpdateSources('delete')">批量删除</el-button>
      <el-button :icon="CircleCheck" :loading="checking" @click="checkInvalidSources">
        {{ checking ? '正在' : '' }}检测书源
      </el-button>
      <el-dropdown @command="handleSourceBatchMoreCommand">
        <el-button :disabled="!selection.length && !failedHealthSourceIds.length">
          更多批量操作<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="enable" :disabled="!selection.length">启用选中</el-dropdown-item>
            <el-dropdown-item command="disable" :disabled="!selection.length">停用选中</el-dropdown-item>
            <el-dropdown-item command="group" :disabled="!selection.length">设置分组</el-dropdown-item>
            <el-dropdown-item command="export" :disabled="!selection.length">导出选中 {{ selection.length || '' }}</el-dropdown-item>
            <el-dropdown-item command="disable-failed" :disabled="!failedHealthSourceIds.length">停用失败 {{ failedHealthSourceIds.length || '' }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-button @click="clearSourceBatchState">取消</el-button>
    </div>

    <el-dialog v-model="showRemote" title="远程书源" width="460px" :fullscreen="isMobileDialog">
      <el-input v-model="remoteURL" placeholder="输入书源 JSON 订阅地址" />
      <template #footer>
        <el-button @click="showRemote = false">取消</el-button>
        <el-button type="primary" :loading="remoteLoading" @click="importRemote">导入</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showImportPreview" title="导入书源" width="720px" :fullscreen="isMobileDialog">
      <div class="source-import-preview">
        <el-checkbox
          v-model="importCheckAll"
          :indeterminate="importCheckIndeterminate"
          border
          @change="toggleImportCheckAll"
        >
          全选
        </el-checkbox>
        <span class="check-tip">已选择 {{ checkedImportSourceIndexes.length }} / {{ importPreviewSources.length }} 个</span>
        <el-checkbox-group v-model="checkedImportSourceIndexes" class="source-import-list" @change="handleImportSelectionChange">
          <el-checkbox
            v-for="(source, index) in importPreviewSources"
            :key="index"
            :label="index"
            class="source-import-item"
          >
            <strong>{{ importSourceName(source) || '未命名书源' }}</strong>
            <span>{{ importSourceURL(source) || '未设置地址' }}</span>
            <em v-if="importSourceTags(source)">{{ importSourceTags(source) }}</em>
          </el-checkbox>
        </el-checkbox-group>
      </div>
      <template #footer>
        <el-button @click="closeImportPreview">取消</el-button>
        <el-button type="primary" :loading="importPreviewSaving" :disabled="!checkedImportSourceIndexes.length" @click="saveSelectedImportSources">
          确定导入
        </el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="showEditor" :title="editingSourceId ? '编辑书源' : '新增书源'" :direction="drawerDirection" :size="editorDrawerSize">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="sourceForm.name" /></el-form-item>
        <el-form-item label="分组"><el-input v-model="sourceForm.group" placeholder="默认分组" /></el-form-item>
        <el-form-item label="基础地址"><el-input v-model="sourceForm.baseUrl" /></el-form-item>
        <el-form-item label="搜索地址"><el-input v-model="sourceForm.searchUrl" /></el-form-item>
        <el-form-item label="详情页 URL 正则"><el-input v-model="sourceForm.bookUrlPattern" /></el-form-item>
        <el-form-item label="书源类型">
          <el-select v-model="sourceForm.bookSourceType">
            <el-option label="文本" :value="0" />
            <el-option label="音频" :value="1" />
          </el-select>
        </el-form-item>
        <el-form-item label="注释"><el-input v-model="sourceForm.bookSourceComment" type="textarea" :rows="2" /></el-form-item>
        <el-form-item label="探索地址">
          <el-input v-model="ruleForm.exploreUrl" placeholder="用于书海/发现页的 exploreUrl，可包含 {page}" />
        </el-form-item>
        <el-form-item label="编码">
          <el-select v-model="sourceForm.charset">
            <el-option label="UTF-8" value="utf-8" />
            <el-option label="GBK" value="gbk" />
          </el-select>
        </el-form-item>
        <el-form-item label="并发率">
          <el-input v-model="sourceForm.concurrentRate" placeholder="例如 1000 或 3/1000" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="sourceForm.customOrder" :step="1" />
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
            <el-collapse-item title="探索结果" name="explore">
              <div class="rule-grid">
                <el-input v-model="ruleForm.exploreBookListRule" placeholder="探索列表 exploreBookListRule（留空复用搜索规则）" />
                <el-input v-model="ruleForm.exploreBookNameRule" placeholder="书名 exploreBookNameRule" />
                <el-input v-model="ruleForm.exploreBookAuthorRule" placeholder="作者 exploreBookAuthorRule" />
                <el-input v-model="ruleForm.exploreBookCoverRule" placeholder="封面 exploreBookCoverRule" />
                <el-input v-model="ruleForm.exploreBookIntroRule" placeholder="简介 exploreBookIntroRule" />
                <el-input v-model="ruleForm.exploreLatestChapterRule" placeholder="最新章节 exploreLatestChapterRule" />
                <el-input v-model="ruleForm.exploreBookUrlRule" placeholder="详情地址 exploreBookUrlRule" />
                <el-input v-model="ruleForm.explorePaginationRule" placeholder="下一页 explorePaginationRule（可选）" />
              </div>
            </el-collapse-item>
            <el-collapse-item title="目录" name="toc">
              <div class="rule-grid">
                <el-input v-model="ruleForm.bookInfoNameRule" placeholder="详情书名 bookInfoNameRule" />
                <el-input v-model="ruleForm.bookInfoAuthorRule" placeholder="详情作者 bookInfoAuthorRule" />
                <el-input v-model="ruleForm.bookInfoCoverRule" placeholder="详情封面 bookInfoCoverRule" />
                <el-input v-model="ruleForm.bookInfoIntroRule" placeholder="详情简介 bookInfoIntroRule" />
                <el-input v-model="ruleForm.bookInfoKindRule" placeholder="详情分类 bookInfoKindRule" />
                <el-input v-model="ruleForm.bookInfoLatestChapterRule" placeholder="详情最新章节 bookInfoLatestChapterRule" />
                <el-input v-model="ruleForm.bookInfoUpdateTimeRule" placeholder="详情更新时间 bookInfoUpdateTimeRule" />
                <el-input v-model="ruleForm.bookInfoWordCountRule" placeholder="详情字数 bookInfoWordCountRule" />
                <el-input v-model="ruleForm.tocUrlRule" placeholder="目录地址 tocUrlRule" />
                <el-input v-model="ruleForm.chapterListRule" placeholder="章节列表 chapterListRule" />
                <el-input v-model="ruleForm.chapterNameRule" placeholder="章节名 chapterNameRule" />
                <el-input v-model="ruleForm.chapterUrlRule" placeholder="章节地址 chapterUrlRule" />
                <el-input v-model="ruleForm.nextTocUrlRule" placeholder="目录下一页 nextTocUrlRule（可选）" />
              </div>
            </el-collapse-item>
            <el-collapse-item title="正文" name="content">
              <div class="rule-grid">
                <el-input v-model="ruleForm.contentUrlRule" placeholder="正文地址 contentUrlRule" />
                <el-input v-model="ruleForm.contentRule" placeholder="正文内容 contentRule" />
                <el-input v-model="ruleForm.nextContentUrlRule" placeholder="正文下一页 nextContentUrlRule（可选）" />
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
        <el-form-item>
          <el-switch v-model="sourceForm.enabledExplore" active-text="启用发现" inactive-text="关闭发现" />
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
import { ArrowDown, CircleCheck, Download, Link, Plus, Search, Upload } from '@element-plus/icons-vue'
import {
  batchSources,
  batchTestSources,
  clearSources,
  createSource,
  defaultSourceStatus,
  deleteSource as deleteSourceApi,
  exportSources as exportSourcesApi,
  importSources,
  listSources,
  previewRemoteSource,
  restoreDefaultSources,
  saveDefaultSources,
  testSourceChapter,
  testSourceContent,
  testSourceSearch,
  updateSource,
} from '../api/sources'
import { useReaderStore } from '../stores/reader'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'

const route = useRoute()
const reader = useReaderStore()
const sources = ref([])
const keyword = ref('')
const selectedGroup = ref('')
const selection = ref([])
const health = ref({})
const checking = ref(false)
const failedOnly = ref(false)
const checkConfig = reactive({ keyword: '斗罗大陆', timeout: 5000, concurrent: 5 })
const sourcePage = ref(1)
const sourcePageSize = ref(25)
const sourcePageSizes = [25, 50, 100, 200, 300, 400]
const defaultSource = reactive({ configured: false, count: 0 })
const defaultSaving = ref(false)
const defaultRestoring = ref(false)

const showRemote = ref(false)
const remoteURL = ref('')
const remoteLoading = ref(false)
const sourceUploadRef = ref(null)
const showImportPreview = ref(false)
const importPreviewSources = ref([])
const checkedImportSourceIndexes = ref([])
const importCheckAll = ref(false)
const importCheckIndeterminate = ref(false)
const importPreviewSaving = ref(false)

const showEditor = ref(false)
const editingSourceId = ref(null)
const savingSource = ref(false)
const sourceForm = reactive({ name: '', group: '', baseUrl: '', searchUrl: '', bookUrlPattern: '', bookSourceType: 0, bookSourceComment: '', charset: 'utf-8', concurrentRate: '', customOrder: 0, rules: '', enabled: true, enabledExplore: true })
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
  'exploreBookListRule',
  'exploreBookNameRule',
  'exploreBookAuthorRule',
  'exploreBookCoverRule',
  'exploreBookIntroRule',
  'exploreLatestChapterRule',
  'exploreBookUrlRule',
  'explorePaginationRule',
  'bookInfoNameRule',
  'bookInfoAuthorRule',
  'bookInfoCoverRule',
  'bookInfoIntroRule',
  'bookInfoKindRule',
  'bookInfoLatestChapterRule',
  'bookInfoUpdateTimeRule',
  'bookInfoWordCountRule',
  'tocUrlRule',
  'chapterListRule',
  'chapterNameRule',
  'chapterUrlRule',
  'nextTocUrlRule',
  'contentUrlRule',
  'contentRule',
  'nextContentUrlRule',
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
const windowWidth = ref(currentViewportWidth())
let sourceReloadTimer

const sourceGroupOptions = computed(() => buildSourceGroupOptions(sources.value))
const sourceShowGroups = computed(() => {
  const groups = []
  const seen = new Set()
  let ungroupedCount = 0
  for (const source of sources.value || []) {
    const rawGroup = String(source?.group || '').trim()
    if (!rawGroup) {
      ungroupedCount += 1
      continue
    }
    if (seen.has(rawGroup)) continue
    seen.add(rawGroup)
    groups.push({ value: rawGroup, label: rawGroup })
  }
  groups.sort((a, b) => a.label.localeCompare(b.label))
  if (ungroupedCount > 0) {
    groups.push({ value: '默认分组', label: '未分组' })
  }
  return groups
})
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
const pagedSources = computed(() => {
  const start = (sourcePage.value - 1) * sourcePageSize.value
  return shownSources.value.slice(start, start + sourcePageSize.value)
})
const debugResultText = computed(() => debugResult.value ? JSON.stringify(debugResult.value, null, 2) : '')
const debugSearchRows = computed(() => Array.isArray(debugResult.value?.results) ? debugResult.value.results : [])
const debugChapterRows = computed(() => Array.isArray(debugResult.value?.chapters) ? debugResult.value.chapters : [])
const isMobileDialog = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const drawerDirection = computed(() => isMobileDialog.value ? 'btt' : 'rtl')
const editorDrawerSize = computed(() => isMobileDialog.value ? '88%' : '520px')

onMounted(async () => {
  window.addEventListener('resize', handleResize)
  window.addEventListener('openreader:sources-update', handleSourcesUpdate)
  const [sourcesResult, defaultResult] = await Promise.allSettled([
    loadSources(),
    loadDefaultSourceStatus(),
  ])
  if (sourcesResult.status === 'rejected') {
    ElMessage.warning(readError(sourcesResult.reason, '加载书源失败'))
  }
  if (defaultResult.status === 'rejected') {
    ElMessage.warning(readError(defaultResult.reason, '默认书源状态加载失败'))
  }
  applyRouteAction()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('openreader:sources-update', handleSourcesUpdate)
  clearSourceReloadTimer()
})

watch(
  () => [route.query.panel, route.query.action],
  () => applyRouteAction(),
)

watch(sourceGroupOptions, (items) => {
  if (selectedGroup.value && !items.some(item => item.value === selectedGroup.value)) {
    selectedGroup.value = ''
  }
})

watch([keyword, selectedGroup, failedOnly, sourcePageSize], () => {
  sourcePage.value = 1
})

watch(
  () => shownSources.value.length,
  () => ensureSourcePageInRange(),
)

async function loadSources() {
  const { data } = await listSources()
  sources.value = data
}

async function loadDefaultSourceStatus() {
  try {
    const { data } = await defaultSourceStatus()
    defaultSource.configured = !!data?.configured
    defaultSource.count = Number(data?.count || 0)
  } catch {
    defaultSource.configured = false
    defaultSource.count = 0
  }
}

function handleSourcesUpdate(event) {
  scheduleSourcesReload(event?.detail)
}

function scheduleSourcesReload(detail = {}) {
  clearSourceReloadTimer()
  sourceReloadTimer = window.setTimeout(async () => {
    sourceReloadTimer = undefined
    try {
      await loadSources()
      if (['save-default', 'restore-default'].includes(detail?.kind)) {
        await loadDefaultSourceStatus()
      }
    } catch {
      // Keep the current list visible; the next explicit refresh or sync event can recover.
    }
  }, 350)
}

function clearSourceReloadTimer() {
  if (!sourceReloadTimer) return
  window.clearTimeout(sourceReloadTimer)
  sourceReloadTimer = undefined
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
  if (route.query.action === 'import') {
    openSourceImportPicker()
  }
  if (route.query.action === 'debug') {
    openFirstDebugSource()
  }
}

function handleResize() {
  windowWidth.value = currentViewportWidth()
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
    bookUrlPattern: source?.bookUrlPattern || '',
    bookSourceType: Number(source?.bookSourceType) || 0,
    bookSourceComment: source?.bookSourceComment || '',
    charset: source?.charset || 'utf-8',
    concurrentRate: source?.concurrentRate || '',
    customOrder: Number(source?.customOrder) || 0,
    rules: source?.rules || '',
    enabled: source?.enabled ?? true,
    enabledExplore: source?.enabledExplore ?? true,
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
  const source = sources.value.find(item => item.id === id)
  if (source && !isSourceSelectable(source)) {
    ElMessage.warning(`该书源仍有 ${source.usedBookCount} 本书在使用，不能删除`)
    return
  }
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
    const skippedUsed = Number(data.skippedUsed || 0)
    ElMessage.success(`已${actionName} ${data.affected || 0} 个书源${skippedUsed ? `，跳过 ${skippedUsed} 个使用中的书源` : ''}`)
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

function handleSourceBatchMoreCommand(command) {
  if (command === 'enable') {
    batchUpdateSources('enable')
  } else if (command === 'disable') {
    batchUpdateSources('disable')
  } else if (command === 'group') {
    setSelectedSourceGroup()
  } else if (command === 'export') {
    exportSources()
  } else if (command === 'disable-failed') {
    disableFailedSources()
  }
}

function clearSourceBatchState() {
  selection.value = []
}

function buildSourceGroupOptions(rows) {
  const counts = new Map()
  for (const source of rows || []) {
    const group = String(source?.group || '默认分组').trim() || '默认分组'
    counts.set(group, (counts.get(group) || 0) + 1)
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, count]) => ({ value, label: `${value} (${count})`, count }))
}

function isSourceGroupChipSelected(group) {
  return selectedGroup.value === group
}

function setSourceGroupChip(group) {
  selectedGroup.value = selectedGroup.value === group ? '' : group
}

function isSourceSelected(source) {
  return selection.value.some(item => item.id === source.id)
}

function isSourceSelectable(source) {
  return Number(source?.usedBookCount || 0) === 0
}

function toggleMobileSourceSelection(source, checked) {
  if (!isSourceSelectable(source)) return
  if (checked) {
    if (!isSourceSelected(source)) selection.value = [...selection.value, source]
    return
  }
  selection.value = selection.value.filter(item => item.id !== source.id)
}

function selectShownSources() {
  selection.value = pagedSources.value.filter(isSourceSelectable)
}

function ensureSourcePageInRange() {
  const maxPage = Math.max(1, Math.ceil(shownSources.value.length / sourcePageSize.value))
  if (sourcePage.value > maxPage) {
    sourcePage.value = maxPage
  }
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

async function setCurrentAsDefault() {
  if (!sources.value.length) return
  defaultSaving.value = true
  try {
    await ElMessageBox.confirm(
      `确定将当前 ${sources.value.length} 个书源保存为默认书源吗？之后“恢复默认”会用它替换当前书源。`,
      '设为默认书源',
      { type: 'warning' },
    )
    const { data } = await saveDefaultSources()
    defaultSource.configured = true
    defaultSource.count = Number(data.count || sources.value.length)
    ElMessage.success(`已保存 ${defaultSource.count} 个默认书源`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '保存默认书源失败'))
  } finally {
    defaultSaving.value = false
  }
}

async function restoreDefaults() {
  if (!defaultSource.configured) return
  defaultRestoring.value = true
  try {
    await ElMessageBox.confirm(
      `确定恢复默认书源吗？当前书源会被默认快照中的 ${defaultSource.count || 0} 个书源替换。`,
      '恢复默认书源',
      { type: 'warning' },
    )
    const { data } = await restoreDefaultSources()
    selection.value = []
    health.value = {}
    failedOnly.value = false
    ElMessage.success(sourceImportMessage(data))
    await Promise.all([loadSources(), loadDefaultSourceStatus()])
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '恢复默认书源失败'))
  } finally {
    defaultRestoring.value = false
  }
}

async function importFile(data) {
  const file = data.raw
  if (!file) return
  try {
    const list = parseImportSourceList(JSON.parse(await file.text()))
    if (!list.length) {
      ElMessage.error('书源文件错误')
      return
    }
    openImportPreview(list)
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  }
}

function openSourceImportPicker() {
  const input = sourceUploadRef.value?.$el?.querySelector?.('input[type="file"]')
  if (input) {
    input.click()
    return
  }
  ElMessage.info('请点击页面右上角“导入”选择书源 JSON 文件')
}

function parseImportSourceList(value) {
  return Array.isArray(value)
    ? value
    : Array.isArray(value?.bookSources)
      ? value.bookSources
      : Array.isArray(value?.sources)
        ? value.sources
        : value?.name
          ? [value]
          : value?.bookSourceName
          ? [value]
          : []
}

async function importRemote() {
  if (!remoteURL.value.trim()) return
  remoteLoading.value = true
  try {
    const url = remoteURL.value.trim()
    const { data: preview } = await previewRemoteSource(url)
    const list = parseImportSourceList(preview.sources || [])
    if (!list.length) {
      ElMessage.error('远程订阅未识别到书源')
      return
    }
    showRemote.value = false
    remoteURL.value = ''
    openImportPreview(list)
  } catch (err) {
    ElMessage.error(readError(err, '远程导入失败'))
  } finally {
    remoteLoading.value = false
  }
}

function openImportPreview(list) {
  importPreviewSources.value = list
  checkedImportSourceIndexes.value = list
    .map((source, index) => importSourceTags(source) ? null : index)
    .filter(index => index !== null)
  updateImportCheckState()
  showImportPreview.value = true
  if (checkedImportSourceIndexes.value.length < list.length) {
    ElMessage.info('部分使用 Javascript 或 WebView 的书源未默认勾选')
  }
}

function closeImportPreview() {
  showImportPreview.value = false
  importPreviewSources.value = []
  checkedImportSourceIndexes.value = []
  updateImportCheckState()
}

function toggleImportCheckAll(checked) {
  checkedImportSourceIndexes.value = checked
    ? importPreviewSources.value.map((source, index) => importSourceTags(source) ? null : index).filter(index => index !== null)
    : []
  updateImportCheckState()
  if (checked && checkedImportSourceIndexes.value.length < importPreviewSources.value.length) {
    ElMessage.info('部分使用 Javascript 或 WebView 的书源未勾选')
  }
}

function handleImportSelectionChange() {
  updateImportCheckState()
}

function updateImportCheckState() {
  const totalSelectable = importPreviewSources.value.filter(source => !importSourceTags(source)).length
  const selected = checkedImportSourceIndexes.value.length
  importCheckAll.value = totalSelectable > 0 && selected === totalSelectable
  importCheckIndeterminate.value = selected > 0 && selected < totalSelectable
}

function importSourceName(source) {
  return source?.name || source?.bookSourceName || ''
}

function importSourceURL(source) {
  return source?.baseUrl || source?.bookSourceUrl || source?.searchUrl || ''
}

function importSourceTags(source) {
  const sourceText = JSON.stringify(source || {})
  const tags = []
  if (sourceText.includes('@js:')) tags.push('@Javascript')
  if (sourceText.includes('webView:')) tags.push('@WebView')
  return tags.join(' ')
}

async function saveSelectedImportSources() {
  if (!checkedImportSourceIndexes.value.length) {
    ElMessage.warning('请选择需要导入的源')
    return
  }
  importPreviewSaving.value = true
  try {
    const selectedSources = checkedImportSourceIndexes.value.map(index => importPreviewSources.value[index])
    const form = new FormData()
    form.append('file', new Blob([JSON.stringify(selectedSources)], { type: 'application/json' }), 'bookSources.json')
    const { data: result } = await importSources(form)
    ElMessage.success(sourceImportMessage(result))
    closeImportPreview()
    await loadSources()
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importPreviewSaving.value = false
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
    const selectedIds = selection.value.map(source => source.id).filter(Boolean)
    const resp = await exportSourcesApi(selectedIds)
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([resp.data], { type: 'application/json' }))
    a.download = selectedIds.length ? 'bookSources-selected.json' : 'bookSources.json'
    a.click()
    URL.revokeObjectURL(a.href)
    ElMessage.success(selectedIds.length ? `已导出 ${selectedIds.length} 个书源` : '已导出全部书源')
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
      keyword: checkConfig.keyword,
      timeout: checkConfig.timeout,
      concurrent: checkConfig.concurrent,
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

function openFirstDebugSource() {
  const source = sources.value.find(item => item.enabled) || sources.value[0]
  if (source) {
    openDebug(source)
    return
  }
  ElMessage.info('请先添加书源后再调试')
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

.source-used {
  color: var(--app-warning, #b7791f);
  font-weight: 600;
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

.source-check-config {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--app-text-muted);
  font-size: 13px;
}

.source-check-config :deep(.el-input) {
  width: 150px;
  min-width: 0;
  flex: none;
}

.source-check-config :deep(.el-input-number) {
  width: 110px;
}

.health-summary {
  color: var(--app-text-muted);
  font-size: 13px;
  white-space: nowrap;
}

.source-table {
  width: 100%;
}

.source-group-wrapper {
  display: flex;
  min-width: 0;
  overflow-x: auto;
  gap: 8px;
  padding: 0 2px 2px;
}

.source-group-btn {
  flex: 0 0 auto;
  cursor: pointer;
  user-select: none;
}

.source-group-btn.selected {
  border-color: var(--app-primary);
  color: var(--app-primary);
}

.source-pagination {
  display: flex;
  justify-content: flex-end;
  min-width: 0;
}

.source-batch-footer {
  display: none;
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

.source-import-preview {
  display: grid;
  min-width: 0;
  gap: 12px;
}

.source-import-list {
  display: grid;
  max-height: min(58vh, 520px);
  min-width: 0;
  overflow: auto;
  gap: 8px;
}

.source-import-item {
  display: block;
  min-width: 0;
  margin-right: 0;
  padding: 10px 12px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.source-import-item :deep(.el-checkbox__label) {
  display: grid;
  min-width: 0;
  gap: 3px;
  line-height: 1.5;
  white-space: normal;
}

.source-import-item strong {
  color: var(--app-text);
}

.source-import-item span,
.source-import-item em {
  overflow-wrap: anywhere;
  color: var(--app-text-muted);
  font-size: 12px;
  font-style: normal;
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

  .source-batch-command {
    display: none;
  }

  .head-actions,
  .source-toolbar :deep(.el-input),
  .source-toolbar :deep(.el-select),
  .source-toolbar :deep(.el-button),
  .debug-row :deep(.el-input),
  .debug-row :deep(.el-button) {
    width: 100%;
  }

  .source-check-config {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
  }

  .source-check-config :deep(.el-input),
  .source-check-config :deep(.el-input-number) {
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

  .source-pagination {
    justify-content: center;
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .source-batch-footer {
    position: sticky;
    z-index: 2;
    bottom: max(10px, env(safe-area-inset-bottom));
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 10px;
    box-shadow: 0 -8px 22px rgba(15, 23, 42, 0.08);
  }

  .source-batch-footer .check-tip {
    grid-column: 1 / -1;
    color: var(--app-text-muted);
    font-size: 13px;
  }

  .source-batch-footer :deep(.el-button),
  .source-batch-footer :deep(.el-dropdown),
  .source-batch-footer :deep(.el-dropdown .el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }
}
</style>
