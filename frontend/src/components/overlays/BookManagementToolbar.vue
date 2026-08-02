<template>
  <el-input
    :model-value="modelValue"
    placeholder="搜索书名或作者"
    clearable
    size="small"
    class="search-input"
    :prefix-icon="Search"
    @update:model-value="value => emit('update:modelValue', value)"
  />
</template>

<script setup>
import { Search } from '@element-plus/icons-vue'

defineProps({
  modelValue: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['update:modelValue'])
</script>

<style scoped>
.search-input {
  margin-bottom: 10px;
}
</style>
