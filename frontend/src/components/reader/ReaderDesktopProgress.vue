<template>
  <footer class="reader-page-control">
    <ReaderCachePanel
      class="desktop-cache-zone"
      :visible="cacheVisible"
      :caching="caching"
      :status-text="cacheStatusText"
      @cache="$emit('cache', $event)"
      @cancel="$emit('cache-cancel')"
    />
    <button class="progress-box" type="button" title="缓存章节" @click="$emit('cache-toggle')">{{ bookProgressLabel }}</button>
    <button class="page-step chapter-step" type="button" title="上一章" :disabled="previousDisabled" @click="$emit('previous')">
      <el-icon :size="24"><ArrowLeft /></el-icon>
    </button>
    <button class="page-step chapter-step" type="button" title="下一章" :disabled="nextDisabled" @click="$emit('next')">
      <el-icon :size="24"><ArrowRight /></el-icon>
    </button>
  </footer>
</template>

<script setup>
import { ArrowLeft, ArrowRight } from '@element-plus/icons-vue'
import ReaderCachePanel from './ReaderCachePanel.vue'

defineProps({
  bookProgressLabel: {
    type: String,
    default: '0%',
  },
  cacheVisible: {
    type: Boolean,
    default: false,
  },
  caching: {
    type: Boolean,
    default: false,
  },
  cacheStatusText: {
    type: String,
    default: '',
  },
  previousDisabled: {
    type: Boolean,
    default: false,
  },
  nextDisabled: {
    type: Boolean,
    default: false,
  },
})

defineEmits([
  'cache-toggle',
  'cache',
  'cache-cancel',
  'previous',
  'next',
])
</script>

<style scoped>
.reader-page-control {
  position: fixed;
  right: auto;
  bottom: 0;
  left: var(--reader-right-x);
  z-index: 4;
  display: grid;
  width: 46px;
  background: color-mix(in srgb, var(--reader-popup-bg) 82%, transparent);
  border: 1px solid rgba(148, 132, 87, 0.38);
  border-bottom: 0;
}

.desktop-cache-zone {
  position: absolute;
  right: 54px;
  bottom: 0;
  width: 300px;
}

.progress-box,
.page-step {
  display: grid;
  height: 43px;
  place-items: center;
  color: #121212;
  background: color-mix(in srgb, var(--reader-popup-bg) 62%, transparent);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.32);
  font-size: 16px;
}

.page-step {
  cursor: pointer;
}

.progress-box {
  width: 100%;
  padding: 0;
  cursor: pointer;
}

.chapter-step {
  padding: 0;
  font-size: 16px;
}

.chapter-step:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.page-step:hover {
  background: var(--reader-popup-bg);
}

@media (max-width: 750px) {
  .reader-page-control {
    display: none;
  }
}
</style>
