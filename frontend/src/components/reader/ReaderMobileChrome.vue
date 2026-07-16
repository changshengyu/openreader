<template>
  <header class="reader-mobile-top" :class="{ visible }">
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'shelf')">
      <el-icon :size="19"><Notebook /></el-icon>
      <span>书架</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'source')">
      <el-icon :size="19"><Grid /></el-icon>
      <span>书源</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'toc')">
      <el-icon :size="19"><List /></el-icon>
      <span>目录</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'settings')">
      <el-icon :size="19"><Setting /></el-icon>
      <span>设置</span>
    </button>
    <button class="mobile-tool-button" type="button" @click="$emit('action', 'home')">
      <el-icon :size="19"><ArrowLeft /></el-icon>
      <span>首页</span>
    </button>
  </header>

  <aside class="reader-mobile-float-tools reader-mobile-float-left" :class="{ visible, 'cache-zone-visible': cacheVisible }">
    <button type="button" title="书签" @click="$emit('action', 'bookmarks')">
      <el-icon :size="18"><CollectionTag /></el-icon>
    </button>
    <button type="button" title="搜索正文" @click="$emit('action', 'search')">
      <el-icon :size="18"><Search /></el-icon>
    </button>
    <button type="button" title="书籍信息" @click="$emit('action', 'info')">
      <el-icon :size="18"><InfoFilled /></el-icon>
    </button>
  </aside>

  <aside class="reader-mobile-float-tools reader-mobile-float-right" :class="{ visible, 'cache-zone-visible': cacheVisible }">
    <button type="button" title="重新载入章节" @click="$emit('action', 'reload')">
      <el-icon :size="18"><RefreshRight /></el-icon>
    </button>
    <button v-if="autoReadingSupported" type="button" :class="{ active: autoReading }" title="自动阅读" @click="$emit('action', 'auto-read')">
      <el-icon :size="18"><View /></el-icon>
    </button>
    <button
      v-if="ttsSupported"
      type="button"
      :class="{ active: ttsPlaying }"
      title="朗读"
      @click="$emit('action', 'tts')"
    >
      <el-icon :size="18"><Headset /></el-icon>
    </button>
    <button type="button" :title="isNight ? '日间模式' : '夜间模式'" @click="$emit('action', 'night')">
      <el-icon :size="18">
        <Sunny v-if="isNight" />
        <Moon v-else />
      </el-icon>
    </button>
  </aside>

  <footer class="reader-mobile-bottom" :class="{ visible }">
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
    <ReaderCachePanel
      class="mobile-cache-zone"
      :visible="cacheVisible"
      :caching="caching"
      :status-text="cacheStatusText"
      @cache="$emit('cache', $event)"
      @cancel="$emit('cache-cancel')"
    />
    <button class="mobile-chapter-step" type="button" :disabled="previousDisabled" @click="$emit('action', 'previous')">
      <el-icon :size="18"><ArrowLeft /></el-icon>
      <span>上一章</span>
    </button>
    <button class="mobile-chapter-progress" type="button" title="缓存章节" @click="$emit('action', 'cache')">
      <strong>{{ bookProgressLabel }}</strong>
      <span>{{ chapterLabel }}</span>
    </button>
    <button class="mobile-chapter-step" type="button" :disabled="nextDisabled" @click="$emit('action', 'next')">
      <span>下一章</span>
      <el-icon :size="18"><ArrowRight /></el-icon>
    </button>
  </footer>
</template>

<script setup>
import {
  ArrowLeft,
  ArrowRight,
  CollectionTag,
  Grid,
  Headset,
  InfoFilled,
  List,
  Moon,
  Notebook,
  RefreshRight,
  Search,
  Setting,
  Sunny,
  View,
} from '@element-plus/icons-vue'
import ReaderCachePanel from './ReaderCachePanel.vue'

defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  autoReading: {
    type: Boolean,
    default: false,
  },
  autoReadingSupported: {
    type: Boolean,
    default: true,
  },
  ttsPlaying: {
    type: Boolean,
    default: false,
  },
  ttsSupported: {
    type: Boolean,
    default: false,
  },
  isNight: {
    type: Boolean,
    default: false,
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

defineEmits(['action', 'book-progress-input', 'book-progress-change', 'cache', 'cache-cancel'])
</script>

<style scoped>
.reader-mobile-top,
.reader-mobile-bottom,
.reader-mobile-float-tools {
  display: none;
}

@media (max-width: 750px) {
  .reader-mobile-top.visible {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    /* Primary popovers occupy the viewport, but reader-dev keeps the tool
       strip above its popovers so the active tool can close/switch them. */
    z-index: 11;
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    padding: max(5px, env(safe-area-inset-top)) 8px 5px;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border-bottom: 1px solid rgba(148, 132, 87, 0.28);
    box-shadow: 0 8px 24px rgba(73, 57, 27, 0.08);
  }

  .mobile-tool-button {
    display: grid;
    min-width: 0;
    min-height: 48px;
    place-items: center;
    align-content: center;
    gap: 2px;
    padding: 4px 2px;
    color: #25221d;
    background: transparent;
    border: 0;
    font-size: 12px;
  }

  .mobile-tool-button:disabled {
    opacity: 0.4;
  }

  .reader-mobile-float-tools.visible {
    position: fixed;
    bottom: 134px;
    z-index: 8;
    display: grid;
    gap: 12px;
  }

  .reader-mobile-float-tools.cache-zone-visible {
    bottom: 202px;
  }

  .reader-mobile-float-left {
    left: 18px;
  }

  .reader-mobile-float-right {
    right: 18px;
  }

  .reader-mobile-float-tools button {
    display: grid;
    width: 38px;
    height: 38px;
    place-items: center;
    color: #191714;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border: 1px solid rgba(255, 255, 255, 0.72);
    border-radius: 999px;
    box-shadow: 0 4px 12px rgba(73, 57, 27, 0.12);
  }

  .reader-mobile-float-tools button.active {
    color: #0f5451;
  }

  .reader-mobile-float-tools button:disabled {
    opacity: 0.42;
  }

  .reader-mobile-bottom.visible {
    position: fixed;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: 8;
    display: grid;
    grid-template-columns: minmax(72px, 92px) minmax(0, 1fr) minmax(72px, 92px);
    align-items: center;
    gap: 8px;
    min-height: calc(88px + env(safe-area-inset-bottom));
    box-sizing: border-box;
    padding: 8px 12px max(10px, env(safe-area-inset-bottom));
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border-top: 1px solid rgba(148, 132, 87, 0.35);
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
  }

  .mobile-progress-slider-row {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    color: #756c5a;
    font-size: 12px;
  }

  .mobile-progress-slider {
    width: 100%;
    min-width: 0;
    accent-color: #409eff;
  }

  .mobile-cache-zone {
    grid-column: 1 / -1;
  }

  .mobile-chapter-step {
    display: flex;
    min-width: 0;
    min-height: 36px;
    align-items: center;
    justify-content: center;
    gap: 2px;
    color: #24201b;
    background: transparent;
    border: 0;
    font-size: 13px;
  }

  .mobile-chapter-step:disabled {
    opacity: 0.45;
  }

  .mobile-chapter-progress {
    display: grid;
    min-width: 0;
    justify-items: center;
    gap: 2px;
    padding: 0;
    background: transparent;
    border: 0;
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
}
</style>
