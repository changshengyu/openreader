<template>
  <section class="book-info-shared" :class="`variant-${variant}`">
    <div class="book-cover-zone" :class="{ editable: coverEditable, uploading: coverUploading }" @click="triggerCoverUpload">
      <div class="book-cover-bg" :style="coverBgStyle" />
      <BookCover :book="book" />
      <span v-if="coverEditable" class="cover-edit-label">{{ coverUploading ? '上传中' : '更换封面' }}</span>
      <input
        v-if="coverEditable"
        ref="coverInput"
        type="file"
        accept="image/jpg,image/png,image/jpeg"
        class="cover-file-input"
        @change="handleCoverFileChange"
      />
    </div>
    <div class="book-info-main">
      <div class="book-info-title">
        <h2>{{ bookTitle }}</h2>
        <el-tag v-if="statusLabel && variant !== 'dialog'" size="small" effect="plain" :type="statusType">{{ statusLabel }}</el-tag>
      </div>
      <div v-if="bookKindTags.length" class="book-kind-tags">
        <span v-for="tag in bookKindTags" :key="tag">{{ tag }}</span>
      </div>
      <div class="book-props">
        <div>
          <span>作者：</span>
          <strong>{{ book?.author || '未知' }}</strong>
        </div>
        <div v-if="book?.wordCount">
          <span>字数：</span>
          <strong>{{ book.wordCount }}</strong>
        </div>
        <div>
          <span>来源：</span>
          <strong>{{ displaySourceName }}</strong>
          <button
            v-if="showLocalRefreshAction"
            type="button"
            class="book-prop-action"
            :disabled="localRefreshLoading"
            @click="emit('local-refresh')"
          >
            {{ localRefreshLoading ? '更新中' : '更新' }}
          </button>
        </div>
        <div class="book-latest-prop">
          <span>最新：</span>
          <strong>{{ latestChapterLabel }}</strong>
          <span v-if="showUpdateSwitch && variant === 'dialog'" class="inline-update-switch">
            追更
            <el-switch
              :model-value="canUpdateValue"
              :loading="updateSwitchLoading"
              @change="value => emit('can-update-change', value)"
            />
          </span>
        </div>
        <div v-if="inShelf">
          <span>分组：</span>
          <strong>{{ categoryName || '未分组' }}</strong>
          <button v-if="showCategoryAction" type="button" class="book-prop-action" @click="emit('category-action')">
            {{ categoryActionLabel }}
          </button>
        </div>
        <div v-else-if="showAddAction" class="book-operate-zone">
          <el-tag
            type="success"
            effect="light"
            class="book-operate-btn"
            :class="{ loading: addLoading }"
            role="button"
            tabindex="0"
            @click.stop="emit('add')"
            @keydown.enter.prevent="emit('add')"
            @keydown.space.prevent="emit('add')"
          >
            {{ addLoading ? '加入中…' : '加入书架' }}
          </el-tag>
        </div>
        <div v-if="showStats">
          <span>章节：</span>
          <strong>{{ chapterCount }}</strong>
        </div>
        <div v-if="showStats">
          <span>进度：</span>
          <strong>{{ progressLabel }}</strong>
        </div>
        <div v-if="showStats && browserCacheCount >= 0">
          <span>浏览器缓存：</span>
          <strong>{{ browserCacheCount }} 章</strong>
        </div>
      </div>
      <div v-if="showUpdateSwitch && variant !== 'dialog'" class="book-info-controls">
        <span>追更：</span>
        <el-switch
          :model-value="canUpdateValue"
          :loading="updateSwitchLoading"
          active-text="开启"
          inactive-text="关闭"
          @change="value => emit('can-update-change', value)"
        />
      </div>
      <div class="book-info-intro">
        <p v-for="(paragraph, index) in introParagraphs" :key="index">{{ paragraph }}</p>
      </div>
      <slot />
    </div>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import BookCover from './BookCover.vue'
import { bookCoverUrl } from '../utils/bookCover'

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
  coverEditable: {
    type: Boolean,
    default: false,
  },
  coverUploading: {
    type: Boolean,
    default: false,
  },
  showUpdateSwitch: {
    type: Boolean,
    default: false,
  },
  canUpdate: {
    type: Boolean,
    default: true,
  },
  updateSwitchLoading: {
    type: Boolean,
    default: false,
  },
  browserCacheCount: {
    type: Number,
    default: -1,
  },
  inShelf: {
    type: Boolean,
    default: true,
  },
  showCategoryAction: {
    type: Boolean,
    default: false,
  },
  categoryActionLabel: {
    type: String,
    default: '设置分组',
  },
  showLocalRefreshAction: {
    type: Boolean,
    default: false,
  },
  localRefreshLoading: {
    type: Boolean,
    default: false,
  },
  showAddAction: {
    type: Boolean,
    default: false,
  },
  addLoading: {
    type: Boolean,
    default: false,
  },
  variant: {
    type: String,
    default: 'detail',
    validator: value => ['detail', 'dialog'].includes(value),
  },
})

const emit = defineEmits(['cover-upload', 'can-update-change', 'category-action', 'local-refresh', 'add'])
const coverInput = ref(null)

