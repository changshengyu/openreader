<template>
  <section class="book-info-shared">
    <div class="book-cover-zone" :class="{ editable: coverEditable, uploading: coverUploading }" @click="triggerCoverUpload">
      <div class="book-cover-bg" :style="coverBgStyle" />
      <BookCover :book="book" />
      <span v-if="coverEditable" class="cover-edit-label">{{ coverUploading ? '上传中' : '更换封面' }}</span>
      <input
        v-if="coverEditable"
        ref="coverInput"
        type="file"
        accept="image/*"
        class="cover-file-input"
        @change="handleCoverFileChange"
      />
    </div>
    <div class="book-info-main">
      <div class="book-info-title">
        <h2>{{ book?.title || '未命名书籍' }}</h2>
        <el-tag v-if="statusLabel" size="small" effect="plain" :type="statusType">{{ statusLabel }}</el-tag>
      </div>
      <div v-if="bookKindTags.length" class="book-kind-tags">
        <span v-for="tag in bookKindTags" :key="tag">{{ tag }}</span>
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
          <strong>{{ latestChapterLabel }}</strong>
        </div>
        <div>
          <span>分组：</span>
          <strong>{{ categoryName || '未分组' }}</strong>
          <button v-if="showCategoryAction" type="button" class="book-prop-action" @click="emit('category-action')">
            {{ categoryActionLabel }}
          </button>
        </div>
        <div>
          <span>章节：</span>
          <strong>{{ chapterCount }}</strong>
        </div>
        <div>
          <span>进度：</span>
          <strong>{{ progressLabel }}</strong>
        </div>
        <div v-if="browserCacheCount >= 0">
          <span>浏览器缓存：</span>
          <strong>{{ browserCacheCount }} 章</strong>
        </div>
      </div>
      <div v-if="showUpdateSwitch" class="book-info-controls">
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
  showCategoryAction: {
    type: Boolean,
    default: false,
  },
  categoryActionLabel: {
    type: String,
    default: '设置分组',
  },
})

const emit = defineEmits(['cover-upload', 'can-update-change', 'category-action'])
const coverInput = ref(null)

const chapterCount = computed(() => Array.isArray(props.chapters) ? props.chapters.length : (props.chapters || props.book?.chapterCount || 0))
const latestChapterLabel = computed(() => props.book?.lastChapter || props.book?.latestChapter || props.book?.latestChapterTitle || '-')
const progressLabel = computed(() => `${Math.round(Math.max(0, Math.min(1, props.progress || 0)) * 100)}%`)
const canUpdateValue = computed(() => props.book?.canUpdate !== false && props.canUpdate !== false)
const bookKindTags = computed(() => {
  const raw = props.book?.kind ?? props.book?.category ?? props.book?.categoryName ?? props.book?.genre ?? props.book?.tags
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
