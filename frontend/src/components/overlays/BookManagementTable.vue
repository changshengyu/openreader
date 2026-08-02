<template>
  <el-table
    ref="tableRef"
    :data="books"
    row-key="id"
    :height="isMobile ? 'calc(100dvh - 226px)' : 'min(358px, calc(70dvh - 226px))'"
    class="book-manage-table"
    @selection-change="rows => emit('selection-change', rows)"
  >
    <el-table-column
      type="selection"
      width="25"
      :fixed="isMobile"
      reserve-selection
    />
    <el-table-column
      prop="title"
      label="书名名"
      min-width="100"
      :fixed="isMobile"
    >
      <template #default="{ row }">
        <el-button
          text
          class="text-button"
          @click="emit('open-info', row)"
        >
          {{ row.title }}
        </el-button>
      </template>
    </el-table-column>
    <el-table-column
      prop="author"
      label="作者"
      min-width="100"
    />
    <el-table-column label="分组" min-width="120">
      <template #default="{ row }">{{ categoryName(row) }}</template>
    </el-table-column>
    <el-table-column label="章节" min-width="120">
      <template #default="{ row }">
        <span>共 {{ row.chapterCount || 0 }} 章</span><br>
        <template v-if="Number(row.sourceId || 0) > 0">
          <span>服务器缓存： {{ serverCacheCount(row) }} 章</span><br>
        </template>
        <span>浏览器缓存： {{ localCacheCount(row) }} 章</span>
      </template>
    </el-table-column>
    <el-table-column label="操作" width="100px">
      <template #default="{ row }">
        <BookManagementActions
          :book="row"
          :caching="isCachingBook(row)"
          @edit="emit('open-edit', row)"
          @group="emit('set-group', row)"
          @cache="command => emit('cache', row, command)"
          @export="format => emit('export', row, format)"
        />
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup>
import { ref } from 'vue'
import BookManagementActions from './BookManagementActions.vue'

defineProps({
  books: {
    type: Array,
    default: () => [],
  },
  isMobile: {
    type: Boolean,
    default: false,
  },
  isCachingBook: {
    type: Function,
    required: true,
  },
  categoryName: {
    type: Function,
    required: true,
  },
  serverCacheCount: {
    type: Function,
    required: true,
  },
  localCacheCount: {
    type: Function,
    required: true,
  },
})

const emit = defineEmits([
  'selection-change',
  'open-info',
  'open-edit',
  'set-group',
  'cache',
  'export',
])
const tableRef = ref(null)

function clearSelection() {
  tableRef.value?.clearSelection()
}

defineExpose({ clearSelection })
</script>

<style scoped>
.text-button {
  padding: 3px 5px;
}
</style>
