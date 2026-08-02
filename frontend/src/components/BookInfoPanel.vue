<template>
  <section class="book-info-container">
    <div class="book-cover">
      <div class="book-cover-bg" :style="coverBgStyle" />
      <div
        class="book-cover-bg-image"
        :class="{ editable: coverEditable, uploading: coverUploading }"
        @click="triggerCoverUpload"
      >
        <BookCover :book="book" size="book-info" />
        <input
          v-if="coverEditable"
          ref="coverInput"
          type="file"
          accept="image/jpg,image/png,image/jpeg"
          class="cover-file-input"
          @change="handleCoverFileChange"
        />
      </div>
    </div>

    <div class="book-name">{{ bookTitle }}</div>

    <div v-if="bookKindTags.length" class="book-kind">
      <span v-for="(tag, index) in bookKindTags" :key="`${index}-${tag}`">{{ tag }}</span>
    </div>

    <div class="book-props">
      <div class="book-prop book-author">作者： {{ book?.author || '未知' }}</div>

      <div class="book-prop book-origin">
        来源： {{ sourceName }}
        <button
          v-if="showLocalRefreshAction"
          type="button"
          class="book-prop-btn"
          :disabled="localRefreshLoading"
          @click="emit('local-refresh')"
        >{{ localRefreshLoading ? '更新中' : '更新' }}</button>
      </div>

      <div class="book-prop book-latest">
        <span class="latest-title">最新： {{ latestChapterLabel }}</span>
        <span v-if="inShelf" class="book-prop-btn inline-update-switch">
          追更
          <el-switch
            :model-value="canUpdateValue"
            :loading="updateSwitchLoading"
            active-color="#13ce66"
            inactive-color="#ff4949"
            @change="value => emit('can-update-change', value)"
          />
        </span>
      </div>

      <div v-if="inShelf" class="book-prop book-group">
        分组： {{ categoryName || '未分组' }}
        <button type="button" class="book-prop-btn" @click="emit('category-action')">设置分组</button>
      </div>

      <div v-else-if="showAddAction" class="book-prop book-operate-zone">
        <el-tag
          type="success"
          :effect="isNight ? 'dark' : 'light'"
          class="book-operate-btn"
          :class="{ loading: addLoading }"
          role="button"
          tabindex="0"
          @click.stop="emitAdd"
          @keydown.enter.prevent="emitAdd"
          @keydown.space.prevent="emitAdd"
        >加入书架</el-tag>
      </div>
    </div>

    <div class="book-intro">
      <p v-for="(paragraph, index) in introParagraphs" :key="index">{{ paragraph }}</p>
    </div>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import BookCover from './BookCover.vue'
import { bookCoverUrl } from '../utils/bookCover'

const props = defineProps({
  book: { type: Object, default: () => ({}) },
  sourceName: { type: String, default: '未知书源' },
  categoryName: { type: String, default: '' },
  coverEditable: { type: Boolean, default: false },
  coverUploading: { type: Boolean, default: false },
  canUpdate: { type: Boolean, default: true },
  updateSwitchLoading: { type: Boolean, default: false },
  inShelf: { type: Boolean, default: true },
  showLocalRefreshAction: { type: Boolean, default: false },
  localRefreshLoading: { type: Boolean, default: false },
  showAddAction: { type: Boolean, default: false },
  addLoading: { type: Boolean, default: false },
  isNight: { type: Boolean, default: false },
})

const emit = defineEmits(['cover-upload', 'can-update-change', 'category-action', 'local-refresh', 'add'])
const coverInput = ref(null)

const bookTitle = computed(() => props.book?.title || props.book?.name || props.book?.bookName || '')
const latestChapterLabel = computed(() => (
  props.book?.lastChapter ||
  props.book?.latestChapter ||
  props.book?.latestChapterTitle ||
  props.book?.durChapterTitle ||
  ''
))
const canUpdateValue = computed(() => props.book?.canUpdate !== false && props.canUpdate !== false)
const bookKindTags = computed(() => normalizeKindTags(
  props.book?.kind ?? props.book?.category ?? props.book?.categoryName ?? '',
))
const introParagraphs = computed(() => (
  String(props.book?.intro || '暂无简介')
    .split('\n')
    .map(line => `\u00a0\u00a0\u00a0\u00a0\u00a0\u00a0${line.replace(/^\s+/g, '')}`)
))
const coverBgStyle = computed(() => {
  const url = bookCoverUrl(props.book)
  return url ? { backgroundImage: `url(${url})` } : {}
})

function normalizeKindTags(value) {
  return String(value || '').split(',').filter(item => item)
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

function emitAdd() {
  if (props.addLoading) return
  emit('add')
}
</script>

<style scoped>
.book-info-container .book-cover {
  position: relative;
  width: 100%;
  height: 150px;
}

.book-cover-bg,
.book-cover-bg-image {
  position: absolute;
  width: 100%;
  height: 100%;
}

.book-cover-bg {
  background-position: center;
  background-size: cover;
  filter: blur(50px);
}

.book-cover-bg-image {
  display: flex;
  align-items: center;
  justify-content: center;
}

.book-cover-bg-image.editable {
  cursor: pointer;
}

.book-cover-bg-image.uploading {
  cursor: progress;
}

.book-cover-bg-image :deep(.book-cover-shared) {
  display: inline-grid;
  margin: 0 auto;
}

.cover-file-input {
  display: none;
}

.book-name {
  display: block;
  padding: 10px 0;
  color: var(--app-text);
  font-size: 16px;
  font-weight: 500;
  text-align: center;
}

.book-kind {
  display: block;
  padding: 5px 0;
  color: red;
  text-align: center;
}

.book-kind span + span {
  margin-left: 0;
}

.book-props {
  padding: 5px 0;
}

.book-prop {
  min-width: 0;
  padding: 3px 0;
  color: var(--app-text);
}

.book-latest {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
}

.latest-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.book-prop-btn {
  float: right;
  height: 19px;
  padding: 0;
  color: var(--el-color-primary);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.book-prop-btn:disabled {
  cursor: progress;
  opacity: 0.65;
}

.inline-update-switch {
  display: inline-flex;
  width: 80px;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  color: var(--app-text);
  text-align: right;
  white-space: nowrap;
}

.book-operate-zone {
  text-align: center;
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

.book-intro {
  max-height: calc(var(--vh, 1vh) * 70 - 54px - 60px - 150px - 75px - 120px);
  overflow-y: auto;
  color: var(--app-text);
  line-height: 1.6;
}

.book-intro p {
  margin: 1em 0;
}

:global(html.dark-reader) .book-name {
  color: #eee !important;
}

@media (max-width: 750px) {
  .book-intro {
    max-height: calc(var(--vh, 1vh) * 100 - 54px - 60px - 150px - 75px - 120px);
  }
}
</style>
