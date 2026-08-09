<template>
  <el-dialog
    :model-value="modelValue"
    title="导入RSS源"
    width="min(1000px, max(750px, 70vw))"
    :fullscreen="isMobile"
    class="rss-source-import-dialog"
    append-to-body
    @update:model-value="handleVisibleChange"
  >
    <div class="rss-import-source-list">
      <el-checkbox-group :model-value="selected" @update:model-value="$emit('update:selected', $event)">
        <el-checkbox
          v-for="(source, index) in sources"
          :key="index"
          :value="index"
          class="rss-source-checkbox"
        >
          {{ source.sourceName }} {{ source.sourceUrl }} {{ riskText(source) }}
        </el-checkbox>
      </el-checkbox-group>
    </div>
    <template #footer>
      <div class="rss-import-footer">
        <el-checkbox
          :model-value="checkAll"
          :indeterminate="indeterminate"
          border
          @update:model-value="$emit('check-all', $event)"
        >全选</el-checkbox>
        <span class="rss-import-count">已选择 {{ selected.length }} 个</span>
        <span class="rss-import-spacer" />
        <el-button @click="$emit('cancel')">取消</el-button>
        <el-button type="primary" :loading="saving" @click="$emit('confirm')">确定</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { rssSourceRiskTags } from '../../utils/rssSourceImport'

defineProps({
  modelValue: Boolean,
  sources: {
    type: Array,
    default: () => [],
  },
  selected: {
    type: Array,
    default: () => [],
  },
  checkAll: Boolean,
  indeterminate: Boolean,
  isMobile: Boolean,
  saving: Boolean,
})

const emit = defineEmits([
  'update:modelValue',
  'update:selected',
  'check-all',
  'cancel',
  'confirm',
])

function handleVisibleChange(visible) {
  emit('update:modelValue', visible)
  if (!visible) emit('cancel')
}

function riskText(source) {
  return rssSourceRiskTags(source).join('  ')
}
</script>

<style scoped>
.rss-import-source-list {
  max-height: calc(var(--vh, 1vh) * 70 - 114px);
  overflow-y: auto;
}

.rss-source-checkbox {
  display: flex;
  margin: 0;
  padding: 8px 4px;
  white-space: normal;
}

.rss-import-footer {
  display: flex;
  align-items: center;
  gap: 10px;
}

.rss-import-count {
  font-size: 14px;
}

.rss-import-spacer {
  flex: 1;
}

@media (max-width: 750px) {
  .rss-import-source-list {
    max-height: calc(var(--vh, 1vh) * 100 - 114px);
  }

  .rss-import-footer {
    gap: 6px;
  }
}
</style>
