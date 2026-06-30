<template>
  <div class="reader-cache-panel">
    <div class="reader-cache-actions">
      <button type="button" :disabled="caching" @click="$emit('cache', 50)">后面50章</button>
      <button type="button" :disabled="caching" @click="$emit('cache', 100)">后面100章</button>
      <button type="button" :disabled="caching" @click="$emit('cache', true)">后面全部</button>
    </div>
    <div v-if="caching" class="reader-cache-status">
      <span>{{ statusText }}</span>
      <button type="button" @click="$emit('cancel')">取消</button>
    </div>
  </div>
</template>

<script setup>
defineProps({
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
.reader-cache-panel {
  display: grid;
  gap: 16px;
  color: #5f553f;
  font-size: 14px;
}

.reader-cache-actions {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.reader-cache-actions button,
.reader-cache-status button {
  min-height: 42px;
  color: #2a2925;
  background: var(--reader-popup-bg);
  border: 1px solid #e7dabb;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.reader-cache-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.reader-cache-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--reader-popup-bg) 88%, transparent);
  border: 1px solid #eadfca;
  border-radius: 6px;
}

.reader-cache-status button {
  flex: 0 0 auto;
  min-height: 34px;
  padding: 0 14px;
}
</style>
