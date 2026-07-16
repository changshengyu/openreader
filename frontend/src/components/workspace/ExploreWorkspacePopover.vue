<template>
  <section
    class="explore-workspace-popover"
    :class="{ 'mobile-explore-workspace-popover': isMobile }"
    role="dialog"
    aria-modal="true"
    aria-label="书海"
    @click.stop
  >
    <header class="explore-popover-head">
      <h2>书海</h2>
      <div>
        <span>共{{ filteredSources.length }}个可用书源</span>
        <button type="button" aria-label="关闭书海" @click="close">×</button>
      </div>
    </header>

    <div v-if="sourceGroups.length" class="explore-source-groups" role="tablist" aria-label="书源分组">
      <button
        v-for="group in sourceGroups"
        :key="group.value"
        type="button"
        :class="{ active: selectedGroup === group.value }"
        role="tab"
        :aria-selected="selectedGroup === group.value"
        @click="toggleGroup(group.value)"
      >{{ group.label }}</button>
    </div>

    <div ref="sourceList" v-loading="loadingSources || loadingEntry" class="explore-source-list">
      <el-collapse v-model="expandedSources" accordion>
        <el-collapse-item v-for="source in filteredSources" :key="source.id" :name="String(source.id)">
          <template #title>
            <span class="explore-source-title">{{ source.name }}</span>
            <span class="explore-source-group">{{ source.group || '未分组' }}</span>
          </template>
          <div v-for="(group, groupIndex) in sourceExploreGroups(source)" :key="`${source.id}-${groupIndex}`" class="explore-entry-row">
            <button
              v-for="entry in group"
              :key="entry.url"
              type="button"
              :class="{ active: isActiveEntry(source, entry) }"
              @click="selectEntry(source, entry)"
            >{{ entry.name }}</button>
          </div>
        </el-collapse-item>
      </el-collapse>
      <el-empty v-if="!loadingSources && !filteredSources.length" description="没有配置 exploreUrl 的书源" />
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { exploreBooks, listExploreSources } from '../../api/explore'
import { useIndexWorkspaceStore } from '../../stores/indexWorkspace'
import { createAsyncRequestGate } from '../../utils/workspaceContinuation'

const props = defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['close', 'selected'])
const workspace = useIndexWorkspaceStore()
const sources = ref([])
const selectedGroup = ref('')
const expandedSources = ref('')
const loadingSources = ref(false)
const loadingEntry = ref(false)
const sourceList = ref(null)
const requestGate = createAsyncRequestGate()

const sourceGroups = computed(() => {
  const groups = new Set()
  for (const source of sources.value) {
    const group = String(source.group || '').trim()
    if (group) groups.add(group)
  }
  return [...groups].sort((a, b) => a.localeCompare(b)).map(value => ({ label: value, value }))
    .concat(sources.value.some(source => !String(source.group || '').trim()) ? [{ label: '未分组', value: '未分组' }] : [])
})
const filteredSources = computed(() => {
  if (!selectedGroup.value) return sources.value
  return sources.value.filter(source => (String(source.group || '').trim() || '未分组') === selectedGroup.value)
})

onMounted(loadSources)

watch(
  () => workspace.exploreChooserRevision,
  () => applyWorkspaceIntent(),
  { immediate: true },
)

async function loadSources() {
  loadingSources.value = true
  try {
    const { data } = await listExploreSources()
    sources.value = Array.isArray(data) ? data : []
    applyWorkspaceIntent()
  } catch (error) {
    ElMessage.error(readError(error, '加载探索书源失败'))
  } finally {
    loadingSources.value = false
  }
}

function applyWorkspaceIntent() {
  const intent = workspace.explore
  if (intent.sourceGroup) selectedGroup.value = intent.sourceGroup
  if (intent.sourceId) expandedSources.value = String(intent.sourceId)
}

function toggleGroup(group) {
  selectedGroup.value = selectedGroup.value === group ? '' : group
}

function sourceExploreGroups(source) {
  return Array.isArray(source?.exploreGroups)
    ? source.exploreGroups.filter(group => Array.isArray(group) && group.length)
    : []
}

function isActiveEntry(source, entry) {
  return String(workspace.explore.sourceId) === String(source.id)
    && workspace.explore.url === entry.url
}

