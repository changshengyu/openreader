<template>
  <el-dialog
    :model-value="modelValue"
    :title="title"
    width="500px"
    :fullscreen="isMobile"
    class="rss-article-list-dialog"
    append-to-body
    @update:model-value="handleVisibleChange"
  >
    <el-tabs
      v-if="sortOptions.length > 1"
      :model-value="sortName"
      @tab-change="$emit('sort-change', $event)"
    >
      <el-tab-pane
        v-for="sort in sortOptions"
        :key="sort.name"
        :label="sort.name"
        :name="sort.name"
      />
    </el-tabs>
    <div class="rss-article-list-container">
      <div
        v-for="(article, index) in articles"
        :key="article.id || `${article.link}-${index}`"
        class="rss-article"
        role="button"
        tabindex="0"
        @click="$emit('open-article', article)"
        @keydown.enter.prevent="$emit('open-article', article)"
      >
        <div class="rss-article-info">
          <div class="rss-article-title">{{ article.title }}</div>
          <div class="rss-article-date">{{ article.pubDate }}</div>
        </div>
        <div v-if="article.image" class="rss-article-image" @click.stop>
          <div class="image-wrapper">
            <el-image
              class="rss-article-img"
              :src="article.image"
              :preview-src-list="articleImages"
              fit="cover"
              lazy
            />
          </div>
        </div>
      </div>
      <div
        class="load-more-rss"
        :class="{ disabled: loading || loadingMore || !hasMore }"
        @click="handleLoadMore"
      >{{ hasMore ? '加载更多' : '没有更多啦' }}</div>
    </div>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  modelValue: Boolean,
  title: {
    type: String,
    default: '',
  },
  isMobile: Boolean,
  sortOptions: {
    type: Array,
    default: () => [],
  },
  sortName: {
    type: String,
    default: '',
  },
  articles: {
    type: Array,
    default: () => [],
  },
  loading: Boolean,
  loadingMore: Boolean,
  hasMore: Boolean,
})

const emit = defineEmits(['update:modelValue', 'close', 'sort-change', 'open-article', 'load-more'])
const articleImages = computed(() => props.articles.map(article => article.image).filter(Boolean))

function handleVisibleChange(visible) {
  emit('update:modelValue', visible)
  if (!visible) emit('close')
}

function handleLoadMore() {
  if (!props.loading && !props.loadingMore && props.hasMore) emit('load-more')
}
</script>

<style scoped>
.rss-article-list-container {
  max-height: calc(var(--vh, 1vh) * 70 - 114px);
  overflow-y: auto;
}

.rss-article {
  display: flex;
  flex-direction: row;
  padding: 15px 10px;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
}

.rss-article-info {
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: space-between;
  min-width: 0;
  padding-right: 5px;
}

.rss-article-title {
  font-size: 14px;
  font-weight: 600;
}

.rss-article-date {
  margin-top: 10px;
  color: var(--app-text-muted);
  font-size: 12px;
}

.rss-article-image {
  display: flex;
  align-items: center;
  width: 120px;
}

.image-wrapper {
  width: 100%;
  height: 0;
  padding-bottom: 62.5%;
  overflow: hidden;
}

.rss-article-img {
  width: 120px;
  height: 75px;
}

.load-more-rss {
  padding: 10px;
  text-align: center;
  cursor: pointer;
}

.load-more-rss.disabled {
  cursor: default;
}

@media (max-width: 750px) {
  .rss-article-list-container {
    max-height: calc(var(--vh, 1vh) * 100 - 94px);
  }

  .rss-article {
    padding: 15px 5px;
  }

  .rss-article-image {
    width: 100px;
  }

  .rss-article-img {
    width: 100px;
    height: 62.5px;
  }
}
</style>
