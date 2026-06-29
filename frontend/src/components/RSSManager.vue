<template>
  <section class="rss-manager">
    <article class="rss-panel">
      <header class="rss-head">
        <div>
          <strong>RSS 源</strong>
          <span>{{ sources.length }} 个订阅</span>
        </div>
        <div class="rss-actions">
          <el-button size="small" @click="rssEditMode = !rssEditMode">{{ rssEditMode ? '取消' : '编辑' }}</el-button>
          <el-button size="small" :loading="importingSources" @click="triggerSourceImport">导入</el-button>
          <el-button size="small" type="primary" @click="openEditor()">新增</el-button>
          <el-button size="small" :loading="sourcesLoading" @click="loadSources">刷新</el-button>
          <input ref="sourceImportInput" class="rss-source-import-input" type="file" accept=".json,application/json" @change="importRSSSources" />
        </div>
      </header>
      <div v-loading="sourcesLoading" class="rss-source-list">
        <div
          v-for="source in sources"
          :key="source.id"
          class="rss-source-card"
          :class="{ active: selectedSourceId === source.id }"
        >
          <button type="button" @click="selectSource(source.id)">
            <span class="rss-source-icon" :class="{ placeholder: !source.icon }">
              <img v-if="source.icon" :src="source.icon" alt="" loading="lazy" @error="source.icon = ''" />
              <span v-else>{{ sourceInitial(source) }}</span>
            </span>
            <strong>{{ source.title || '未命名 RSS' }}</strong>
            <small v-if="source.group">{{ source.group }}</small>
          </button>
          <span class="rss-source-tools">
            <el-tag size="small" :type="source.enabled === false ? 'info' : 'success'" effect="plain">
              {{ source.enabled === false ? '停用' : '启用' }}
            </el-tag>
            <template v-if="rssEditMode">
              <el-button size="small" text :loading="refreshingSourceId === source.id" @click="refreshSource(source)">刷新</el-button>
              <el-button size="small" text @click="openEditor(source)">编辑</el-button>
              <el-button size="small" text type="danger" @click="removeSource(source)">删除</el-button>
            </template>
          </span>
        </div>
        <el-empty v-if="!sourcesLoading && !sources.length" description="暂无 RSS 源" />
      </div>
    </article>

    <article class="rss-panel">
      <header class="rss-head">
        <div>
          <strong>文章</strong>
          <span>{{ articleCountText }}</span>
        </div>
        <div class="rss-actions">
          <el-select
            v-if="selectedSortOptions.length > 1"
            v-model="selectedSortURL"
            size="small"
            class="rss-sort-select"
            @change="refreshSelectedSource"
          >
            <el-option v-for="option in selectedSortOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
          <el-radio-group v-model="articleFilter" size="small" @change="loadArticles">
            <el-radio-button value="all">全部</el-radio-button>
            <el-radio-button value="unread">未读</el-radio-button>
            <el-radio-button value="favorite">收藏</el-radio-button>
          </el-radio-group>
          <el-button size="small" :loading="refreshingSourceId === selectedSourceId" @click="refreshSelectedSource">刷新文章</el-button>
        </div>
      </header>
      <div v-loading="articlesLoading" class="rss-article-list">
        <article v-for="article in articles" :key="article.id" class="rss-article-row" :class="{ read: article.isRead }">
          <button type="button" @click="openArticle(article)">
            <span class="rss-article-info">
              <strong>{{ article.title }}</strong>
              <small>{{ articleDateText(article) }} · {{ article.author || '未知作者' }}</small>
              <span>{{ stripHTML(article.summary || article.content || '无摘要') }}</span>
            </span>
            <span v-if="article.image" class="rss-article-image" @click.stop.prevent="openArticleListImagePreview(article)">
              <img :src="article.image" alt="" loading="lazy" />
            </span>
          </button>
          <span class="rss-article-tools">
            <el-button size="small" text @click="toggleRead(article)">
              {{ article.isRead ? '标未读' : '标已读' }}
            </el-button>
            <el-button
              size="small"
              text
              :type="article.favorite ? 'warning' : 'info'"
              @click="toggleFavorite(article)"
            >
              {{ article.favorite ? '已收藏' : '收藏' }}
            </el-button>
          </span>
        </article>
        <button v-if="articles.length || hasMoreArticles" type="button" class="load-more-rss" :disabled="!hasMoreArticles || articlesLoadingMore" @click="loadMoreArticles">
          {{ hasMoreArticles ? (articlesLoadingMore ? '加载中...' : '加载更多') : '没有更多啦' }}
        </button>
        <el-empty v-if="!articlesLoading && !articles.length" description="暂无 RSS 文章" />
      </div>
    </article>

    <el-dialog v-model="editorVisible" :title="editingSourceId ? '编辑 RSS 源' : '新增 RSS 源'" width="520px" :fullscreen="isMobile">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="draft.title" /></el-form-item>
        <el-form-item label="订阅地址"><el-input v-model="draft.url" /></el-form-item>
        <el-form-item label="图标地址"><el-input v-model="draft.icon" /></el-form-item>
        <el-form-item label="分组"><el-input v-model="draft.group" /></el-form-item>
        <el-form-item label="源注释"><el-input v-model="draft.comment" type="textarea" :rows="2" /></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="draft.customOrder" :min="0" :step="1" controls-position="right" /></el-form-item>
        <el-form-item><el-switch v-model="draft.enabled" active-text="启用" inactive-text="停用" /></el-form-item>
        <el-collapse class="rss-rule-collapse">
          <el-collapse-item title="高级规则" name="advanced">
            <div class="rss-rule-grid">
              <el-form-item label="单页地址"><el-switch v-model="draft.singleUrl" active-text="单页" inactive-text="分页" /></el-form-item>
              <el-form-item label="文章样式"><el-input-number v-model="draft.articleStyle" :min="0" :step="1" controls-position="right" /></el-form-item>
              <el-form-item label="启用 JS（兼容字段）"><el-switch v-model="draft.enableJs" active-text="启用" inactive-text="停用" /></el-form-item>
              <el-form-item label="使用基础地址（兼容字段）"><el-switch v-model="draft.loadWithBaseUrl" active-text="启用" inactive-text="停用" /></el-form-item>
              <el-form-item label="请求头 header"><el-input v-model="draft.header" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" placeholder='JSON 或每行 Header: Value' /></el-form-item>
              <el-form-item label="登录地址（仅保存）"><el-input v-model="draft.loginUrl" /></el-form-item>
              <el-form-item label="登录检测 JS（仅保存）"><el-input v-model="draft.loginCheckJs" type="textarea" :rows="2" /></el-form-item>
              <el-form-item label="并发率"><el-input v-model="draft.concurrentRate" placeholder="例如 1000 或 3/1000" /></el-form-item>
              <el-form-item label="排序地址 sortUrl"><el-input v-model="draft.sortUrl" /></el-form-item>
              <el-form-item label="文章列表 ruleArticles"><el-input v-model="draft.ruleArticles" /></el-form-item>
              <el-form-item label="标题 ruleTitle"><el-input v-model="draft.ruleTitle" /></el-form-item>
              <el-form-item label="发布时间 rulePubDate"><el-input v-model="draft.rulePubDate" /></el-form-item>
              <el-form-item label="摘要 ruleDescription"><el-input v-model="draft.ruleDescription" /></el-form-item>
              <el-form-item label="图片 ruleImage"><el-input v-model="draft.ruleImage" /></el-form-item>
              <el-form-item label="链接 ruleLink"><el-input v-model="draft.ruleLink" /></el-form-item>
              <el-form-item label="正文 ruleContent"><el-input v-model="draft.ruleContent" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" /></el-form-item>
              <el-form-item label="显示样式（仅保存）"><el-input v-model="draft.style" type="textarea" :rows="2" /></el-form-item>
            </div>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingSource" @click="saveSource">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="articleDialogVisible" :title="selectedArticle?.title || 'RSS 文章'" width="720px" class="rss-reader-dialog" :fullscreen="isMobile">
      <article v-if="selectedArticle" v-loading="articleContentLoading" class="rss-reader">
        <h2>{{ selectedArticle.title }}</h2>
        <small>{{ articleDateText(selectedArticle) }} · {{ selectedArticle.author || '未知作者' }}</small>
        <div class="rss-reader-content" v-html="articleBodyHTML(selectedArticle)" @click="handleArticleContentClick" />
      </article>
      <template #footer>
        <el-button @click="articleDialogVisible = false">关闭</el-button>
        <el-button v-if="selectedArticle" @click="toggleRead(selectedArticle)">
          {{ selectedArticle.isRead ? '标为未读' : '标为已读' }}
        </el-button>
        <el-button v-if="selectedArticle" :type="selectedArticle.favorite ? 'warning' : 'default'" @click="toggleFavorite(selectedArticle)">
          {{ selectedArticle.favorite ? '取消收藏' : '收藏' }}
        </el-button>
        <el-button v-if="selectedArticle?.link" type="primary" @click="openExternal(selectedArticle.link)">打开原文</el-button>
      </template>
    </el-dialog>

    <el-image-viewer
      v-if="articleImagePreviewVisible"
      :url-list="articlePreviewImages"
      :initial-index="articlePreviewIndex"
      @close="articleImagePreviewVisible = false"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createRSSSource, deleteRSSSource, getRSSArticleContent, listRSSArticles, listRSSSources, refreshRSSSource, updateRSSArticle, updateRSSSource } from '../api/rss'
