<template>
  <!-- reader-dev Reader.vue mini `.tool-bar`: 首页 is ordered first, then 书架 书源 目录 设置. -->
  <header class="reader-mobile-top" :class="{ visible }">
    <div class="tools">
      <button class="mobile-tool-button" type="button" @click="$emit('action', 'home')">
        <span class="iconfont">&#58920;</span>
        <span class="icon-text">首页</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="$emit('action', 'shelf')">
        <span class="iconfont">&#58892;</span>
        <span class="icon-text">书架</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="$emit('action', 'source')">
        <span class="tool-el-icon"><el-icon><Menu /></el-icon></span>
        <span class="icon-text">书源</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="$emit('action', 'toc')">
        <span class="iconfont">&#58905;</span>
        <span class="icon-text">目录</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="$emit('action', 'settings')">
        <span class="iconfont">&#58971;</span>
        <span class="icon-text">设置</span>
      </button>
    </div>
  </header>

  <aside class="reader-mobile-float-tools reader-mobile-float-left" :class="{ visible }">
    <button type="button" aria-label="书签" @click="$emit('action', 'bookmarks')">
      <el-icon><CollectionTag /></el-icon>
    </button>
    <button type="button" aria-label="搜索正文" @click="$emit('action', 'search')">
      <el-icon><Search /></el-icon>
    </button>
    <button type="button" aria-label="书籍信息" @click="$emit('action', 'info')">
      <el-icon><InfoFilled /></el-icon>
    </button>
    <button type="button" aria-label="顶部" @click="$emit('action', 'top')">
      <el-icon><Top /></el-icon>
    </button>
    <button type="button" aria-label="底部" @click="$emit('action', 'bottom')">
      <el-icon><Bottom /></el-icon>
    </button>
  </aside>

  <aside class="reader-mobile-float-tools reader-mobile-float-right" :class="{ visible }">
    <button type="button" aria-label="重新载入章节" @click="$emit('action', 'reload')">
      <el-icon><RefreshRight /></el-icon>
    </button>
    <button v-if="autoReadingSupported" type="button" aria-label="自动阅读" @click="$emit('action', 'auto-read')">
      <el-icon><View /></el-icon>
    </button>
    <button v-if="ttsSupported" type="button" aria-label="朗读" @click="$emit('action', 'tts')">
      <el-icon><Headset /></el-icon>
    </button>
    <button type="button" :aria-label="isNight ? '日间模式' : '夜间模式'" @click="$emit('action', 'night')">
      <el-icon>
        <Sunny v-if="isNight" />
        <Moon v-else />
      </el-icon>
    </button>
  </aside>

  <footer class="reader-mobile-bottom" :class="{ visible }">
    <label v-if="pageSliderVisible" class="mobile-progress-slider-row">
      <span class="progress-bar" :style="{ '--page-slider-progress': pageSliderProgress }">
        <input
          class="mobile-progress-slider"
          type="range"
          min="1"
          :max="pageSliderMax"
          step="1"
          :value="pageSliderValue"
          :aria-label="`页码进度 ${pageSliderLabel}`"
          @input="$emit('page-progress-input', $event)"
          @change="$emit('page-progress-change', $event)"
        />
      </span>
      <span class="progress-tip">{{ pageSliderLabel }}</span>
    </label>
    <ReaderCachePanel
      class="mobile-cache-zone"
      :visible="cacheVisible"
      :caching="caching"
      :status-text="cacheStatusText"
      @cache="$emit('cache', $event)"
      @cancel="$emit('cache-cancel')"
    />
    <div class="tools">
      <button class="mobile-chapter-step" type="button" @click="$emit('action', 'previous')">
        <span class="iconfont">&#58920;</span>
        <span>上一章</span>
      </button>
      <button class="mobile-chapter-progress" type="button" @click="$emit('action', 'cache')">阅读进度: {{ bookProgressLabel }}</button>
      <button class="mobile-chapter-step" type="button" @click="$emit('action', 'next')">
        <span>下一章</span>
        <span class="iconfont">&#58913;</span>
      </button>
    </div>
  </footer>
</template>

<script setup>
import {
  Bottom,
  CollectionTag,
  Headset,
  InfoFilled,
  Menu,
  Moon,
  RefreshRight,
  Search,
  Sunny,
  Top,
  View,
} from '@element-plus/icons-vue'
import { computed } from 'vue'
import ReaderCachePanel from './ReaderCachePanel.vue'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  autoReadingSupported: {
    type: Boolean,
    default: true,
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
    default: '',
  },
  pageSliderVisible: {
    type: Boolean,
    default: true,
  },
  pageSliderValue: {
    type: Number,
    default: 1,
  },
  pageSliderMax: {
    type: Number,
    default: 1,
  },
  pageSliderLabel: {
    type: String,
    default: '第 1/1 页',
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
})

defineEmits(['action', 'page-progress-input', 'page-progress-change', 'cache', 'cache-cancel'])

const pageSliderProgress = computed(() => {
  const span = props.pageSliderMax - 1
  return span > 0 ? `${((props.pageSliderValue - 1) / span) * 100}%` : '0%'
})
</script>

<style scoped>
/* reader-dev Reader.vue `.chapter-wrapper.mini-interface` `.tool-bar` and `.read-bar`, measured on
   the fixed upstream build (docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md 3.1).
   Day colors are the fallbacks; night and Kindle set the --reader-chrome-* variables. */
