<template>
  <!-- reader-dev BookSource.vue: title zone with the group select, then a 300px list. -->
  <div class="title-zone">
    <div class="title">来源({{ sources.length }})</div>
    <div class="title-actions" :class="{ loading: loadingMore }">
      <el-select
        :model-value="group"
        size="small"
        placeholder="全部分组"
        filterable
        class="source-group-select"
        :disabled="opening"
        @update:model-value="$emit('groupChange', $event || '')"
      >
        <el-option v-for="item in normalizedGroups" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <button type="button" :disabled="opening || refreshing || loadingMore" @click="$emit('refresh')">{{ refreshing ? '刷新中...' : '刷新' }}</button>
      <button type="button" :disabled="opening || refreshing || loadingMore || !hasMore" @click="$emit('loadMore')">{{ loadingMore ? '加载中...' : (hasMore ? '加载更多' : '没有更多') }}</button>
    </div>
  </div>

  <div ref="sourceList" class="source-switch-list">
    <button
      v-for="source in sources"
      :key="sourceKey(source)"
      :ref="setSourceItemRef"
      class="source-item"
      :class="{ selected: isSelected(source) }"
      type="button"
      :disabled="isSelected(source) || changingSource === sourceId(source)"
      @click="$emit('change', source)"
    >
      <div class="source-title">
        <span class="source-name">{{ sourceName(source) }}</span>
        <span class="source-time">{{ sourceTime(source) }}</span>
      </div>
      <div class="source-latest-chapter">{{ latestChapter(source) || '无最新章节' }}</div>
      <small v-if="changingSource === sourceId(source)" class="source-status">切换中...</small>
    </button>
    <el-empty v-if="!loading && !sources.length" description="没有找到可用来源" />
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUpdate, ref, watch } from 'vue'
import {
  sourceCandidateBookUrl,
  sourceCandidateKey,
  sourceCandidateSourceName,
  sourceCandidateSourceId,
} from '../../utils/sourceCandidate'

const props = defineProps({
  book: {
    type: Object,
    default: null,
  },
  sources: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
  opening: {
    type: Boolean,
    default: false,
  },
  refreshing: {
    type: Boolean,
    default: false,
  },
  loadingMore: {
    type: Boolean,
    default: false,
  },
  group: {
    type: String,
    default: '',
  },
  groups: {
    type: Array,
    default: () => [],
  },
  changingSource: {
    type: [Number, String],
    default: null,
  },
  currentSourceName: {
    type: String,
    default: '',
  },
  hasMore: {
    type: Boolean,
    default: true,
  },
  sourceTotal: {
    type: Number,
    default: 0,
  },
})

defineEmits(['refresh', 'loadMore', 'groupChange', 'change'])

const sourceList = ref(null)
const sourceItemRefs = ref([])

// reader-dev bookSourceGroupList starts with 全部分组 and the number of book sources.
const normalizedGroups = computed(() => [{ value: '', label: `全部分组 (${props.sourceTotal})` }, ...props.groups.map((item) => {
  if (typeof item === 'string') {
    return { value: item, label: item }
  }
  const value = item?.value ?? item?.name ?? ''
  const count = Number(item?.count || 0)
  return {
    value,
    label: count > 0 ? `${item?.label || value} (${count})` : (item?.label || value),
  }
}).filter(item => item.value)])

onBeforeUpdate(() => {
  sourceItemRefs.value = []
})

watch(
  () => [props.sources.map(sourceKey).join('|'), currentBookKey()],
  () => nextTick(jumpToActive),
  { immediate: true },
)

function sourceName(source) {
  return sourceCandidateSourceName(source)
}

function sourceKey(source) {
  return sourceCandidateKey(source)
}

function sourceId(source) {
  return sourceCandidateSourceId(source)
}

function latestChapter(source) {
  return source?.latestChapterTitle || source?.latestChapter || source?.lastChapter || ''
}

function sourceTime(source) {
  const time = Number(source?.time || 0)
  return time > 0 ? `⏱ ${time}ms` : ''
}

function currentBookKey() {
  if (!props.book) return ''
  return `${props.book.sourceId || props.book.bookSourceId || ''}-${props.book.url || props.book.bookUrl || ''}`
}

function isSelected(source) {
  if (source?.current) return true
  const bookSourceId = String(props.book?.sourceId || props.book?.bookSourceId || '')
  const bookUrl = String(props.book?.url || props.book?.bookUrl || '')
  return String(sourceId(source)) === bookSourceId && String(sourceCandidateBookUrl(source)) === bookUrl
}

function setSourceItemRef(el) {
  if (el) sourceItemRefs.value.push(el)
}

// reader-dev BookSource.vue#jumpToActive: the current source scrolls to the top of the list.
function jumpToActive() {
  const index = props.sources.findIndex(isSelected)
  const target = sourceItemRefs.value[index]
  if (!target || !sourceList.value) return
  const wrapper = sourceList.value
  wrapper.scrollTop = Math.max(
    0,
    wrapper.scrollTop + target.getBoundingClientRect().top - wrapper.getBoundingClientRect().top,
  )
}
</script>

<style scoped>
.title-zone {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: space-between;
  width: 100%;
  min-width: 0;
  margin: 0 0 20px;
}

.title {
  width: fit-content;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-family: -apple-system, "Noto Sans", "Helvetica Neue", Helvetica, "Nimbus Sans L", Arial, "Liberation Sans", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", "Source Han Sans SC", "Source Han Sans CN", "Microsoft YaHei", "Wenquanyi Micro Hei", "WenQuanYi Zen Hei", "ST Heiti", SimHei, "WenQuanYi Zen Hei Sharp", sans-serif;
  font-size: 18px;
  font-weight: 400;
}

.title-actions {
  display: flex;
  align-items: center;
  color: #ed4259;
  cursor: pointer;
  font-size: 14px;
  line-height: 26px;
}

.title-actions.loading {
  color: #606266;
}

.title-actions button {
  margin-left: 15px;
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  line-height: inherit;
}

.title-actions button:disabled {
  color: #606266;
  cursor: default;
}

/* reader-dev el-select size mini: 140×28, 12px text 15px in. */
.source-group-select {
  width: 140px;
}

.source-group-select :deep(.el-select__wrapper) {
  min-height: 28px;
  padding: 0 10px 0 15px;
  font-size: 12px;
  line-height: 28px;
}

.source-switch-list {
  height: 300px;
  min-width: 0;
  overflow: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
}

.source-switch-list::-webkit-scrollbar {
  width: 0 !important;
}

.source-item {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
  padding: 8px 0;
  color: var(--reader-list-text, inherit);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--reader-list-border, #eee);
  cursor: pointer;
  font: inherit;
  text-align: inherit;
}

.source-item:disabled {
  cursor: default;
  opacity: 1;
}

.source-title {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
}

.source-name {
  min-width: 0;
  overflow: hidden;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-time {
  font-size: 12px;
}

.source-latest-chapter {
  min-width: 0;
  overflow: hidden;
  margin-top: 6px;
  color: var(--reader-source-chapter-text, #888);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-status {
  color: var(--reader-source-chapter-text, #888);
  font-size: 12px;
}

.source-item.selected .source-name {
  color: #eb4259;
}
</style>