import { cacheFirstRequest, networkFirstRequest, removeBrowserCache } from '../utils/browserCache'
import { currentUserScope } from '../utils/authScope'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const ARTICLE_LIMIT = 50

const sources = ref([])
const articles = ref([])
const selectedSourceId = ref('')
const selectedSortURL = ref('')
const sourcesLoading = ref(false)
const articlesLoading = ref(false)
const articlesLoadingMore = ref(false)
const refreshingSourceId = ref(null)
const rssEditMode = ref(false)
const editorVisible = ref(false)
const savingSource = ref(false)
const importingSources = ref(false)
const editingSourceId = ref(null)
const draft = ref({ title: '', url: '', icon: '', group: '', customOrder: 0, enabled: true })
const articleDialogVisible = ref(false)
const articleContentLoading = ref(false)
const selectedArticle = ref(null)
const articleFilter = ref('all')
const articlePage = ref(1)
const hasMoreArticles = ref(false)
const sourceImportInput = ref(null)
const articleImagePreviewVisible = ref(false)
const articlePreviewImages = ref([])
const articlePreviewIndex = ref(0)
let rssReloadTimer

const RSS_ADVANCED_FIELDS = [
  'singleUrl',
  'articleStyle',
  'comment',
  'concurrentRate',
  'header',
  'loginUrl',
  'loginCheckJs',
  'sortUrl',
  'ruleArticles',
  'ruleNextPage',
  'ruleTitle',
  'rulePubDate',
  'ruleDescription',
  'ruleImage',
  'ruleLink',
  'ruleContent',
  'style',
  'enableJs',
  'loadWithBaseUrl',
]

