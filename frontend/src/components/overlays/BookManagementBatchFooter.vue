<template>
  <div class="manage-footer">
    <el-button
      type="primary"
      :disabled="!selectedCount"
      :loading="busy"
      @click="emit('delete-selected')"
    >
      批量删除
    </el-button>
    <el-dropdown @command="category => emit('add-category', category)">
      <el-button type="primary" :disabled="!selectedCount" :loading="busy">
        批量添加分组
        <el-icon class="el-icon--right"><ArrowDown /></el-icon>
      </el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item
            v-for="category in categories"
            :key="category.id"
            :command="category"
          >
            {{ category.name }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
    <el-dropdown @command="category => emit('remove-category', category)">
      <el-button type="primary" :disabled="!selectedCount" :loading="busy">
        批量移除分组
        <el-icon class="el-icon--right"><ArrowDown /></el-icon>
      </el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item
            v-for="category in categories"
            :key="category.id"
            :command="category"
          >
            {{ category.name }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
    <span class="check-tip">已选择 {{ selectedCount }} 个</span>
    <el-dropdown @command="command => emit('more-command', command)">
      <el-button :disabled="!selectedCount" :loading="busy">
        更多批量操作
        <el-icon class="el-icon--right"><ArrowDown /></el-icon>
      </el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="cache">
            批量缓存到服务器
          </el-dropdown-item>
          <el-dropdown-item command="clear-cache">
            批量清服务器缓存
          </el-dropdown-item>
          <el-dropdown-item command="export">
            批量导出
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
    <el-button @click="emit('close')">取消</el-button>
  </div>
</template>

<script setup>
import { ArrowDown } from '@element-plus/icons-vue'

defineProps({
  categories: {
    type: Array,
    default: () => [],
  },
  selectedCount: {
    type: Number,
    default: 0,
  },
  busy: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits([
  'delete-selected',
  'add-category',
  'remove-category',
  'more-command',
  'close',
])
</script>

<style scoped>
.manage-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--app-border);
}

.check-tip {
  color: var(--app-text-muted);
  font-size: 13px;
}

@media (max-width: 750px) {
  .manage-footer {
    align-items: stretch;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .manage-footer :deep(.el-button),
  .manage-footer :deep(.el-dropdown),
  .manage-footer :deep(.el-dropdown .el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .manage-footer .check-tip {
    grid-column: 1 / -1;
    order: -1;
  }
}
</style>
