<template>
  <footer class="reader-page-control">
    <button class="progress-box" type="button" title="缓存章节" @click="$emit('cache')">{{ bookProgressLabel }}</button>
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

defineProps({
  bookProgressLabel: {
    type: String,
    default: '0%',
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
  'cache',
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