const articleCountText = computed(() => `${articles.value.length} 篇${hasMoreArticles.value ? '+' : ''}`)
const rssArticleImageList = computed(() => articles.value.map(article => article.image).filter(Boolean))
const selectedSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value) || null)
const selectedSortOptions = computed(() => rssSortOptions(selectedSource.value))
const selectedSortOption = computed(() => selectedSortOptions.value.find(option => option.value === selectedSortURL.value) || null)

onMounted(async () => {
  window.addEventListener('openreader:rss-updated', handleRSSUpdated)
  await loadSources()
  await loadArticles()
})

onBeforeUnmount(() => {
  window.removeEventListener('openreader:rss-updated', handleRSSUpdated)
  clearRSSReloadTimer()
})

async function loadSources() {
  sourcesLoading.value = true
  try {
    const response = await cacheFirstRequest(
      () => listRSSSources(),
      rssSourcesCacheKey(),
      { validate: data => Array.isArray(data) },
    )
    applyRSSSources(response.data)
    if (response.fromCache) refreshRSSSourcesCache().catch(() => {})
  } catch (err) {
    ElMessage.error(readError(err, '加载 RSS 源失败'))
  } finally {
    sourcesLoading.value = false
  }
}

async function refreshRSSSourcesCache() {
  const response = await networkFirstRequest(
    () => listRSSSources(),
    rssSourcesCacheKey(),
    { validate: data => Array.isArray(data) },
  )
  applyRSSSources(response.data)
}

