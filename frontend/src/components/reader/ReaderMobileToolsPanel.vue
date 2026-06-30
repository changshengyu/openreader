<template>
  <div class="mobile-more-grid">
    <button type="button" class="mobile-more-item" @click="$emit('action', 'shelf')">
      <el-icon :size="22"><Notebook /></el-icon>
      <span>书架</span>
    </button>
    <button v-if="remoteBook" type="button" class="mobile-more-item" @click="$emit('action', 'source')">
      <el-icon :size="22"><Grid /></el-icon>
      <span>书源</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'info')">
      <el-icon :size="22"><InfoFilled /></el-icon>
      <span>信息</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'note')">
      <el-icon :size="22"><EditPen /></el-icon>
      <span>笔记</span>
    </button>
    <button v-if="remoteBook" type="button" class="mobile-more-item" @click="$emit('action', 'cache')">
      <el-icon :size="22"><Download /></el-icon>
      <span>缓存</span>
    </button>
    <button v-if="remoteBook" type="button" class="mobile-more-item" @click="$emit('action', 'clear-cache')">
      <el-icon :size="22"><Delete /></el-icon>
      <span>清缓存</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'reload')">
      <el-icon :size="22"><RefreshRight /></el-icon>
      <span>刷新</span>
    </button>
    <button type="button" class="mobile-more-item" :class="{ active: autoReading }" @click="$emit('action', 'auto-read')">
      <el-icon :size="22"><VideoPlay /></el-icon>
      <span>自动</span>
    </button>
    <button type="button" class="mobile-more-item" :class="{ active: ttsPlaying }" :disabled="!ttsSupported" @click="$emit('action', 'tts')">
      <el-icon :size="22"><Headset /></el-icon>
      <span>听书</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'night')">
      <el-icon :size="22"><Moon /></el-icon>
      <span>夜间</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'top')">
      <el-icon :size="22"><ArrowUpBold /></el-icon>
      <span>顶部</span>
    </button>
    <button type="button" class="mobile-more-item" @click="$emit('action', 'bottom')">
      <el-icon :size="22"><ArrowDownBold /></el-icon>
      <span>底部</span>
    </button>
  </div>
  <p v-if="!ttsSupported" class="mobile-more-hint">当前浏览器不支持系统朗读，听书入口已禁用。</p>
</template>

<script setup>
import {
  ArrowDownBold,
  ArrowUpBold,
  Delete,
  Download,
  EditPen,
  Grid,
  Headset,
  InfoFilled,
  Moon,
  Notebook,
  RefreshRight,
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
})

defineEmits(['action'])
</script>

<style scoped>
.mobile-more-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  padding: 4px 0 10px;
}

.mobile-more-item {
  display: grid;
  min-height: 72px;
  place-items: center;
  align-content: center;
  gap: 7px;
  color: #232323;
  background: var(--reader-popup-bg);
  border: 1px solid #eee4c9;
  border-radius: 8px;
  font-size: 13px;
}

.mobile-more-item:active {
  background: rgba(114, 91, 43, 0.1);
}

.mobile-more-item.active {
  color: #0f5451;
  border-color: #0f5451;
  background: color-mix(in srgb, var(--reader-popup-bg) 90%, #fff1bc);
}

.mobile-more-item:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.mobile-more-hint {
  margin: 4px 0 0;
  color: #8a8171;
  font-size: 12px;
  line-height: 1.6;
}
</style>
