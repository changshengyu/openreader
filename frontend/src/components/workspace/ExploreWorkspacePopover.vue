<template>
  <section
    class="explore-workspace-popover"
    :class="{ 'mobile-explore-workspace-popover': isMobile }"
    role="dialog"
    aria-label="书海"
    @click.stop
  >
    <header class="explore-popover-head">
      <h2>书海</h2>
      <div>
        <span>共{{ filteredSources.length }}个可用书源</span>
        <button v-if="isMobile" type="button" aria-label="关闭书海" @click="close">×</button>
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
      <el-collapse v-model="expandedSources">
        <el-collapse-item v-for="source in filteredSources" :key="source.id" :name="String(source.id)">
          <template #title>
            <span class="explore-source-title">{{ source.name }}</span>
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
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { exploreBooks, listExploreSources } from '../../api/explore'
import { useIndexWorkspaceStore } from '../../stores/indexWorkspace'
import { createAuthenticatedOperationGuard } from '../../utils/authenticatedOperation'
import {
  captureWorkspaceSession,
  createAsyncRequestGate,
  isWorkspaceSessionCurrent,
} from '../../utils/workspaceContinuation'
import {
  expandedExploreSources,
  exploreSourceGroupOptions,
  filteredExploreSources,
  toggledExploreGroup,
} from '../../utils/exploreChooserPresentation.js'

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
const expandedSources = ref([])
const loadingSources = ref(false)
const loadingEntry = ref(false)
const sourceList = ref(null)
const requestGate = createAsyncRequestGate()
const exploreSessionOperations = createAuthenticatedOperationGuard()

const sourceGroups = computed(() => exploreSourceGroupOptions(sources.value))
const filteredSources = computed(() => filteredExploreSources(sources.value, selectedGroup.value))

onMounted(loadSources)

onBeforeUnmount(() => {
  requestGate.invalidate()
  exploreSessionOperations.reset()
})

watch(
  () => workspace.exploreChooserRevision,
  () => applyWorkspaceIntent(),
  { immediate: true },
)

async function loadSources() {
  const operation = exploreSessionOperations.begin('sources')
  loadingSources.value = true
  try {
    const { data } = await listExploreSources()
    if (!exploreSessionOperations.canCommit(operation)) return
    sources.value = Array.isArray(data) ? data : []
    applyWorkspaceIntent()
  } catch (error) {
    if (exploreSessionOperations.canCommit(operation)) {
      ElMessage.error(readError(error, '加载探索书源失败'))
    }
  } finally {
    if (exploreSessionOperations.canCommit(operation)) loadingSources.value = false
  }
}

function applyWorkspaceIntent() {
  const intent = workspace.explore
  if (intent.sourceGroup) selectedGroup.value = intent.sourceGroup
  if (intent.sourceId) {
    expandedSources.value = expandedExploreSources(expandedSources.value, intent.sourceId)
  }
}

function toggleGroup(group) {
  selectedGroup.value = toggledExploreGroup(selectedGroup.value, group)
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
  const operation = exploreSessionOperations.begin('entry')
  const workspaceStamp = captureWorkspaceSession(workspace)
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
    if (!isActiveEntryRequest(requestToken, operation, workspaceStamp)) return
    const result = normalizeExploreResult(data, 1)
    workspace.showExploreResults(result.items, {
      ...intent,
      page: result.page,
      hasMore: result.hasMore,
    })
    emit('selected')
  } catch (error) {
    if (isActiveEntryRequest(requestToken, operation, workspaceStamp)) {
      ElMessage.error(readError(error, '探索失败'))
    }
  } finally {
    if (isActiveEntryRequest(requestToken, operation, workspaceStamp)) loadingEntry.value = false
  }
}

function isActiveEntryRequest(requestToken, operation, workspaceStamp) {
  return requestGate.isCurrent(requestToken)
    && exploreSessionOperations.canCommit(operation)
    && isWorkspaceSessionCurrent(workspace, workspaceStamp)
}

function close() {
  requestGate.invalidate()
  exploreSessionOperations.invalidate('entry')
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
  grid-template-rows: auto auto 300px;
  width: min(600px, 100vw);
  min-width: min(600px, 100vw);
  max-width: 600px;
  min-height: 0;
  box-sizing: border-box;
  overflow: hidden;
  color: var(--app-text);
  background: var(--app-surface, #fff);
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

.explore-popover-head {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
  padding: calc(24px + env(safe-area-inset-top)) 24px 0;
}

.explore-popover-head h2 {
  margin: 0;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
  font-weight: 400;
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
  padding: 5px 24px;
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
  height: 300px;
  min-height: 0;
  padding: 0 24px 13px;
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

.explore-entry-row {
  display: flex;
  min-width: 0;
  flex-wrap: nowrap;
  justify-content: space-between;
  gap: 15px;
  padding: 2px 0 5px;
  overflow-x: auto;
  border-bottom: 1px dashed var(--app-border);
  scrollbar-width: none;
}

.mobile-explore-workspace-popover {
  width: 100vw;
  min-width: 100vw;
  max-width: 100vw;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

@media (max-width: 750px) {
  .explore-popover-head {
    padding: calc(24px + env(safe-area-inset-top)) 24px 0;
  }

  .explore-source-groups {
    padding-right: 24px;
    padding-left: 24px;
  }

  .explore-source-list {
    padding: 0 24px 13px;
  }
}
</style>