function applyRSSSources(data) {
  sources.value = Array.isArray(data) ? data : []
  if (!sources.value.length) rssEditMode.value = false
  if (!selectedSourceId.value && sources.value.length) selectedSourceId.value = sources.value[0].id
  if (selectedSourceId.value && !sources.value.some(source => source.id === selectedSourceId.value)) {
    selectedSourceId.value = sources.value[0]?.id || ''
  }
  syncSelectedSortURL()
}

function rssSourcesCacheKey() {
  return `rssSources@${currentUserScope()}`
}

async function invalidateRSSSourcesCache() {
  await removeBrowserCache(rssSourcesCacheKey())
}

function handleRSSUpdated(event) {
  const detail = event?.detail || {}
  const article = detail.payload?.article
  if (article?.id && selectedArticle.value?.id === article.id) {
    selectedArticle.value = { ...selectedArticle.value, ...article }
  }
  scheduleRSSReload(detail)
}

function scheduleRSSReload(detail = {}) {
  clearRSSReloadTimer()
  rssReloadTimer = window.setTimeout(async () => {
    rssReloadTimer = undefined
    try {
      if (detail.sources) {
        await invalidateRSSSourcesCache()
        await loadSources()
      }
      if (detail.articles) await loadArticles()
    } catch {
      // Keep the visible RSS state; the next manual refresh or sync event can recover.
    }
  }, 250)
}

function clearRSSReloadTimer() {
  if (!rssReloadTimer) return
  window.clearTimeout(rssReloadTimer)
  rssReloadTimer = undefined
}

async function selectSource(sourceId) {
  selectedSourceId.value = sourceId
  syncSelectedSortURL(true)
  await loadArticles()
}

async function loadArticles() {
  articlesLoading.value = true
  articlePage.value = 1
  try {
    const { data } = await listRSSArticles(articleParams(articlePage.value))
    const result = normalizeArticleResult(data, articlePage.value)
    articles.value = result.items
    hasMoreArticles.value = result.hasMore
  } catch (err) {
    ElMessage.error(readError(err, '加载 RSS 文章失败'))
  } finally {
    articlesLoading.value = false
  }
}

async function loadMoreArticles() {
  if (!hasMoreArticles.value || articlesLoadingMore.value) return
  articlesLoadingMore.value = true
  try {
    const nextPage = articlePage.value + 1
    const { data } = await listRSSArticles(articleParams(nextPage))
    const result = normalizeArticleResult(data, nextPage)
    const known = new Set(articles.value.map(article => article.id))
    const nextItems = result.items.filter(article => !known.has(article.id))
    articles.value = [...articles.value, ...nextItems]
    articlePage.value = result.page || nextPage
    hasMoreArticles.value = result.hasMore && nextItems.length > 0
  } catch (err) {
    ElMessage.error(readError(err, '加载更多 RSS 文章失败'))
  } finally {
    articlesLoadingMore.value = false
  }
}

