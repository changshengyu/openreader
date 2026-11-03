<template>
  <!-- reader-dev Reader.vue `.cache-content-zone`: one row, 缓存章节 then the three ranges, or the progress and a cancel icon. -->
  <section v-if="visible" class="reader-cache-zone" @click.stop>
    <span class="reader-cache-title">缓存章节</span>
    <div v-if="caching" class="reader-cache-status">
      <span>{{ statusText }}</span>
      <button class="reader-cache-cancel" type="button" title="取消缓存" aria-label="取消缓存" @click="$emit('cancel')">
        <el-icon><Close /></el-icon>
      </button>
    </div>
    <div v-else class="reader-cache-actions">
      <button type="button" @click="$emit('cache', 50)">后面50章</button>
      <button type="button" @click="$emit('cache', 100)">后面100章</button>
      <button type="button" @click="$emit('cache', true)">后面全部</button>
    </div>
  </section>
</template>

<script setup>
import { Close } from '@element-plus/icons-vue'

defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  caching: {
    type: Boolean,
    default: false,
  },
  statusText: {
    type: String,
    default: '',
  },
})

defineEmits(['cache', 'cancel'])
</script>

<style scoped>
.reader-cache-zone {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 36px;
  color: var(--reader-chrome-label, var(--reader-chrome-text, rgba(0, 0, 0, 0.4)));
  background: inherit;
  border: 0;
  box-shadow: none;
  font-size: 14px;
}

/* The wrappers only group the states; their children are the row's items, as in reader-dev. */
.reader-cache-actions,
.reader-cache-status {
  display: contents;
}

.reader-cache-actions button,
.reader-cache-status button {
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  line-height: normal;
}

.reader-cache-cancel {
  display: inline-flex;
  align-items: center;
}

.reader-cache-actions button:focus-visible,
.reader-cache-cancel:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 1px;
}
</style>