async function selectEntry(source, entry) {
  if (loadingEntry.value) return
  const requestToken = requestGate.begin()
  const intent = {
    sourceId: source.id,
    sourceGroup: source.group || '',
    sourceName: source.name || '',
    url: entry.url,
    name: entry.name || '',
  }
  loadingEntry.value = true
  try {
    const { data } = await exploreBooks(intent.sourceId, { page: 1, url: intent.url })
    if (!requestGate.isCurrent(requestToken)) return
    const result = normalizeExploreResult(data, 1)
    workspace.showExploreResults(result.items, {
      ...intent,
      page: result.page,
      hasMore: result.hasMore,
    })
    emit('selected')
  } catch (error) {
    if (requestGate.isCurrent(requestToken)) {
      ElMessage.error(readError(error, '探索失败'))
    }
  } finally {
    if (requestGate.isCurrent(requestToken)) loadingEntry.value = false
  }
}

function close() {
  requestGate.invalidate()
  emit('close')
}

function normalizeExploreResult(data, fallbackPage) {
  if (Array.isArray(data)) return { items: data, page: fallbackPage, hasMore: false }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    page: Number(data?.page || fallbackPage),
    hasMore: Boolean(data?.hasMore),
  }
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || error?.message || fallback
}
</script>

<style scoped>
.explore-workspace-popover {
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  min-width: min(520px, calc(100vw - 32px));
  max-width: min(520px, calc(100vw - 32px));
  min-height: 0;
  max-height: min(640px, calc(100dvh - 32px));
  box-sizing: border-box;
  overflow: hidden;
  color: var(--app-text);
  background: var(--app-surface, #fff);
  border: 1px solid var(--app-border);
  border-radius: 4px;
  box-shadow: var(--app-shadow-md);
}

.explore-popover-head {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 14px;
}

.explore-popover-head h2 {
  margin: 0;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
  font-weight: 500;
  line-height: 1.45;
}

.explore-popover-head > div {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 16px;
  color: var(--app-text-muted);
  font-size: 13px;
}

.explore-popover-head span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.explore-popover-head button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  place-items: center;
  flex: 0 0 28px;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 24px;
  line-height: 1;
}

.explore-source-groups {
  display: flex;
  min-width: 0;
  gap: 10px;
  overflow-x: auto;
  padding: 0 24px 10px;
}

.explore-source-groups button,
.explore-entry-row button {
  flex: 0 0 auto;
  padding: 4px 10px;
  color: var(--app-text);
  background: transparent;
  border: 1px solid var(--app-border);
  border-radius: 4px;
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  line-height: 1.5;
  white-space: nowrap;
}

.explore-source-groups button.active,
.explore-entry-row button.active,
.explore-source-groups button:hover,
.explore-entry-row button:hover {
  color: #ed4259;
  border-color: #ed4259;
}

.explore-source-list {
  min-height: 0;
  padding: 0 24px 20px;
  overflow: auto;
  scrollbar-width: none;
}

.explore-source-list::-webkit-scrollbar {
  display: none;
}

.explore-source-list :deep(.el-collapse),
.explore-source-list :deep(.el-collapse-item__wrap) {
  border: 0;
  background: transparent;
}

.explore-source-list :deep(.el-collapse-item__header) {
  min-width: 0;
  gap: 10px;
  color: var(--app-text);
  background: transparent;
}

.explore-source-title {
  min-width: 0;
  overflow: hidden;
  flex: 1;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.explore-source-group {
  color: var(--app-text-muted);
  font-size: 12px;
}

.explore-entry-row {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px 15px;
  padding: 6px 0 12px;
  border-bottom: 1px dashed var(--app-border);
}

.mobile-explore-workspace-popover {
  min-width: 100%;
  max-width: 100%;
  min-height: 100%;
  max-height: 100%;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

@media (max-width: 750px) {
  .explore-popover-head {
    padding: max(20px, env(safe-area-inset-top)) 24px 14px;
  }

  .explore-source-groups {
    padding-right: 24px;
    padding-left: 24px;
  }

  .explore-source-list {
    padding: 0 24px calc(20px + env(safe-area-inset-bottom));
  }
}
</style>