function articleParams(page) {
  const params = { page, limit: ARTICLE_LIMIT }
  if (selectedSourceId.value) params.sourceId = selectedSourceId.value
  if (selectedSortOptions.value.length > 1 && selectedSortOption.value?.label) params.sort = selectedSortOption.value.label
  if (articleFilter.value === 'unread') params.unread = true
  if (articleFilter.value === 'favorite') params.favorite = true
  return params
}

function normalizeArticleResult(data, fallbackPage) {
  if (Array.isArray(data)) return { items: data, page: fallbackPage, hasMore: false }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    page: Number(data?.page || fallbackPage),
    hasMore: !!data?.hasMore,
  }
}

function openEditor(source = null) {
  editingSourceId.value = source?.id || null
  draft.value = {
    title: source?.title || '',
    url: source?.url || '',
    icon: source?.icon || '',
    group: source?.group || '',
    comment: source?.comment || '',
    customOrder: Number(source?.customOrder || 0),
    enabled: source?.enabled ?? true,
    ...pickRSSAdvancedFields(source),
  }
  editorVisible.value = true
}

async function saveSource() {
  if (!draft.value.url.trim()) {
    ElMessage.warning('RSS 地址不能为空')
    return
  }
  savingSource.value = true
  try {
    const payload = {
      ...draft.value,
      title: draft.value.title.trim(),
      url: draft.value.url.trim(),
      icon: draft.value.icon.trim(),
      group: draft.value.group.trim(),
      customOrder: Number(draft.value.customOrder || 0),
      ...pickRSSAdvancedFields(draft.value),
    }
    if (editingSourceId.value) {
      await updateRSSSource(editingSourceId.value, payload)
      ElMessage.success('RSS 源已更新')
    } else {
      await createRSSSource(payload)
      ElMessage.success('RSS 源已创建')
    }
    editorVisible.value = false
    await invalidateRSSSourcesCache()
    await loadSources()
    await loadArticles()
  } catch (err) {
    ElMessage.error(readError(err, '保存 RSS 源失败'))
  } finally {
    savingSource.value = false
  }
}

function triggerSourceImport() {
  sourceImportInput.value?.click()
}

async function importRSSSources(event) {
  const file = event?.target?.files?.[0]
  if (event?.target) event.target.value = ''
  if (!file) return
  importingSources.value = true
  try {
    const text = await file.text()
    const parsed = JSON.parse(text)
    const imported = normalizeRSSSourceImport(parsed)
    if (!imported.length) {
      ElMessage.warning('没有找到可导入的 RSS 源')
      return
    }
    const existingURLs = new Set(sources.value.map(source => normalizeURL(source.url)).filter(Boolean))
    const nextSources = imported.filter(source => !existingURLs.has(normalizeURL(source.url)))
    const skipped = imported.length - nextSources.length
    if (!nextSources.length) {
      ElMessage.warning('导入文件中的 RSS 源已存在')
      return
    }
    await ElMessageBox.confirm(
      `将导入 ${nextSources.length} 个 RSS 源${skipped ? `，跳过 ${skipped} 个已存在源` : ''}。`,
      '导入 RSS 源',
      { type: 'info' },
    )
    for (const source of nextSources) {
      await createRSSSource(source)
    }
    ElMessage.success(`已导入 ${nextSources.length} 个 RSS 源`)
    await invalidateRSSSourcesCache()
    await loadSources()
    await loadArticles()
    window.dispatchEvent(new CustomEvent('openreader:rss-updated', { detail: { sources: true, articles: true } }))
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '导入 RSS 源失败'))
  } finally {
    importingSources.value = false
  }
}

