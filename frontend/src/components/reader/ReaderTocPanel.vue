<template>
  <el-input v-model="keyword" placeholder="搜索章节..." clearable size="small" class="toc-search" />
  <div class="toc-list">
    <div
      v-for="item in filteredChapters"
      :key="item.id"
      class="toc-item"
      :class="{ active: item.index === currentIndex }"
      @click="$emit('jump', item.index)"
    >
      <span>{{ item.title }}</span>
      <el-tag v-if="item.cachePath" size="small" type="success" effect="plain">已缓存</el-tag>
    </div>
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
})

const emit = defineEmits(['update:modelValue', 'jump'])

const keyword = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})

const filteredChapters = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  if (!value) return props.chapters
  return props.chapters.filter(chapter => chapter.title.toLowerCase().includes(value))
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
  padding: 12px 8px;
  cursor: pointer;
  border-bottom: 1px solid #f0f0f0;
  font-size: 14px;
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
