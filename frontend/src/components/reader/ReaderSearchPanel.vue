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
    <el-empty
      v-if="keyword && !loading && searched && !results.length"
      :description="hasMore ? '当前已搜索章节没有匹配，可继续搜索后续章节' : '没有匹配内容'"
    />
    <el-empty v-else-if="!keyword" description="输入关键词搜索整本书正文" />
  </div>
  <div v-if="keyword && searched" class="search-footer">
    <span>{{ statusText }}</span>
    <span class="search-actions">
      <el-button size="small" :loading="loading" :disabled="!hasMore" @click="$emit('loadMore')">
        {{ hasMore ? '继续搜索' : '没有更多' }}
      </el-button>
      <el-button v-if="hasMore" size="small" plain :loading="loading" @click="$emit('loadAll')">搜完全书</el-button>
    </span>
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

const emit = defineEmits(['update:modelValue', 'search', 'loadMore', 'loadAll', 'jump'])

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
  min-width: 0;
}

.content-search-row .el-input {
  min-width: 0;
  flex: 1;
}

.search-result-list {
  display: grid;
  gap: 10px;
  min-width: 0;
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
  gap: 8px;
  font-size: 12px;
  justify-content: space-between;
  margin-top: 12px;
}

.search-footer span {
  min-width: 0;
}

.search-actions {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 6px;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .content-search-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 8px;
    margin-bottom: 10px;
  }

  .content-search-row :deep(.el-input__wrapper),
  .content-search-row :deep(.el-button) {
    min-height: 38px;
  }

  .search-result-list {
    gap: 8px;
    padding-bottom: max(8px, env(safe-area-inset-bottom));
  }

  .search-result-item {
    padding: 9px;
  }

  .search-result-item span {
    display: -webkit-box;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
  }

  .search-footer {
    align-items: stretch;
    flex-direction: column;
  }

  .search-actions {
    display: grid;
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .search-footer :deep(.el-button) {
    min-height: 38px;
    width: 100%;
  }
}
</style>
