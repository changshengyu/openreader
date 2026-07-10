<template>
  <section v-if="visible" class="reader-cache-zone" @click.stop>
    <span class="reader-cache-title">缓存章节</span>
    <div v-if="caching" class="reader-cache-status">
      <span>{{ statusText }}</span>
      <button type="button" @click="$emit('cancel')">取消</button>
    </div>
    <div v-else class="reader-cache-actions">
      <button type="button" @click="$emit('cache', 50)">后面50章</button>
      <button type="button" @click="$emit('cache', 100)">后面100章</button>
      <button type="button" @click="$emit('cache', true)">后面全部</button>
    </div>
  </section>
</template>

<script setup>
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
  color: #5f553f;
  background: var(--reader-popup-bg);
  border: 1px solid rgba(148, 132, 87, 0.38);
  box-shadow: 0 4px 14px rgba(73, 57, 27, 0.1);
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
  color: #2a2925;
  background: transparent;
  border: 1px solid #e7dabb;
  border-radius: 3px;
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
