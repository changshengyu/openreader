<template>
  <div v-if="books.length" class="mobile-manage-list">
    <article
      v-for="book in books"
      :key="book.id"
      class="mobile-manage-card"
      :class="{ selected: selectedBookIds.includes(book.id) }"
    >
      <header>
        <el-checkbox
          :model-value="selectedBookIds.includes(book.id)"
          @change="value => emit('toggle-selection', book.id, value)"
        />
        <span
          class="mobile-manage-cover"
          :class="{ 'has-cover': hasBookCover(book) }"
          :style="coverStyle(book)"
        >{{ coverInitial(book) }}</span>
        <button type="button" @click="emit('open-info', book)">
          <strong>{{ book.title }}</strong>
          <span>
            {{ book.author || '未知作者' }} · {{ categoryName(book) }}
          </span>
          <span>
            {{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }}
            · {{ progressLabel(book) }}
          </span>
        </button>
      </header>
      <p>
        共 {{ book.chapterCount || 0 }} 章
        <template v-if="Number(book.sourceId || 0) > 0">
          · 服务器缓存 {{ serverCacheCount(book) }} 章
        </template>
        · 浏览器缓存 {{ localCacheCount(book) }} 章
        <template v-if="book.lastChapter">
          · 最新：{{ book.lastChapter }}
        </template>
      </p>
      <footer>
        <BookManagementActions
          :book="book"
          :caching="cachingBookId === book.id"
          compact
          @edit="emit('open-edit', book)"
          @group="emit('set-group', book)"
          @cache="command => emit('cache', book, command)"
          @export="format => emit('export', book, format)"
        />
      </footer>
    </article>
  </div>
  <el-empty
    v-else
    class="mobile-manage-empty"
    description="没有匹配的书籍"
  />
</template>

<script setup>
import { bookCoverUrl, hasBookCover } from '../../utils/bookCover'
import BookManagementActions from './BookManagementActions.vue'

defineProps({
  books: {
    type: Array,
    default: () => [],
  },
  selectedBookIds: {
    type: Array,
    default: () => [],
  },
  cachingBookId: {
    type: [String, Number],
    default: null,
  },
  categoryName: {
    type: Function,
    required: true,
  },
  progressLabel: {
    type: Function,
    required: true,
  },
  serverCacheCount: {
    type: Function,
    required: true,
  },
  localCacheCount: {
    type: Function,
    required: true,
  },
})

const emit = defineEmits([
  'toggle-selection',
  'open-info',
  'open-edit',
  'set-group',
  'cache',
  'export',
])

function coverInitial(book) {
  if (hasBookCover(book)) return ''
  return (book?.title || '?').slice(0, 1)
}

function coverStyle(book) {
  const url = bookCoverUrl(book)
  return url ? { backgroundImage: `url(${url})` } : {}
}
</script>

<style scoped>
.mobile-manage-list {
  display: none;
}

.mobile-manage-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-manage-card.selected {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.mobile-manage-card header,
.mobile-manage-card footer {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-manage-card header button {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  text-align: left;
}

.mobile-manage-cover {
  display: grid;
  width: 34px;
  height: 46px;
  place-items: center;
  flex: 0 0 34px;
  color: #fffdf8;
  background: var(--app-primary);
  border-radius: 4px;
  font-size: 16px;
  font-weight: 800;
}

.mobile-manage-cover.has-cover {
  background-position: center;
  background-size: cover;
  color: transparent;
}

.mobile-manage-card strong,
.mobile-manage-card span,
.mobile-manage-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-manage-card strong {
  font-size: 14px;
}

.mobile-manage-card span,
.mobile-manage-card p {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-manage-card p {
  margin: 0;
}

.mobile-manage-card footer {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.mobile-manage-empty {
  display: none;
}

@media (max-width: 750px) {
  .mobile-manage-list {
    display: grid;
    max-height: calc(100vh - 220px);
    overflow: auto;
    gap: 10px;
    margin-bottom: 12px;
  }

  .mobile-manage-empty {
    display: block;
  }
}
</style>