const bookTitle = computed(() => props.book?.title || props.book?.name || props.book?.bookName || '未命名书籍')
const chapterCount = computed(() => {
  if (Array.isArray(props.chapters)) return props.chapters.length
  return props.chapters || props.book?.chapterCount || props.book?.totalChapterNum || props.book?.chapterNum || 0
})
const latestChapterLabel = computed(() => props.book?.lastChapter || props.book?.latestChapter || props.book?.latestChapterTitle || props.book?.durChapterTitle || '-')
const displaySourceName = computed(() => {
  if (props.sourceName) return props.sourceName
  if (props.book?.sourceName) return props.book.sourceName
  if (props.book?.originName) return props.book.originName
  if (props.book?.origin === 'loc_book' || props.book?.origin === 'local') return '本地'
  if (props.book?.origin) return props.book.origin
  return props.book?.sourceId ? '远程书籍' : '本地'
})
const progressLabel = computed(() => `${Math.round(Math.max(0, Math.min(1, props.progress || 0)) * 100)}%`)
const canUpdateValue = computed(() => props.book?.canUpdate !== false && props.canUpdate !== false)
const showStats = computed(() => props.variant !== 'dialog')
const bookKindTags = computed(() => {
  const raw = props.book?.kind ?? props.book?.category ?? props.book?.categoryName ?? props.book?.genre ?? props.book?.tags ?? props.book?.type
  return normalizeKindTags(raw)
})
const introParagraphs = computed(() => {
  const text = String(props.book?.intro || '暂无简介').trim()
  return text ? text.split(/\n+/).map(line => line.trim()).filter(Boolean) : ['暂无简介']
})
const coverBgStyle = computed(() => {
  const url = bookCoverUrl(props.book)
  return url ? { backgroundImage: `url(${url})` } : {}
})

function normalizeKindTags(value) {
  if (Array.isArray(value)) {
    return value.flatMap(item => normalizeKindTags(item)).filter(Boolean).slice(0, 8)
  }
  return String(value || '')
    .split(/[,\uFF0C|/、]+/)
    .map(item => item.trim())
    .filter(Boolean)
    .slice(0, 8)
}

function triggerCoverUpload() {
  if (!props.coverEditable || props.coverUploading) return
  coverInput.value?.click()
}

function handleCoverFileChange(event) {
  const file = event.target.files?.[0]
  if (file) emit('cover-upload', file)
  event.target.value = ''
}
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

.book-cover-zone.editable {
  cursor: pointer;
}

.book-cover-zone.uploading {
  cursor: progress;
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

.cover-edit-label {
  position: absolute;
  z-index: 2;
  right: 8px;
  bottom: 8px;
  left: 8px;
  padding: 5px 6px;
  color: #fff;
  background: rgba(0, 0, 0, 0.54);
  border-radius: 4px;
  font-size: 12px;
  text-align: center;
}

.cover-file-input {
  display: none;
}

.book-info-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 12px 0 6px;
  color: var(--app-text-muted);
  font-size: 14px;
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

.book-kind-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: -3px;
}

.book-kind-tags span {
  max-width: 100%;
  padding: 3px 8px;
  overflow: hidden;
  color: var(--app-text-muted);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: 999px;
  font-size: 12px;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.book-prop-action {
  flex: 0 0 auto;
  padding: 0;
  color: #409eff;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 13px;
}

.book-props .book-operate-zone {
  min-height: 28px;
  align-items: center;
}

.book-operate-btn {
  cursor: pointer;
  user-select: none;
}

.book-operate-btn.loading {
  cursor: progress;
  pointer-events: none;
}

.book-operate-btn:focus-visible {
  outline: 2px solid var(--el-color-success-light-5);
  outline-offset: 2px;
}

.inline-update-switch {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  white-space: nowrap;
}

.book-info-shared.variant-dialog {
  display: block;
}

.variant-dialog .book-cover-zone {
  width: 100%;
  height: 150px;
  min-height: 150px;
  border-radius: 0;
}

.variant-dialog .book-cover-bg {
  filter: blur(50px);
  opacity: 0.45;
}

.variant-dialog .book-cover-zone :deep(.book-cover-shared) {
  width: 100px;
  height: 150px;
  margin: 0 auto;
  box-shadow: 0 8px 24px rgba(58, 41, 10, 0.18);
}

.variant-dialog .cover-edit-label {
  right: calc(50% - 50px);
  bottom: 8px;
  left: calc(50% - 50px);
}

.variant-dialog .book-info-main {
  display: block;
}

.variant-dialog .book-info-title {
  justify-content: center;
  padding: 10px 0 4px;
  text-align: center;
}

.variant-dialog .book-info-title h2 {
  font-size: 16px;
  font-weight: 600;
}

.variant-dialog .book-kind-tags {
  justify-content: center;
  margin: 0;
  padding: 4px 0;
}

.variant-dialog .book-kind-tags span {
  padding: 0 3px;
  color: #d03050;
  background: transparent;
  border: 0;
  border-radius: 0;
  font-size: 13px;
}

.variant-dialog .book-props {
  gap: 0;
  padding: 5px 0;
}

.variant-dialog .book-props div {
  padding: 3px 0;
  font-size: 14px;
}

.variant-dialog .book-latest-prop strong {
  flex: 1 1 auto;
}

.variant-dialog .book-info-controls {
  justify-content: flex-end;
  margin: 2px 0 8px;
}

.variant-dialog .book-info-intro {
  max-height: calc(var(--vh, 1vh) * 70 - 54px - 60px - 150px - 75px - 120px);
  line-height: 1.6;
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

  .variant-dialog .book-cover-zone {
    justify-self: stretch;
    width: 100%;
    min-height: 150px;
  }

  .variant-dialog .book-info-intro {
    max-height: calc(var(--vh, 1vh) * 100 - 54px - 60px - 150px - 75px - 120px);
  }
}
</style>
