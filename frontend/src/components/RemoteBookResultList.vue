<template>
  <div class="book-list wrapper remote-result-list">
    <article
      v-for="book in books"
      :key="bookKey(book)"
      class="book-row book remote-result-book"
      role="button"
      tabindex="0"
      @click="$emit('read', book)"
      @keyup.enter="$emit('read', book)"
    >
      <span class="cover-img" @click.stop="$emit('preview', book)">
        <BookCover class="list-cover" :book="book" />
      </span>
      <span class="list-main info">
        <span class="book-operation">
          <button class="operation-icon" type="button" title="编辑" @click.stop="$emit('edit', book)">
            <el-icon><Edit /></el-icon>
          </button>
        </span>
        <strong class="name edit">{{ remoteBookTitle(book) }}</strong>
        <span class="sub">
          <span class="author">{{ remoteBookAuthor(book) }}</span>
          <span v-if="remoteBookChapterCount(book)" class="dot">•</span>
          <span v-if="remoteBookChapterCount(book)" class="size">共{{ remoteBookChapterCount(book) }}章</span>
        </span>
        <span v-if="remoteBookLatestChapter(book)" class="last-chapter">
          {{ latestChapterLabel(book) }}：{{ remoteBookLatestChapter(book) }}
        </span>
        <span class="result-add-zone">
          <el-tag
            type="success"
            :effect="isNight ? 'dark' : 'light'"
            class="result-add-book setting-connect"
            :class="{ loading: addingBookKey === bookKey(book) }"
            role="button"
            tabindex="0"
            @click.stop="$emit('add', book)"
            @keydown.enter.prevent.stop="$emit('add', book)"
            @keydown.space.prevent.stop="$emit('add', book)"
          >加入书架</el-tag>
        </span>
      </span>
    </article>
  </div>
</template>

<script setup>
import { Edit } from '@element-plus/icons-vue'
import BookCover from './BookCover.vue'
import {
  remoteBookAuthor,
  remoteBookChapterCount,
  remoteBookKey,
  remoteBookLastCheckTime,
  remoteBookLatestChapter,
  remoteBookTitle,
} from '../utils/remoteBookResult.js'
import { relativeShelfTimeLabel } from '../utils/shelfPresentation.js'

const props = defineProps({
  books: { type: Array, default: () => [] },
  addingBookKey: { type: String, default: '' },
  fallbackSourceId: { type: [String, Number], default: '' },
  isNight: { type: Boolean, default: false },
})

defineEmits(['preview', 'read', 'add', 'edit'])

function bookKey(book) {
  return remoteBookKey(book, props.fallbackSourceId)
}

function latestChapterLabel(book) {
  const rawTime = remoteBookLastCheckTime(book)
  return rawTime ? relativeShelfTimeLabel(rawTime) : '最新'
}
</script>
