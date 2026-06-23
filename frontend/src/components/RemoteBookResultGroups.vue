<template>
  <div class="source-result-list">
    <section v-for="group in groups" :key="group.sourceId || group.sourceName" class="source-result-group">
      <header class="source-result-head">
        <h2>{{ group.sourceName || '未知书源' }}</h2>
        <el-tag effect="plain">{{ group.items?.length || 0 }} 条</el-tag>
      </header>
      <div class="result-list">
        <article
          v-for="item in group.items || []"
          :key="bookKey(item, group)"
          class="result-card app-panel"
          @click="$emit('preview', item)"
        >
          <BookCover :book="item" />
          <div class="result-main">
            <div class="result-title">
              <h3>{{ remoteBookTitle(item) }}</h3>
              <el-tag size="small" effect="plain">{{ remoteBookSourceName(item, group.sourceName) }}</el-tag>
            </div>
            <p>{{ remoteBookAuthor(item) || '未知作者' }}</p>
            <p v-if="remoteBookLatestChapter(item)" class="latest-chapter">{{ remoteBookLatestChapter(item) }}</p>
            <p v-if="remoteBookKind(item) || remoteBookWordCount(item)">{{ [remoteBookKind(item), remoteBookWordCount(item)].filter(Boolean).join(' · ') }}</p>
            <p class="result-intro">{{ remoteBookIntro(item) || '暂无简介' }}</p>
          </div>
          <div class="result-actions" @click.stop>
            <el-button type="primary" size="small" @click="$emit('preview', item)">查看信息</el-button>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup>
import BookCover from './BookCover.vue'
import {
  remoteBookAuthor,
  remoteBookIntro,
  remoteBookKind,
  remoteBookKey,
  remoteBookLatestChapter,
  remoteBookSourceName,
  remoteBookTitle,
  remoteBookWordCount,
} from '../utils/remoteBookResult'

defineProps({
  groups: { type: Array, default: () => [] },
})

defineEmits(['preview'])

function bookKey(item, group) {
  return remoteBookKey(item, group?.sourceId)
}
</script>

<style scoped>
.source-result-list {
  display: grid;
  gap: 14px;
}

.source-result-group {
  display: grid;
  gap: 10px;
}

.source-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.source-result-head h2 {
  margin: 0;
  font-size: 18px;
}

.result-list {
  display: grid;
  gap: 12px;
}

.result-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 14px;
  align-items: center;
  padding: 14px;
  cursor: pointer;
}

.result-main,
.result-title {
  min-width: 0;
}

.result-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.result-title h3 {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  font-size: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-main p {
  margin: 4px 0 0;
  color: var(--app-text-muted);
}

.latest-chapter {
  color: var(--app-accent) !important;
}

.result-intro {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.result-actions {
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 750px) {
  .source-result-list {
    gap: 10px;
  }

  .source-result-head h2 {
    font-size: 16px;
  }

  .result-card {
    grid-template-columns: 42px minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
  }

  .result-actions {
    grid-column: 2;
    justify-content: flex-start;
  }

  .result-actions :deep(.el-button) {
    min-height: 32px;
  }

  .result-title h3 {
    font-size: 15px;
  }

  .result-main p {
    font-size: 12px;
  }
}
</style>
