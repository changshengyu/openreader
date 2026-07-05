<template>
  <el-table
    :data="books"
    row-key="id"
    height="calc(100vh - 188px)"
    class="manage-table desktop-manage-table"
    @selection-change="rows => emit('selection-change', rows)"
  >
    <el-table-column type="selection" width="42" />
    <el-table-column
      prop="title"
      label="书名"
      min-width="180"
      show-overflow-tooltip
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
      min-width="120"
      show-overflow-tooltip
    />
    <el-table-column label="分组" min-width="120">
      <template #default="{ row }">{{ categoryName(row) }}</template>
    </el-table-column>
    <el-table-column label="章节" min-width="150">
      <template #default="{ row }">
        <span>共 {{ row.chapterCount || 0 }} 章</span><br>
        <span>阅读进度：{{ progressLabel(row) }}</span>
        <template v-if="Number(row.sourceId || 0) > 0">
          <br><span>服务器缓存：{{ serverCacheCount(row) }} 章</span>
        </template>
        <br><span>浏览器缓存：{{ localCacheCount(row) }} 章</span>
      </template>
    </el-table-column>
    <el-table-column label="操作" width="150" fixed="right">
      <template #default="{ row }">
        <BookManagementActions
          :book="row"
          :caching="cachingBookId === row.id"
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
import BookManagementActions from './BookManagementActions.vue'

defineProps({
  books: {
    type: Array,
    default: () => [],
  },
  cachingBookId: {
    type: [String, Number],
    default: null,
  },
  categoryName: {
    type: Function,
    required: true,
  },
  progressLabel: {
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
</script>

<style scoped>
.manage-table {
  margin-bottom: 12px;
}

.text-button {
  padding: 0;
}

@media (max-width: 750px) {
  .desktop-manage-table {
    display: none;
  }
}
</style>