function normalizeRSSSourceImport(payload) {
  const list = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.sources)
      ? payload.sources
      : Array.isArray(payload?.rssSources)
        ? payload.rssSources
        : Array.isArray(payload?.rssSourceList)
          ? payload.rssSourceList
          : payload && typeof payload === 'object'
            ? [payload]
            : []
  return list
    .map((source, index) => {
      const title = String(source.title || source.sourceName || source.name || `RSS ${index + 1}`).trim()
      const url = String(source.url || source.sourceUrl || source.feedUrl || source.link || '').trim()
      const icon = String(source.icon || source.sourceIcon || '').trim()
      const group = String(source.group || source.sourceGroup || '').trim()
      const order = Number(source.customOrder || source.order || 0)
      const enabled = source.enabled ?? source.isEnabled
      return {
        title,
        url,
        icon,
        group,
        customOrder: Number.isFinite(order) ? order : 0,
        enabled: enabled !== false,
        ...pickRSSAdvancedFields(source),
      }
    })
    .filter(source => source.title && source.url)
}

function pickRSSAdvancedFields(source = {}) {
  const picked = {}
  for (const field of RSS_ADVANCED_FIELDS) {
    if (Object.prototype.hasOwnProperty.call(source, field)) picked[field] = source[field]
  }
  if (!Object.prototype.hasOwnProperty.call(picked, 'comment') && source.sourceComment !== undefined) {
    picked.comment = source.sourceComment
  }
  if (!Object.prototype.hasOwnProperty.call(picked, 'header') && source.headerMap !== undefined) {
    picked.header = typeof source.headerMap === 'string' ? source.headerMap : JSON.stringify(source.headerMap)
  }
  if (!Object.prototype.hasOwnProperty.call(picked, 'singleUrl')) picked.singleUrl = true
  if (!Object.prototype.hasOwnProperty.call(picked, 'articleStyle')) picked.articleStyle = 0
  if (!Object.prototype.hasOwnProperty.call(picked, 'enableJs')) picked.enableJs = true
  if (!Object.prototype.hasOwnProperty.call(picked, 'loadWithBaseUrl')) picked.loadWithBaseUrl = true
  return picked
}

async function refreshSource(source) {
  refreshingSourceId.value = source.id
  try {
    const params = source.id === selectedSourceId.value && selectedSortURL.value
      ? { sortUrl: selectedSortURL.value, sortName: selectedSortOption.value?.label || '' }
      : {}
    const { data } = await refreshRSSSource(source.id, params)
    ElMessage.success(`已同步 ${data.imported || 0}/${data.total || 0} 篇文章`)
    await loadArticles()
  } catch (err) {
    ElMessage.error(readError(err, '刷新 RSS 源失败'))
  } finally {
    refreshingSourceId.value = null
  }
}

async function refreshSelectedSource() {
  if (!selectedSource.value) {
    await loadArticles()
    return
  }
  await refreshSource(selectedSource.value)
}

async function removeSource(source) {
  try {
    await ElMessageBox.confirm(`确定删除 RSS 源“${source.title}”吗？文章缓存也会删除。`, '删除 RSS 源', { type: 'warning' })
    await deleteRSSSource(source.id)
    await invalidateRSSSourcesCache()
    sources.value = sources.value.filter(item => item.id !== source.id)
    if (!sources.value.length) rssEditMode.value = false
    if (selectedSourceId.value === source.id) selectedSourceId.value = sources.value[0]?.id || ''
    await loadArticles()
    ElMessage.success('RSS 源已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除 RSS 源失败'))
  }
}

async function openArticle(article) {
  selectedArticle.value = article
  articleDialogVisible.value = true
  articleImagePreviewVisible.value = false
  articleContentLoading.value = true
  try {
    const { data } = await getRSSArticleContent(article.id)
    Object.assign(article, data)
    selectedArticle.value = article
  } catch (err) {
    ElMessage.error(readError(err, '加载 RSS 正文失败'))
  } finally {
    articleContentLoading.value = false
  }
  if (!article.isRead) await updateArticleState(article, { isRead: true }, { silent: true })
}

