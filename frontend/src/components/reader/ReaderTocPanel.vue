<template>
  <el-input v-model="keyword" placeholder="搜索章节..." clearable size="small" class="toc-search" />
  <div class="toc-list">
    <button
      v-for="item in filteredChapters"
      :key="item.id"
      class="toc-item"
      :class="{ active: item.index === currentIndex }"
      type="button"
      @click="$emit('jump', item.index)"
    >
      <span>{{ item.title }}</span>
      <small v-if="showMeta">第 {{ item.index + 1 }} 章 · {{ item.cachePath ? '已缓存' : '未缓存' }}</small>
      <el-tag v-else-if="item.cachePath" size="small" type="success" effect="plain">已缓存</el-tag>
    </button>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  chapters: {
    type: Array,
    default: () => [],
  },
  currentIndex: {
    type: Number,
    default: 0,
  },
  modelValue: {
    type: String,
    default: '',
  },
  reverse: {
    type: Boolean,
    default: false,
  },
  showMeta: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'jump'])

const keyword = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})

const filteredChapters = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  const list = value
    ? props.chapters.filter(chapter => chapter.title.toLowerCase().includes(value))
    : props.chapters
  return props.reverse ? [...list].reverse() : list
})
</script>

<style scoped>
.toc-search {
  margin-bottom: 12px;
}

.toc-list {
  max-height: calc(100vh - 160px);
  overflow-y: auto;
}

.toc-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  padding: 12px 8px;
  color: inherit;
  background: transparent;
  cursor: pointer;
  border: 0;
  border-bottom: 1px solid #f0f0f0;
  font-size: 14px;
  text-align: left;
}

.toc-item span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.toc-item small {
  flex: 0 0 auto;
  color: var(--app-text-muted, #909399);
  font-size: 12px;
}

.toc-item:hover {
  color: #409eff;
  background: #f5f7fa;
}

.toc-item.active {
  color: #409eff;
  font-weight: 600;
  background: #ecf5ff;
}
</style>
