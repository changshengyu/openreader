<template>
  <div class="title-zone">
    <div class="title">来源({{ sources.length }})</div>
    <div class="title-actions" :class="{ loading }">
      <el-select
        :model-value="group"
        size="small"
        placeholder="全部分组"
        clearable
        filterable
        class="source-group-select"
        @update:model-value="$emit('groupChange', $event || '')"
      >
        <el-option v-for="item in normalizedGroups" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <button type="button" :disabled="loading" @click="$emit('refresh')">{{ loading ? '刷新中...' : '刷新' }}</button>
      <button type="button" :disabled="loading || !hasMore" @click="$emit('loadMore')">{{ loading ? '加载中...' : (hasMore ? '加载更多' : '没有更多') }}</button>
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
})

defineEmits(['refresh', 'loadMore', 'groupChange', 'change'])

const sourceList = ref(null)
const sourceItemRefs = ref([])

const normalizedGroups = computed(() => props.groups.map((item) => {
  if (typeof item === 'string') {
    return { value: item, label: item }
  }
  const value = item?.value ?? item?.name ?? ''
  const count = Number(item?.count || 0)
  return {
    value,
    label: count > 0 ? `${item?.label || value} (${count})` : (item?.label || value),
  }
}).filter(item => item.value))

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

function jumpToActive() {
  const index = props.sources.findIndex(isSelected)
  const target = sourceItemRefs.value[index]
  if (!target || !sourceList.value) return
  const wrapper = sourceList.value
  const nextTop = target.offsetTop - wrapper.clientHeight / 2 + target.clientHeight / 2
  wrapper.scrollTop = Math.max(0, nextTop)
}
</script>

<style scoped>
.title-zone {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
  min-width: 0;
}

.title {
  width: fit-content;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
  font-weight: 400;
}

.title-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  color: #ed4259;
  font-size: 14px;
  line-height: 26px;
}

.title-actions button {
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
}

.title-actions button:disabled {
  color: #999;
  cursor: default;
}

.source-group-select {
  width: 140px;
}

.source-switch-list {
  height: min(52vh, 360px);
  overflow: auto;
  min-width: 0;
}

.source-switch-list::-webkit-scrollbar {
  width: 0 !important;
}

.source-item {
  display: grid;
  gap: 6px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding: 9px 0;
  color: #24282c;
  background: transparent;
  border: 0;
  border-bottom: 1px solid #eee;
  cursor: pointer;
  text-align: left;
}

.source-item:hover {
  background: rgba(237, 66, 89, 0.05);
}

.source-item:disabled {
  cursor: default;
  opacity: 1;
}

.source-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.source-name {
  min-width: 0;
  overflow: hidden;
  color: #24282c;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-time {
  flex: 0 0 auto;
  color: #888;
  font-size: 12px;
}

.source-latest-chapter {
  min-width: 0;
  overflow: hidden;
  color: #888;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-status {
  color: #7b715e;
  font-size: 12px;
}

.source-item.selected .source-name {
  color: #ed4259;
}

@media (max-width: 750px) {
  .title-zone {
    gap: 10px;
    margin-bottom: 12px;
  }

  .title-actions {
    gap: 10px;
  }

  .source-group-select {
    width: 132px;
  }

  .source-switch-list {
    height: 46vh;
    padding-bottom: max(8px, env(safe-area-inset-bottom));
  }

  .source-item {
    min-height: 58px;
    padding: 10px 0;
  }

  .source-name {
    font-size: 15px;
  }
}
</style>
