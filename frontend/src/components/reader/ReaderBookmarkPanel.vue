<template>
  <div class="drawer-actions">
    <el-button v-if="showAdd" type="primary" size="small" @click="$emit('add')">添加书签</el-button>
  </div>
  <div class="bookmark-list">
    <div v-for="item in bookmarks" :key="item.id" class="bookmark-card">
      <button class="bookmark-main" type="button" @click="$emit('jump', item)">
        <strong>{{ item.title || '书签' }}</strong>
        <span>{{ item.excerpt || item.note || '无摘录' }}</span>
        <small>第 {{ item.chapterIndex + 1 }} 章 · {{ Math.round((item.percent || 0) * 100) }}%</small>
      </button>
      <span class="bookmark-actions">
        <el-button v-if="showEdit" text size="small" @click="$emit('edit', item)">编辑</el-button>
        <el-button text type="danger" size="small" @click="$emit('remove', item)">删除</el-button>
      </span>
    </div>
    <el-empty v-if="!bookmarks.length" description="暂无书签" />
  </div>
</template>

<script setup>
defineProps({
  bookmarks: {
    type: Array,
    default: () => [],
  },
  showAdd: {
    type: Boolean,
    default: true,
  },
  showEdit: {
    type: Boolean,
    default: true,
  },
})

defineEmits(['add', 'jump', 'edit', 'remove'])
</script>

<style scoped>
.drawer-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 14px;
  min-width: 0;
}

.bookmark-list {
  display: grid;
  gap: 10px;
  min-width: 0;
}

.bookmark-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: start;
  padding: 10px;
  border: 1px solid #eee4c9;
  border-radius: 6px;
  background: #fffaf0;
}

.bookmark-main {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 0;
  color: #24282c;
  text-align: left;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.bookmark-main strong {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bookmark-main span {
  color: #6f6754;
  font-size: 13px;
  line-height: 1.5;
}

.bookmark-main small {
  color: #9a8e72;
  font-size: 12px;
}

.bookmark-main:hover strong {
  color: #0f5451;
}

.bookmark-actions {
  display: grid;
  gap: 2px;
}

@media (max-width: 1180px), (hover: none) and (pointer: coarse), (any-pointer: coarse) {
  .drawer-actions {
    margin-bottom: 10px;
  }

  .drawer-actions :deep(.el-button) {
    min-height: 38px;
  }

  .bookmark-list {
    gap: 8px;
    padding-bottom: max(8px, env(safe-area-inset-bottom));
  }

  .bookmark-card {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
    padding: 9px;
  }

  .bookmark-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }

  .bookmark-actions :deep(.el-button) {
    min-height: 32px;
    margin: 0;
  }
}
</style>