.reader-mobile-top,
.reader-mobile-bottom,
.reader-mobile-float-tools {
  display: none;
}

.reader-mobile-top.visible {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 12;
  display: block;
  box-sizing: content-box;
  width: 100vw;
  padding: env(safe-area-inset-top) 0 0;
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  font-family: "Avenir", Helvetica, Arial, sans-serif;
}

.reader-mobile-top .tools {
  display: flex;
  flex-direction: row;
  justify-content: space-around;
}

/* A button centers its content; reader-dev's div stacks glyph and label from the top. */
.mobile-tool-button {
  box-sizing: content-box;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  width: 58px;
  height: 48px;
  margin: -1px 0 0;
  padding: 12px 0 0;
  color: var(--reader-chrome-text, #000);
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  font-size: 18px;
  line-height: normal;
  text-align: center;
}

.mobile-tool-button .iconfont {
  display: block;
  width: 16px;
  height: 22px;
  margin: 0 auto;
  font-family: iconfont;
  font-size: 16px;
  line-height: 22px;
}

.mobile-tool-button .tool-el-icon {
  display: flex;
  height: 22px;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}

.mobile-tool-button .icon-text {
  display: block;
  color: var(--reader-chrome-label, var(--reader-chrome-text, rgba(0, 0, 0, 0.4)));
  font-size: 12px;
}

.reader-mobile-float-tools.visible {
  position: fixed;
  bottom: 135px;
  z-index: 10;
  display: flex;
  flex-direction: column;
}

.reader-mobile-float-left {
  left: 20px;
}

.reader-mobile-float-right {
  right: 20px;
  margin-bottom: 20px;
}

.reader-mobile-float-tools button {
  box-sizing: content-box;
  display: flex;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  margin: 20px 0 0;
  padding: 0;
  color: var(--reader-chrome-text, #121212);
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  border: 0;
  border-radius: 100%;
  cursor: pointer;
  font-size: 16px;
}

.reader-mobile-bottom.visible {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 10;
  display: block;
  width: 100vw;
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  font-family: "Avenir", Helvetica, Arial, sans-serif;
}

.mobile-progress-slider-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 36px;
}

/* reader-dev el-slider: a 6px runway (day #fff) whose thumb center travels its full width. */
.mobile-progress-slider-row .progress-bar {
  position: relative;
  flex: 1;
  padding: 0 10px;
}

.mobile-progress-slider-row .progress-bar::before {
  position: absolute;
  top: 50%;
  right: 10px;
  left: 10px;
  height: 6px;
  margin-top: -3px;
  content: "";
  background: linear-gradient(
    to right,
    var(--reader-slider-accent, #409eff) var(--page-slider-progress, 0%),
    var(--reader-slider-runway, #fff) var(--page-slider-progress, 0%)
  );
  border-radius: 3px;
}

.mobile-progress-slider {
  position: relative;
  display: block;
  width: calc(100% + 20px);
  height: 38px;
  margin: 0 -10px;
  -webkit-appearance: none;
  appearance: none;
  background: transparent;
  cursor: pointer;
}

.mobile-progress-slider::-webkit-slider-runnable-track {
  height: 38px;
  background: transparent;
}

.mobile-progress-slider::-webkit-slider-thumb {
  box-sizing: border-box;
  width: 20px;
  height: 20px;
  margin-top: 9px;
  -webkit-appearance: none;
  appearance: none;
  background: var(--reader-slider-runway, #fff);
  border: 2px solid var(--reader-slider-accent, #409eff);
  border-radius: 50%;
  transition: transform 0.2s;
}

.mobile-progress-slider::-moz-range-track {
  height: 38px;
  background: transparent;
}

.mobile-progress-slider::-moz-range-progress {
  background: transparent;
}

.mobile-progress-slider::-moz-range-thumb {
  box-sizing: border-box;
  width: 20px;
  height: 20px;
  background: var(--reader-slider-runway, #fff);
  border: 2px solid var(--reader-slider-accent, #409eff);
  border-radius: 50%;
}

.mobile-progress-slider:hover::-webkit-slider-thumb,
.mobile-progress-slider:active::-webkit-slider-thumb {
  transform: scale(1.2);
}

.mobile-progress-slider-row .progress-tip {
  margin-left: 5px;
  color: var(--reader-chrome-label, var(--reader-chrome-text, rgba(0, 0, 0, 0.4)));
  font-size: 14px;
}

.reader-mobile-bottom .tools {
  display: flex;
  flex-direction: row;
  justify-content: space-around;
  height: 45px;
  padding: 0 15px;
}

.mobile-chapter-step,
.mobile-chapter-progress {
  box-sizing: content-box;
  height: 45px;
  margin: -1px 0 0;
  padding: 0;
  color: var(--reader-chrome-text, #000);
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  font-size: 18px;
  line-height: 45px;
  text-align: center;
}

.mobile-chapter-progress {
  font-size: 16px;
}

.mobile-chapter-step .iconfont {
  display: inline-block;
  width: 16px;
  height: 16px;
  margin: 0 auto 6px;
  font-family: iconfont;
  font-size: 16px;
}

/* reader-dev aligns the text, not the glyph (a div there), to the middle. */
.mobile-chapter-step span:not(.iconfont) {
  vertical-align: middle;
}
</style>