async function toggleFavorite(article) {
  await updateArticleState(article, { favorite: !article.favorite })
}

async function toggleRead(article) {
  await updateArticleState(article, { isRead: !article.isRead })
}

async function updateArticleState(article, payload, { silent = false } = {}) {
  try {
    const { data } = await updateRSSArticle(article.id, payload)
    Object.assign(article, data)
    if (selectedArticle.value?.id === article.id) selectedArticle.value = article
    if (shouldHideArticle(article)) articles.value = articles.value.filter(item => item.id !== article.id)
    if (!silent) ElMessage.success('文章状态已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新 RSS 文章失败'))
  }
}

function shouldHideArticle(article) {
  if (articleFilter.value === 'unread' && article.isRead) return true
  if (articleFilter.value === 'favorite' && !article.favorite) return true
  return false
}

function articleBodyHTML(article) {
  return article?.content || article?.summary || '无正文内容'
}

function stripHTML(value) {
  return String(value || '')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/p>/gi, '\n\n')
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .trim()
}

function openExternal(url) {
  window.open(url, '_blank', 'noopener,noreferrer')
}

function handleArticleContentClick(event) {
  const image = event?.target?.closest?.('img')
  if (!image) return
  const root = event.currentTarget
  const images = Array.from(root.querySelectorAll('img'))
    .map(item => item.currentSrc || item.src)
    .filter(Boolean)
  if (!images.length) return
  const clickedURL = image.currentSrc || image.src
  articlePreviewImages.value = images
  articlePreviewIndex.value = Math.max(0, images.indexOf(clickedURL))
  articleImagePreviewVisible.value = true
}

function openArticleListImagePreview(article) {
  const images = rssArticleImageList.value
  if (!images.length || !article?.image) return
  articlePreviewImages.value = images
  articlePreviewIndex.value = Math.max(0, images.indexOf(article.image))
  articleImagePreviewVisible.value = true
}

function sourceInitial(source) {
  return String(source?.title || source?.url || 'R').trim().slice(0, 1).toUpperCase() || 'R'
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function articleDateText(article) {
  const raw = String(article?.pubDate || '').trim()
  if (raw) return raw
  const publishedAt = String(article?.publishedAt || '')
  if (publishedAt && !publishedAt.startsWith('0001-01-01')) return formatDate(publishedAt)
  return formatDate(article?.updatedAt)
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}

function normalizeURL(value) {
  return String(value || '').trim()
}

function rssSortOptions(source) {
  const baseURL = String(source?.url || '').trim()
  const raw = String(source?.sortUrl || '').trim()
  if (!raw || raw.startsWith('@js:') || raw.startsWith('<js>')) {
    return baseURL ? [{ label: '全部', value: baseURL }] : []
  }
  const options = raw.split(/(?:&&|\r?\n)+/)
    .map((row, index) => {
      const separator = row.indexOf('::')
      const label = separator >= 0 ? row.slice(0, separator).trim() : `分类 ${index + 1}`
      const value = separator >= 0 ? row.slice(separator + 2).trim() : row.trim()
      return value ? { label: label || `分类 ${index + 1}`, value } : null
    })
    .filter(Boolean)
  return options.length ? options : (baseURL ? [{ label: '全部', value: baseURL }] : [])
}

function syncSelectedSortURL(reset = false) {
  const options = selectedSortOptions.value
  if (reset || !options.some(option => option.value === selectedSortURL.value)) {
    selectedSortURL.value = options[0]?.value || ''
  }
}
</script>

<style scoped>
.rss-manager {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 14px;
  min-height: calc(100vh - 150px);
}

.rss-panel {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  min-width: 0;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: rgba(255, 255, 255, 0.62);
}

.rss-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 12px;
  border-bottom: 1px solid var(--app-border);
}

.rss-head > div:first-child {
  display: grid;
  gap: 2px;
}

