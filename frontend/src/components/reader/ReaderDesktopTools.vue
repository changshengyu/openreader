<template>
  <!-- reader-dev Reader.vue `.tool-bar`: seven tools left of the reading frame. -->
  <aside class="reader-left-rail">
    <div class="tools">
      <button class="rail-item" type="button" aria-label="书架" @click="$emit('action', 'shelf')">
        <span class="iconfont">&#58892;</span>
        <span class="icon-text">书架</span>
      </button>
      <button class="rail-item" type="button" aria-label="书源" @click="$emit('action', 'source')">
        <span class="tool-el-icon"><el-icon><Menu /></el-icon></span>
        <span class="icon-text">书源</span>
      </button>
      <button class="rail-item" type="button" aria-label="目录" @click="$emit('action', 'toc')">
        <span class="iconfont">&#58905;</span>
        <span class="icon-text">目录</span>
      </button>
      <button class="rail-item" type="button" aria-label="设置" @click="$emit('action', 'settings')">
        <span class="iconfont">&#58971;</span>
        <span class="icon-text">设置</span>
      </button>
      <button class="rail-item" type="button" aria-label="返回首页" @click="$emit('action', 'home')">
        <span class="iconfont">&#58920;</span>
        <span class="icon-text">首页</span>
      </button>
      <button class="rail-item" type="button" aria-label="回到顶部" @click="$emit('action', 'top')">
        <span class="iconfont">&#58914;</span>
        <span class="icon-text">顶部</span>
      </button>
      <button class="rail-item" type="button" aria-label="跳到底部" @click="$emit('action', 'bottom')">
        <span class="iconfont">&#58915;</span>
        <span class="icon-text">底部</span>
      </button>
    </div>
  </aside>

  <!-- reader-dev `.float-btn-zone` of the right `.read-bar`: left zone then right zone. -->
  <aside class="reader-right-rail">
    <button class="round-tool" type="button" aria-label="书签" @click="$emit('action', 'bookmarks')">
      <el-icon><CollectionTag /></el-icon>
    </button>
    <button class="round-tool" type="button" aria-label="搜索正文" @click="$emit('action', 'search')">
      <el-icon><Search /></el-icon>
    </button>
    <button class="round-tool" type="button" aria-label="书籍信息" @click="$emit('action', 'info')">
      <el-icon><InfoFilled /></el-icon>
    </button>
    <button class="round-tool" type="button" aria-label="重新载入章节" @click="$emit('action', 'reload')">
      <el-icon><RefreshRight /></el-icon>
    </button>
    <button v-if="autoReadingSupported" class="round-tool" type="button" aria-label="自动阅读" @click="$emit('action', 'auto-read')">
      <el-icon><View /></el-icon>
    </button>
    <button v-if="ttsSupported" class="round-tool" type="button" aria-label="朗读" @click="$emit('action', 'tts')">
      <el-icon><Headset /></el-icon>
    </button>
    <button class="round-tool" type="button" :aria-label="isNight ? '日间模式' : '夜间模式'" @click="$emit('action', 'night')">
      <el-icon>
        <Sunny v-if="isNight" />
        <Moon v-else />
      </el-icon>
    </button>
  </aside>
</template>

<script setup>
import {
  CollectionTag,
  Headset,
  InfoFilled,
  Menu,
  Moon,
  RefreshRight,
  Search,
  Sunny,
  View,
} from '@element-plus/icons-vue'

defineProps({
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
})

defineEmits(['action'])
</script>

<style scoped>
/* reader-dev Reader.vue desktop `.tool-bar` (left: 50%, margin-left -(readWidthConfig/2 + 68px)) and the
   float zone of the right `.read-bar` (right edge readWidthConfig/2 + 52px right of center, 4px inset,
   135px + 20px above the bottom). Measured in docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md 3.2. */
.reader-left-rail {
  position: fixed;
  top: 0;
  left: 50%;
  z-index: 12;
  box-sizing: content-box;
  margin-left: calc(var(--reader-frame-width) / -2 - 68px);
  padding-top: env(safe-area-inset-top);
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  font-family: "Avenir", Helvetica, Arial, sans-serif;
}

.reader-left-rail .tools {
  display: flex;
  flex-direction: column;
}

/* A button centers its content; reader-dev's div stacks glyph and label from the top. */
.rail-item {
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
  border: 1px solid var(--reader-chrome-border, rgba(0, 0, 0, 0.1));
  cursor: pointer;
  font: inherit;
  font-size: 18px;
  line-height: normal;
  text-align: center;
}

.rail-item .iconfont {
  display: block;
  width: 16px;
  height: 22px;
  margin: 0 auto;
  font-family: iconfont;
  font-size: 16px;
  line-height: 22px;
}

.rail-item .tool-el-icon {
  display: flex;
  height: 22px;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}

.rail-item .icon-text {
  display: block;
  color: var(--reader-chrome-label, var(--reader-chrome-text, rgba(0, 0, 0, 0.4)));
  font-size: 12px;
}

.reader-right-rail {
  position: fixed;
  bottom: 135px;
  left: 50%;
  z-index: 5;
  display: flex;
  flex-direction: column;
  margin-left: calc(var(--reader-frame-width) / 2 + 12px);
  padding-bottom: 20px;
}

.round-tool {
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

@media (max-width: 750px) {
  .reader-left-rail,
  .reader-right-rail {
    display: none;
  }
}
</style>
