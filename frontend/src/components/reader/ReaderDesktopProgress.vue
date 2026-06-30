<template>
  <footer class="reader-page-control">
    <div class="progress-box">{{ bookProgressLabel }}</div>
    <button class="page-step chapter-step" type="button" title="上一章" :disabled="previousDisabled" @click="$emit('previous')">
      <el-icon :size="24"><ArrowLeft /></el-icon>
    </button>
    <button class="page-step chapter-step" type="button" title="下一章" :disabled="nextDisabled" @click="$emit('next')">
      <el-icon :size="24"><ArrowRight /></el-icon>
    </button>
    <label class="desktop-progress-control" title="拖动定位当前章节进度">
      <input
        class="desktop-progress-slider"
        type="range"
        min="0"
        max="1000"
        step="1"
        :value="chapterSliderValue"
        :aria-label="`当前章节进度 ${chapterProgressLabel}`"
        @input="$emit('chapter-progress-input', $event)"
        @change="$emit('chapter-progress-change', $event)"
      />
      <span>{{ chapterProgressLabel }}</span>
    </label>
  </footer>
</template>

<script setup>
import { ArrowLeft, ArrowRight } from '@element-plus/icons-vue'

defineProps({
  bookProgressLabel: {
    type: String,
    default: '0%',
  },
  chapterSliderValue: {
    type: Number,
    default: 0,
  },
  chapterProgressLabel: {
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
  'previous',
  'next',
  'chapter-progress-input',
  'chapter-progress-change',
])
</script>

<style scoped>
.reader-page-control {
  position: fixed;
  right: auto;
  bottom: 0;
  left: calc(50vw + var(--reader-frame-width) / 2 + 52px);
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

.desktop-progress-control {
  display: grid;
  width: 100%;
  min-height: 154px;
  place-items: center;
  gap: 7px;
  padding: 9px 0;
  color: #121212;
  background: color-mix(in srgb, var(--reader-popup-bg) 62%, transparent);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.32);
  font-size: 12px;
}

.desktop-progress-control span {
  line-height: 1;
}

.desktop-progress-slider {
  width: 18px;
  height: 124px;
  margin: 0;
  accent-color: #2f6f6d;
  cursor: pointer;
  writing-mode: vertical-lr;
}

.page-step {
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
