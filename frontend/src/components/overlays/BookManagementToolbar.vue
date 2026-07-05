<template>
  <div class="manage-head">
    <el-input
      :model-value="modelValue"
      placeholder="搜索书名、作者或文件名"
      clearable
      size="small"
      @update:model-value="value => emit('update:modelValue', value)"
    />
    <div class="manage-head-actions">
      <el-button size="small" text @click="emit('select-all')">
        全选
      </el-button>
      <el-button size="small" text @click="emit('clear-selection')">
        清空
      </el-button>
    </div>
  </div>
</template>

<script setup>
defineProps({
  modelValue: {
    type: String,
    default: '',
  },
})

const emit = defineEmits([
  'update:modelValue',
  'select-all',
  'clear-selection',
])
</script>

<style scoped>
.manage-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.manage-head-actions {
  display: none;
  flex: 0 0 auto;
  gap: 6px;
}

@media (max-width: 750px) {
  .manage-head {
    grid-template-columns: 1fr;
  }

  .manage-head-actions {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
