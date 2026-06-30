<template>
  <div ref="listRef" class="reader-shelf-list">
    <button
      v-for="item in books"
      :key="item.id"
      class="reader-shelf-card"
      :class="{ active: Number(item.id) === Number(currentBookId) }"
      :data-book-id="item.id"
      type="button"
      @click="$emit('select', item)"
    >
      <span class="reader-shelf-title-line">
        <strong>{{ item.title }}</strong>
        <em v-if="unreadCount(item)">{{ unreadCount(item) }}</em>
      </span>
      <span class="reader-shelf-chapter">{{ readChapterTitle(item) || '尚未阅读' }}</span>
    </button>
    <el-empty v-if="!loading && !books.length" description="书架暂无书籍" />
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { newestBookProgress } from '../../utils/bookOrder'

const props = defineProps({
  books: {
    type: Array,
    default: () => [],
  },
  currentBookId: {
    type: [Number, String],
    default: 0,
  },
  progressByBook: {
    type: Object,
    default: () => ({}),
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['select'])

const listRef = ref(null)
let locateTimer = 0

function bookProgress(item) {
  return newestBookProgress(item, props.progressByBook)
}

function readChapterTitle(item) {
  const progress = bookProgress(item)
  return progress?.chapterTitle || item.durChapterTitle || ''
}

function unreadCount(item) {
  const progress = bookProgress(item)
  const chapterIndex = Number.isInteger(progress?.chapterIndex) ? progress.chapterIndex : -1
  const total = Number(item.chapterCount || item.totalChapterNum || 0)
  return Math.max(0, total - 1 - chapterIndex)
}

function locateCurrentBook(attempt = 0) {
  if (locateTimer) window.clearTimeout(locateTimer)
  nextTick(() => {
    const list = listRef.value
    const active = list?.querySelector?.(`[data-book-id="${props.currentBookId}"]`)
    if (!list || !active) {
      if (attempt < 20 && props.books.length) {
        locateTimer = window.setTimeout(() => locateCurrentBook(attempt + 1), 50)
      }
      return
    }
    const targetTop = active.offsetTop - Math.max(0, (list.clientHeight - active.clientHeight) / 2)
    const nextTop = Math.max(0, targetTop)
    list.scrollTo({ top: nextTop, behavior: 'auto' })
    requestAnimationFrame(() => {
      list.scrollTop = nextTop
      active.scrollIntoView({ block: 'center', inline: 'nearest' })
    })
  })
}

defineExpose({ locateCurrentBook })

watch(
  () => [props.currentBookId, props.books.length],
  () => locateCurrentBook(),
)

onBeforeUnmount(() => {
  if (locateTimer) window.clearTimeout(locateTimer)
})
</script>

<style scoped>
.reader-shelf-list {
  display: grid;
  max-height: calc(100vh - 154px);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.reader-shelf-card {
  display: grid;
  gap: 6px;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  padding: 8px 0;
  color: #24282c;
  background: transparent;
  border: 0;
  border-bottom: 1px solid rgba(160, 139, 91, 0.22);
  cursor: pointer;
  text-align: left;
}

.reader-shelf-card:hover,
.reader-shelf-card.active {
  color: #ed4259;
  background: transparent;
}

.reader-shelf-title-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.reader-shelf-title-line strong,
.reader-shelf-chapter {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reader-shelf-title-line strong {
  font-size: 16px;
  font-weight: 500;
}

.reader-shelf-title-line em {
  flex: 0 0 auto;
  color: #ed4259;
  font-size: 12px;
  font-style: normal;
}

.reader-shelf-chapter {
  color: #888;
  font-size: 14px;
}

@media (min-width: 900px) {
  .reader-shelf-list {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    align-content: start;
    gap: 16px 24px;
  }

  .reader-shelf-card {
    min-width: 0;
    padding: 10px 0;
  }

  .reader-shelf-list :deep(.el-empty) {
    grid-column: 1 / -1;
  }
}
</style>
