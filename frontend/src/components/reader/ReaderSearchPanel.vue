<template>
  <div class="content-search-row">
    <el-input v-model="keyword" placeholder="搜索整本书..." clearable size="small" @keyup.enter="$emit('search')" />
    <el-button size="small" type="primary" :loading="loading" @click="$emit('search')">搜索</el-button>
  </div>
  <div class="search-result-list">
    <button
      v-for="result in results"
      :key="`${result.chapterIndex}-${result.offset}`"
      class="search-result-item"
      type="button"
      @click="$emit('jump', result)"
    >
      <strong>{{ result.chapterTitle || `第 ${result.chapterIndex + 1} 章` }}</strong>
      <span>{{ result.excerpt }}</span>
    </button>
    <el-empty v-if="keyword && !loading && searched && !results.length" description="没有匹配内容" />
    <el-empty v-else-if="!keyword" description="输入关键词搜索整本书正文" />
  </div>
  <div v-if="keyword && searched" class="search-footer">
    <span>{{ statusText }}</span>
    <el-button size="small" :loading="loading" :disabled="!hasMore" @click="$emit('loadMore')">
      {{ hasMore ? '加载更多' : '没有更多' }}
    </el-button>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: '',
  },
  results: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
  searched: {
    type: Boolean,
    default: false,
  },
  hasMore: {
    type: Boolean,
    default: false,
  },
  statusText: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['update:modelValue', 'search', 'loadMore', 'jump'])

const keyword = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})
</script>

<style scoped>
.content-search-row {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.content-search-row .el-input {
  flex: 1;
}

.search-result-list {
  display: grid;
  gap: 10px;
}

.search-result-item {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 10px;
  color: #24282c;
  text-align: left;
  background: #fffaf0;
  border: 1px solid #eee4c9;
  border-radius: 6px;
  cursor: pointer;
}

.search-result-item strong {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-result-item span {
  color: #6f6754;
  font-size: 13px;
  line-height: 1.5;
}

.search-result-item:hover {
  color: #0f5451;
}

.search-footer {
  align-items: center;
  color: #7b715e;
  display: flex;
  font-size: 12px;
  justify-content: space-between;
  margin-top: 12px;
}
</style>
