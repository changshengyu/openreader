<template>
  <!-- reader-dev Reader.vue right `.read-bar` `.tools`: book progress, previous and next chapter. -->
  <footer class="reader-page-control">
    <ReaderCachePanel
      class="desktop-cache-zone"
      :visible="cacheVisible"
      :caching="caching"
      :status-text="cacheStatusText"
      @cache="$emit('cache', $event)"
      @cancel="$emit('cache-cancel')"
    />
    <button class="progress-box" type="button" @click="$emit('cache-toggle')">{{ bookProgressLabel }}</button>
    <button class="page-step chapter-step" type="button" aria-label="上一章" @click="$emit('previous')">
      <span class="iconfont">&#58920;</span>
    </button>
    <button class="page-step chapter-step" type="button" aria-label="下一章" @click="$emit('next')">
      <span class="iconfont">&#58913;</span>
    </button>
  </footer>
</template>

<script setup>
import ReaderCachePanel from './ReaderCachePanel.vue'

defineProps({
  bookProgressLabel: {
    type: String,
    default: '',
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

defineEmits([
  'cache-toggle',
  'cache',
  'cache-cancel',
  'previous',
  'next',
])
</script>

<style scoped>
/* reader-dev Reader.vue desktop right `.read-bar`: its right edge is readWidthConfig/2 + 52px right of
   the center and it is as wide as its 42px tools plus their borders. */
.reader-page-control {
  position: fixed;
  bottom: 0;
  left: 50%;
  z-index: 5;
  display: flex;
  flex-direction: column;
  margin-left: calc(var(--reader-frame-width) / 2 + 8px);
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  font-family: "Avenir", Helvetica, Arial, sans-serif;
}

.desktop-cache-zone {
  position: absolute;
  top: 0;
  right: 55px;
  box-sizing: content-box;
  width: 300px;
}

/* A button centers its content; reader-dev's div stacks it from the top. */
.progress-box,
.page-step {
  box-sizing: content-box;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  width: 42px;
  height: 31px;
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

.progress-box {
  font-size: 16px;
}

.page-step .iconfont {
  display: block;
  width: 16px;
  height: 16px;
  margin: 0 auto 6px;
  font-family: iconfont;
  font-size: 16px;
}

@media (max-width: 750px) {
  .reader-page-control {
    display: none;
  }
}
</style>