.rss-head span,
.rss-source-row small,
.rss-article-row small,
.rss-article-row span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.rss-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.rss-source-import-input {
  display: none;
}

.rss-source-list,
.rss-article-list {
  display: grid;
  align-content: start;
  max-height: calc(100vh - 230px);
  overflow: auto;
}

.rss-source-card,
.rss-article-row {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--app-border);
}

.rss-source-list {
  grid-template-columns: repeat(auto-fill, minmax(112px, 1fr));
  gap: 10px;
  padding: 12px;
}

.rss-source-card {
  position: relative;
  min-width: 0;
  border: 1px solid transparent;
  border-radius: var(--app-radius-sm);
  text-align: center;
}

.rss-source-card.active {
  background: rgba(145, 118, 62, 0.12);
  border-color: rgba(145, 118, 62, 0.3);
}

.rss-source-card button,
.rss-article-row button {
  display: grid;
  min-width: 0;
  gap: 3px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  text-align: left;
}

.rss-source-card button {
  justify-items: center;
  text-align: center;
}

.rss-source-icon {
  display: grid;
  place-items: center;
  width: 50px;
  height: 50px;
  overflow: hidden;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.72);
  border: 1px solid var(--app-border);
  color: var(--app-primary-strong);
  font-weight: 700;
}

.rss-source-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.rss-source-card strong,
.rss-source-card small,
.rss-article-info strong,
.rss-article-info small,
.rss-article-info span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rss-source-tools,
.rss-article-tools {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.rss-source-tools {
  justify-content: center;
  flex-wrap: wrap;
}

.rss-article-tools {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.rss-article-row {
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
}

.rss-article-row button {
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
}

.rss-article-info {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.rss-article-image {
  width: 120px;
  aspect-ratio: 16 / 10;
  overflow: hidden;
  border-radius: var(--app-radius-sm);
  background: rgba(255, 255, 255, 0.6);
  border: 1px solid var(--app-border);
  cursor: zoom-in;
}

.rss-article-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.rss-article-row.read {
  opacity: 0.68;
}

.load-more-rss {
  padding: 12px;
  color: var(--app-primary-strong);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
}

.load-more-rss:disabled {
  color: var(--app-text-muted);
  cursor: default;
}

.rss-reader {
  display: grid;
  gap: 12px;
}

.rss-reader h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 24px;
  line-height: 1.35;
}

.rss-reader small {
  color: var(--app-text-muted);
}

.rss-reader-content {
  max-height: min(62vh, 680px);
  overflow: auto;
  color: var(--app-text);
  font-size: 16px;
  line-height: 1.85;
}

.rss-reader-content :deep(img),
.rss-reader-content :deep(video) {
  max-width: 100%;
  height: auto;
}

.rss-reader-content :deep(img) {
  cursor: zoom-in;
}

.rss-rule-collapse {
  width: 100%;
}

.rss-rule-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
}

.rss-rule-grid :deep(.el-form-item) {
  margin-bottom: 0;
}

.rss-rule-grid :deep(.el-form-item:last-child) {
  grid-column: 1 / -1;
}

@media (max-width: 750px) {
  .rss-manager {
    grid-template-columns: 1fr;
    min-height: 0;
  }

  .rss-source-list,
  .rss-article-list {
    max-height: 40vh;
  }

  .rss-source-card,
  .rss-article-row {
    grid-template-columns: 1fr;
  }

  .rss-article-row button {
    grid-template-columns: 1fr auto;
  }

  .rss-article-image {
    width: 100px;
  }

  .rss-source-tools,
  .rss-article-tools {
    justify-content: flex-start;
  }

  .rss-reader-dialog :deep(.el-dialog) {
    width: 94vw !important;
    margin-top: 3vh;
  }

  .rss-reader-content {
    max-height: 70vh;
  }

  .rss-rule-grid {
    grid-template-columns: 1fr;
  }
}
</style>
