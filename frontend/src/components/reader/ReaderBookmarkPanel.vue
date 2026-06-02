<template>
  <div class="drawer-actions">
    <el-button v-if="showAdd" type="primary" size="small" @click="$emit('add')">添加书签</el-button>
    <el-button size="small" @click="pickImportFile">导入</el-button>
    <el-button size="small" type="danger" plain :disabled="!selectedBookmarks.length" @click="$emit('removeMany', selectedBookmarks)">批量删除</el-button>
    <span class="selection-count">已选择 {{ selectedBookmarks.length }} 个</span>
    <input ref="fileRef" class="bookmark-file-input" type="file" accept=".json,application/json" @change="onImportFileChange" />
  </div>
  <div class="bookmark-list">
    <div v-for="item in bookmarks" :key="item.id" class="bookmark-card">
      <el-checkbox v-model="selectedIds" :value="item.id" class="bookmark-check" />
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
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

const props = defineProps({
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

const emit = defineEmits(['add', 'jump', 'edit', 'remove', 'removeMany', 'import'])
const selectedIds = ref([])
const fileRef = ref(null)

const selectedBookmarks = computed(() => {
  const ids = new Set(selectedIds.value)
  return props.bookmarks.filter(item => ids.has(item.id))
})

watch(
  () => props.bookmarks.map(item => item.id).join(','),
  () => {
    const ids = new Set(props.bookmarks.map(item => item.id))
    selectedIds.value = selectedIds.value.filter(id => ids.has(id))
  },
)

function pickImportFile() {
  fileRef.value?.click()
}

function onImportFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const rows = JSON.parse(String(reader.result || '[]'))
      if (!Array.isArray(rows) || !rows.length) {
        ElMessage.error('书签文件错误')
        return
      }
      emit('import', rows)
    } catch {
      ElMessage.error('书签文件错误')
    }
  }
  reader.onerror = () => ElMessage.error('读取书签文件失败')
  reader.readAsText(file)
}
</script>

<style scoped>
.drawer-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 14px;
  min-width: 0;
  align-items: center;
  flex-wrap: wrap;
}

.bookmark-file-input {
  display: none;
}

.selection-count {
  color: var(--app-text-muted, #909399);
  font-size: 12px;
}

.bookmark-list {
  display: grid;
  gap: 10px;
  min-width: 0;
}

.bookmark-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: start;
  padding: 10px;
  border: 1px solid #eee4c9;
  border-radius: 6px;
  background: #fffaf0;
}

.bookmark-check {
  padding-top: 2px;
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

@media (max-width: 750px) {
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
    grid-template-columns: auto minmax(0, 1fr);
    gap: 8px;
    padding: 9px;
  }

  .bookmark-actions {
    display: flex;
    grid-column: 2;
    justify-content: flex-end;
    gap: 8px;
  }

  .bookmark-actions :deep(.el-button) {
    min-height: 32px;
    margin: 0;
  }
}
</style>
