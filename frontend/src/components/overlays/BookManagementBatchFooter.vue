<template>
  <div class="manage-footer">
    <div class="manage-footer-left">
      <el-button
        type="primary"
        size="default"
        :loading="busy"
        @click="emit('delete-selected')"
      >
        批量删除
      </el-button>
      <el-dropdown @command="category => emit('add-category', category)">
        <el-button type="primary" size="default" :loading="busy">
          批量添加分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
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
        <el-button type="primary" size="default" :loading="busy">
          批量移除分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
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
    </div>
    <el-button size="default" @click="emit('close')">取消</el-button>
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
  'close',
])
</script>

<style scoped>
.manage-footer,
.manage-footer-left {
  display: flex;
  align-items: center;
}

.manage-footer {
  min-width: 0;
  justify-content: space-between;
  gap: 10px;
}

.manage-footer-left {
  min-width: 0;
  flex-wrap: wrap;
  gap: 5px;
}

.manage-footer-left :deep(.el-button) {
  margin-left: 0;
}

.check-tip {
  margin-left: 5px;
  color: var(--app-text-muted);
  font-size: 13px;
  white-space: nowrap;
}

@media (max-width: 750px) {
  .manage-footer {
    align-items: flex-end;
  }

  .manage-footer-left {
    align-items: flex-start;
  }
}
</style>
