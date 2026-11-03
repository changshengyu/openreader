<template>
  <!-- reader-dev BookShelf.vue: its own title zone over a 300px single-column list. -->
  <div class="reader-shelf-popup">
    <div class="title-zone">
      <div class="title">书架({{ books.length }})</div>
      <div class="title-btn" :class="{ loading }">
        <button type="button" :disabled="loading" @click="$emit('refresh')">
          <el-icon v-if="loading" class="is-loading"><Loading /></el-icon>
          {{ loading ? '刷新中...' : '刷新' }}
        </button>
      </div>
    </div>
    <div ref="listRef" v-loading="loading" class="reader-shelf-list">
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
        <span class="reader-shelf-chapter">{{ readChapterTitle(item) }}</span>
      </button>
      <el-empty v-if="!loading && !books.length" description="书架暂无书籍" />
    </div>
  </div>
</template>

<script setup>
import { Loading } from '@element-plus/icons-vue'
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

defineEmits(['select', 'refresh'])

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

// reader-dev BookShelf.vue#jumpToActive: the current book scrolls to the top of the list.
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
    list.scrollTop = Math.max(
      0,
      list.scrollTop + active.getBoundingClientRect().top - list.getBoundingClientRect().top,
    )
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
.title-zone {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: space-between;
  width: 100%;
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

.title-btn {
  color: #ed4259;
  font-size: 14px;
  line-height: 26px;
}

.title-btn.loading {
  color: #606266;
}

.title-btn button {
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  line-height: inherit;
}

.title-btn button:disabled {
  cursor: default;
}

.reader-shelf-list {
  display: flex;
  flex-direction: column;
  height: 300px;
  gap: 16px;
  overflow: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
}

.reader-shelf-list::-webkit-scrollbar {
  width: 0;
}

.reader-shelf-card {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  padding: 8px 0;
  color: inherit;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--reader-list-border, #eee);
  cursor: pointer;
  font: inherit;
  text-align: inherit;
}

.reader-shelf-title-line {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  color: var(--reader-shelf-title-text, inherit);
}

.reader-shelf-title-line strong {
  overflow: hidden;
  font-size: 16px;
  font-weight: 400;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reader-shelf-title-line em {
  font-size: 12px;
  font-style: normal;
}

.reader-shelf-card.active strong {
  color: #eb4259;
}

.reader-shelf-chapter {
  display: block;
  overflow: hidden;
  margin-top: 6px;
  color: var(--reader-shelf-chapter-text, #888);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
