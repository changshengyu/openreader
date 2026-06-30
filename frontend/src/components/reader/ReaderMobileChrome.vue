<template>
  <header class="reader-mobile-top" :class="{ visible }">
    <button class="mobile-tool-button" type="button" aria-label="返回首页" @click="$emit('action', 'home')">
      <el-icon :size="20"><ArrowLeft /></el-icon>
    </button>
    <div class="mobile-reader-title">
      <strong>{{ bookTitle || '阅读中' }}</strong>
      <span>{{ chapterTitle || chapterLabel }}</span>
    </div>
    <span class="mobile-reader-progress">{{ bookProgressLabel }}</span>
  </header>

  <footer class="reader-mobile-bottom" :class="{ visible }">
    <div class="reader-mobile-progress-panel">
      <label class="mobile-progress-slider-row" title="拖动定位阅读进度">
        <input
          class="mobile-progress-slider"
          type="range"
          min="0"
          max="1000"
          step="1"
          :value="bookSliderValue"
          :aria-label="`阅读进度 ${bookSliderLabel}`"
          @input="$emit('book-progress-input', $event)"
          @change="$emit('book-progress-change', $event)"
        />
        <span>{{ bookSliderLabel }}</span>
      </label>
      <button class="mobile-chapter-step" type="button" :disabled="previousDisabled" @click="$emit('action', 'previous')">
        上一章
      </button>
      <button class="mobile-chapter-progress" type="button" @click="$emit('action', 'toggle')">
        <strong>{{ bookProgressLabel }}</strong>
        <span>{{ chapterLabel }}</span>
      </button>
      <button class="mobile-chapter-step" type="button" :disabled="nextDisabled" @click="$emit('action', 'next')">
        下一章
      </button>
    </div>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'toc')">
      <el-icon :size="20"><List /></el-icon>
      <span>目录</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'bookmarks')">
      <el-icon :size="20"><CollectionTag /></el-icon>
      <span>书签</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'search')">
      <el-icon :size="20"><Search /></el-icon>
      <span>搜索</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'settings')">
      <el-icon :size="20"><Setting /></el-icon>
      <span>设置</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'more')">
      <el-icon :size="20"><MoreFilled /></el-icon>
      <span>更多</span>
    </button>
  </footer>
</template>

<script setup>
import {
  ArrowLeft,
  CollectionTag,
  List,
  MoreFilled,
  Search,
  Setting,
} from '@element-plus/icons-vue'

defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  bookTitle: {
    type: String,
    default: '',
  },
  chapterTitle: {
    type: String,
    default: '',
  },
  bookProgressLabel: {
    type: String,
    default: '0%',
  },
  chapterLabel: {
    type: String,
    default: '',
  },
  bookSliderValue: {
    type: Number,
    default: 0,
  },
  bookSliderLabel: {
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

defineEmits(['action', 'book-progress-input', 'book-progress-change'])
</script>

<style scoped>
.reader-mobile-top,
.reader-mobile-bottom,
.reader-mobile-progress-panel {
  display: none;
}

@media (max-width: 750px) {
  .reader-mobile-top {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    z-index: 8;
    grid-template-columns: 44px minmax(0, 1fr) 52px;
    align-items: center;
    gap: 8px;
    min-height: 58px;
    padding: max(8px, env(safe-area-inset-top)) 12px 8px;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border-bottom: 1px solid rgba(148, 132, 87, 0.28);
    box-shadow: 0 8px 24px rgba(73, 57, 27, 0.08);
  }

  .mobile-reader-title {
    display: grid;
    min-width: 0;
    gap: 2px;
    color: #25282c;
  }

  .mobile-reader-title strong,
  .mobile-reader-title span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mobile-reader-title strong {
    font-size: 14px;
  }

  .mobile-reader-title span,
  .mobile-reader-progress {
    color: #756c5a;
    font-size: 12px;
  }

  .mobile-reader-progress {
    text-align: right;
  }

  .reader-mobile-bottom {
    position: fixed;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: 8;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    align-items: center;
    gap: 7px 4px;
    min-height: calc(76px + env(safe-area-inset-bottom));
    box-sizing: border-box;
    padding: 8px 10px max(10px, env(safe-area-inset-bottom));
    background: color-mix(in srgb, var(--reader-popup-bg) 94%, transparent);
    border-top: 1px solid rgba(148, 132, 87, 0.35);
    border-radius: 10px 10px 0 0;
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
  }

  .reader-mobile-progress-panel {
    grid-column: 1 / -1;
    grid-template-columns: minmax(62px, 76px) minmax(0, 1fr) minmax(62px, 76px);
    align-items: center;
    gap: 8px;
    min-height: 84px;
    padding: 7px;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border: 1px solid rgba(148, 132, 87, 0.28);
    border-radius: 8px;
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
  }

  .mobile-progress-slider-row {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    min-width: 0;
    padding: 0 3px;
    color: #8d8270;
    font-size: 12px;
  }

  .mobile-progress-slider {
    width: 100%;
    min-width: 0;
    accent-color: #409eff;
  }

  .reader-mobile-top.visible,
  .reader-mobile-bottom.visible,
  .reader-mobile-bottom.visible .reader-mobile-progress-panel,
  .reader-mobile-bottom.visible > .mobile-tool-button {
    display: grid;
  }

  .mobile-chapter-step {
    min-width: 0;
    min-height: 38px;
    color: #24201b;
    background: var(--reader-popup-bg);
    border: 1px solid rgba(148, 132, 87, 0.3);
    border-radius: 6px;
    font-size: 13px;
  }

  .mobile-chapter-step:disabled {
    color: #a09282;
    opacity: 0.55;
  }

  .mobile-chapter-progress {
    display: grid;
    min-width: 0;
    justify-items: center;
    gap: 2px;
    padding: 0;
    background: transparent;
    border: 0;
    cursor: pointer;
  }

  .mobile-chapter-progress strong,
  .mobile-chapter-progress span {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mobile-chapter-progress strong {
    color: #121212;
    font-size: 14px;
  }

  .mobile-chapter-progress span {
    color: #756c5a;
    font-size: 12px;
  }

  .mobile-tool-button {
    min-width: 0;
    min-height: 44px;
    place-items: center;
    gap: 3px;
    padding: 6px 4px;
    color: #111;
    background: transparent;
    border: 0;
    border-radius: 6px;
    font-size: 12px;
  }

  .mobile-tool-button:active {
    background: rgba(114, 91, 43, 0.1);
  }
}
</style>
