<template>
  <aside class="reader-left-rail">
    <button class="rail-item" :class="{ active: activePanel === 'shelf' }" type="button" title="书架" @click="$emit('action', 'shelf')">
      <el-icon :size="18"><Notebook /></el-icon>
      <span>书架</span>
    </button>
    <button class="rail-item" :class="{ active: activePanel === 'source' }" type="button" :disabled="!remoteBook" :title="remoteBook ? '书源' : '本地书无可切换书源'" @click="$emit('action', 'source')">
      <el-icon :size="18"><Grid /></el-icon>
      <span>书源</span>
    </button>
    <button class="rail-item" :class="{ active: activePanel === 'toc' }" type="button" title="目录" @click="$emit('action', 'toc')">
      <el-icon :size="18"><List /></el-icon>
      <span>目录</span>
    </button>
    <button class="rail-item" :class="{ active: activePanel === 'settings' }" type="button" title="设置" @click="$emit('action', 'settings')">
      <el-icon :size="18"><Setting /></el-icon>
      <span>设置</span>
    </button>
    <button class="rail-item rail-home" type="button" title="返回首页" @click="$emit('action', 'home')">
      <el-icon :size="18"><ArrowLeft /></el-icon>
      <span>首页</span>
    </button>
    <button class="rail-item" type="button" title="回到顶部" @click="$emit('action', 'top')">
      <el-icon :size="18"><ArrowUpBold /></el-icon>
      <span>顶部</span>
    </button>
    <button class="rail-item" type="button" title="跳到底部" @click="$emit('action', 'bottom')">
      <el-icon :size="18"><ArrowDownBold /></el-icon>
      <span>底部</span>
    </button>
  </aside>

  <aside class="reader-right-rail">
    <button class="round-tool" type="button" title="书签" @click="$emit('action', 'bookmarks')">
      <el-icon :size="18"><CollectionTag /></el-icon>
    </button>
    <button class="round-tool" type="button" title="搜索正文" @click="$emit('action', 'search')">
      <el-icon :size="18"><Search /></el-icon>
    </button>
    <button class="round-tool" type="button" title="书籍信息" @click="$emit('action', 'info')">
      <el-icon :size="18"><InfoFilled /></el-icon>
    </button>
    <button class="round-tool" type="button" title="重新载入章节" @click="$emit('action', 'reload')">
      <el-icon :size="18"><RefreshRight /></el-icon>
    </button>
    <button class="round-tool" type="button" :class="{ active: autoReading }" title="自动阅读" @click="$emit('action', 'auto-read')">
      <el-icon :size="18"><VideoPlay /></el-icon>
    </button>
    <button class="round-tool" type="button" :class="{ active: ttsPlaying }" :disabled="!ttsSupported" :title="ttsSupported ? '朗读' : '当前浏览器不支持朗读'" @click="$emit('action', 'tts')">
      <el-icon :size="18"><Headset /></el-icon>
    </button>
    <button class="round-tool" type="button" title="夜间模式" @click="$emit('action', 'night')">
      <el-icon :size="18"><Moon /></el-icon>
    </button>
  </aside>
</template>

<script setup>
import {
  ArrowDownBold,
  ArrowLeft,
  ArrowUpBold,
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
  VideoPlay,
} from '@element-plus/icons-vue'

defineProps({
  remoteBook: {
    type: Boolean,
    default: false,
  },
  autoReading: {
    type: Boolean,
    default: false,
  },
  ttsPlaying: {
    type: Boolean,
    default: false,
  },
  ttsSupported: {
    type: Boolean,
    default: false,
  },
  activePanel: {
    type: String,
    default: '',
  },
})

defineEmits(['action'])
</script>

<style scoped>
.reader-left-rail {
  position: fixed;
  top: 0;
  bottom: 0;
  left: max(8px, var(--reader-left-x));
  z-index: 4;
  display: grid;
  width: 58px;
  align-content: start;
  background: color-mix(in srgb, var(--reader-popup-bg) 64%, transparent);
  border-right: 1px solid rgba(148, 132, 87, 0.38);
  border-left: 1px solid rgba(148, 132, 87, 0.26);
  backdrop-filter: blur(2px);
}

.rail-item {
  display: grid;
  width: 100%;
  height: 60px;
  place-items: center;
  align-content: center;
  gap: 2px;
  padding: 7px 0 5px;
  color: rgba(36, 33, 27, 0.62);
  background: color-mix(in srgb, var(--reader-popup-bg) 58%, transparent);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.35);
  cursor: pointer;
  font-size: 16px;
}

.rail-item span {
  font-size: 12px;
  line-height: 1;
}

.rail-item:hover {
  color: #1e1f22;
  background: color-mix(in srgb, var(--reader-popup-bg) 82%, transparent);
}

.rail-item.active {
  color: #ed4259;
  background: color-mix(in srgb, var(--reader-popup-bg) 88%, transparent);
}

.rail-item:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.rail-home {
  height: 60px;
  color: #111;
}

.reader-right-rail {
  position: fixed;
  right: auto;
  bottom: 150px;
  left: var(--reader-right-x);
  z-index: 4;
  display: grid;
  max-height: max(120px, calc(100vh - 170px));
  grid-template-columns: 36px;
  grid-auto-rows: 36px;
  align-content: start;
  gap: 16px;
  padding-right: 2px;
  overflow-y: auto;
  scrollbar-width: none;
}

.reader-right-rail::-webkit-scrollbar {
  display: none;
}

.round-tool {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #121212;
  background: var(--reader-popup-bg);
  border: 1px solid rgba(255, 255, 255, 0.7);
  border-radius: 999px;
  box-shadow: 0 4px 10px rgba(80, 62, 28, 0.08);
  cursor: pointer;
}

.round-tool:hover,
.round-tool.active {
  color: #0f5451;
  background: var(--reader-popup-bg);
  box-shadow: 0 12px 26px rgba(80, 62, 28, 0.14);
}

.round-tool:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

@media (max-width: 750px) {
  .reader-left-rail,
  .reader-right-rail {
    display: none;
  }
}
</style>
