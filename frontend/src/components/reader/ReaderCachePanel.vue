<template>
  <section v-if="visible" class="reader-cache-zone" @click.stop>
    <span class="reader-cache-title">缓存章节</span>
    <div v-if="caching" class="reader-cache-status">
      <span>{{ statusText }}</span>
      <button class="reader-cache-cancel" type="button" title="取消缓存" aria-label="取消缓存" @click="$emit('cancel')">
        <el-icon :size="16"><Close /></el-icon>
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
  gap: 14px;
  min-height: 44px;
  box-sizing: border-box;
  padding: 8px 14px;
  color: var(--reader-popup-text, var(--reader-text));
  background: inherit;
  border: 0;
  box-shadow: none;
  font-size: 14px;
}

.reader-cache-title {
  flex: 0 0 auto;
}

.reader-cache-actions {
  display: flex;
  flex: 1;
  justify-content: flex-end;
  gap: 8px;
}

.reader-cache-actions button,
.reader-cache-status button {
  min-height: 30px;
  padding: 0 8px;
  color: var(--reader-popup-text, var(--reader-text));
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 13px;
}

.reader-cache-status {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.reader-cache-status button {
  flex: 0 0 auto;
}

.reader-cache-cancel {
  display: grid;
  width: 30px;
  padding: 0;
  place-items: center;
}

.reader-cache-actions button:focus-visible,
.reader-cache-cancel:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 1px;
}

@media (max-width: 750px) {
  .reader-cache-zone {
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 6px;
    width: 100%;
    padding: 8px 10px;
  }

  .reader-cache-title {
    width: 100%;
  }

  .reader-cache-actions,
  .reader-cache-status {
    width: 100%;
  }

  .reader-cache-actions {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .reader-cache-actions button {
    min-height: 34px;
    padding: 0 4px;
  }
}
</style>
