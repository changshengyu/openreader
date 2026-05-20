<template>
  <section class="book-info-shared">
    <div class="book-cover-zone">
      <div class="book-cover-bg" :style="coverBgStyle" />
      <BookCover :book="book" />
    </div>
    <div class="book-info-main">
      <div class="book-info-title">
        <h2>{{ book?.title || '未命名书籍' }}</h2>
        <el-tag v-if="statusLabel" size="small" effect="plain" :type="statusType">{{ statusLabel }}</el-tag>
      </div>
      <div class="book-props">
        <div>
          <span>作者：</span>
          <strong>{{ book?.author || '未知' }}</strong>
        </div>
        <div>
          <span>来源：</span>
          <strong>{{ sourceName || (book?.sourceId ? '远程书籍' : '本地') }}</strong>
        </div>
        <div>
          <span>最新：</span>
          <strong>{{ book?.lastChapter || '-' }}</strong>
        </div>
        <div>
          <span>分组：</span>
          <strong>{{ categoryName || '未分组' }}</strong>
        </div>
        <div>
          <span>章节：</span>
          <strong>{{ chapterCount }}</strong>
        </div>
        <div>
          <span>进度：</span>
          <strong>{{ progressLabel }}</strong>
        </div>
      </div>
      <div class="book-info-intro">
        <p v-for="(paragraph, index) in introParagraphs" :key="index">{{ paragraph }}</p>
      </div>
      <slot />
    </div>
  </section>
</template>

<script setup>
import { computed } from 'vue'
import BookCover from './BookCover.vue'

const props = defineProps({
  book: {
    type: Object,
    default: () => ({}),
  },
  sourceName: {
    type: String,
    default: '',
  },
  categoryName: {
    type: String,
    default: '',
  },
  progress: {
    type: Number,
    default: 0,
  },
  chapters: {
    type: [Array, Number],
    default: 0,
  },
  statusLabel: {
    type: String,
    default: '',
  },
  statusType: {
    type: String,
    default: 'info',
  },
})

const chapterCount = computed(() => Array.isArray(props.chapters) ? props.chapters.length : (props.chapters || props.book?.chapterCount || 0))
const progressLabel = computed(() => `${Math.round(Math.max(0, Math.min(1, props.progress || 0)) * 100)}%`)
const introParagraphs = computed(() => {
  const text = String(props.book?.intro || '暂无简介').trim()
  return text ? text.split(/\n+/).map(line => line.trim()).filter(Boolean) : ['暂无简介']
})
const coverBgStyle = computed(() => props.book?.coverUrl ? { backgroundImage: `url(${props.book.coverUrl})` } : {})
</script>

<style scoped>
.book-info-shared {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.book-cover-zone {
  position: relative;
  display: grid;
  width: 112px;
  min-height: 150px;
  place-items: center;
  overflow: hidden;
  border-radius: 6px;
}

.book-cover-bg {
  position: absolute;
  inset: 0;
  background: var(--app-bg-soft);
  background-position: center;
  background-size: cover;
  filter: blur(14px);
  opacity: 0.34;
  transform: scale(1.18);
}

.book-cover-zone :deep(.book-cover-shared) {
  position: relative;
  z-index: 1;
}

.book-info-main {
  display: grid;
  min-width: 0;
  gap: 10px;
}

.book-info-title {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 10px;
}

.book-info-title h2,
.book-info-intro {
  margin: 0;
}

.book-info-title h2 {
  min-width: 0;
  font-size: 21px;
  line-height: 1.25;
}

.book-props span,
.book-info-intro {
  color: var(--app-text-muted);
}

.book-info-intro {
  line-height: 1.7;
  max-height: 180px;
  overflow: auto;
}

.book-info-intro p {
  margin: 0 0 6px;
  text-indent: 2em;
}

.book-props {
  display: grid;
  gap: 7px;
}

.book-props div {
  display: flex;
  gap: 4px;
  min-width: 0;
  font-size: 13px;
}

.book-props strong {
  min-width: 0;
  overflow: hidden;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 560px) {
  .book-info-shared {
    grid-template-columns: 1fr;
  }

  .book-cover-zone {
    justify-self: center;
    width: 128px;
    min-height: 172px;
  }

  .book-info-title {
    display: grid;
    justify-items: center;
    text-align: center;
  }

  .book-info-main {
    gap: 12px;
  }
}
</style>
