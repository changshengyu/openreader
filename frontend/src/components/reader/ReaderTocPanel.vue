<template>
  <!-- reader-dev PopCatalog.vue: its own title zone over a 300px list of 40px chapter rows. -->
  <div class="reader-toc-popup">
    <div class="title-zone">
      <div class="title">目录 <span v-if="chapters.length">({{ chapters.length }})</span></div>
      <div class="title-btn">
        <button v-if="chapters.length" class="span-btn" type="button" @click="$emit('toggle-reverse')">{{ reverse ? '顺序' : '倒序' }}</button>
        <button v-if="chapters.length" class="span-btn" type="button" @click="scrollToTop">顶部</button>
        <button v-if="chapters.length" class="span-btn" type="button" @click="scrollToBottom">底部</button>
        <button v-if="canChangeRule" class="span-btn" type="button" :disabled="refreshing" @click="$emit('change-rule')">修改规则</button>
        <button class="refresh-btn" :class="{ loading: refreshing }" type="button" :disabled="refreshing" @click="$emit('refresh')">
          <el-icon v-if="refreshing" class="is-loading"><Loading /></el-icon>
          {{ refreshing ? '刷新中...' : '刷新' }}
        </button>
      </div>
    </div>
    <div ref="tocListRef" class="toc-list">
      <button
        v-for="item in orderedChapters"
        :key="item.id"
        class="toc-item"
        :class="{ active: item.index === currentIndex }"
        :data-chapter-index="item.index"
        type="button"
        @click="$emit('jump', item.index)"
      >
        <span class="log-text" :class="{ cached: isBrowserCached(item) }">{{ item.title }}</span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { Loading } from '@element-plus/icons-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  chapters: {
    type: Array,
    default: () => [],
  },
  currentIndex: {
    type: Number,
    default: 0,
  },
  reverse: {
    type: Boolean,
    default: false,
  },
  locateKey: {
    type: Number,
    default: 0,
  },
  browserCachedMap: {
    type: Object,
    default: () => ({}),
  },
  canChangeRule: {
    type: Boolean,
    default: false,
  },
  refreshing: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['jump', 'toggle-reverse', 'change-rule', 'refresh'])

const orderedChapters = computed(() => (props.reverse ? [...props.chapters].reverse() : props.chapters))

// reader-dev PopCatalog dims only chapters held in the browser cache (cachedCataMap);
// every imported local-book chapter also has a server cachePath.
function isBrowserCached(item) {
  return Boolean(props.browserCachedMap?.[item?.index])
}

const tocListRef = ref(null)
let locateTimer = 0

// reader-dev PopCatalog.vue#jumpToCurrent: the row scrolls to the top of the list.
function scrollRowToTop(row) {
  const list = tocListRef.value
  if (!list || !row) return
  list.scrollTop = Math.max(
    0,
    list.scrollTop + row.getBoundingClientRect().top - list.getBoundingClientRect().top,
  )
}

function locateCurrentChapter(attempt = 0) {
  if (locateTimer) window.clearTimeout(locateTimer)
  nextTick(() => {
    const list = tocListRef.value
    const active = list?.querySelector?.(`[data-chapter-index="${props.currentIndex}"]`)
    if (!list || !active) {
      if (attempt < 20 && props.chapters.length) {
        locateTimer = window.setTimeout(() => locateCurrentChapter(attempt + 1), 50)
      }
      return
    }
    scrollRowToTop(active)
  })
}

defineExpose({
  locateCurrentChapter,
  scrollToTop,
  scrollToBottom,
})

onMounted(locateCurrentChapter)
onBeforeUnmount(() => {
  if (locateTimer) window.clearTimeout(locateTimer)
})

watch(
  () => [props.currentIndex, props.locateKey, orderedChapters.value.length],
  () => locateCurrentChapter(),
)

function scrollToTop() {
  scrollRowToTop(tocListRef.value?.firstElementChild)
}

function scrollToBottom() {
  scrollRowToTop(tocListRef.value?.lastElementChild)
}
</script>

<style scoped>
.title-zone {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: space-between;
  width: 100%;
  margin: 0 0 20px;
}

.title {
  width: fit-content;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-family: -apple-system, "Noto Sans", "Helvetica Neue", Helvetica, "Nimbus Sans L", Arial, "Liberation Sans", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", "Source Han Sans SC", "Source Han Sans CN", "Microsoft YaHei", "Wenquanyi Micro Hei", "WenQuanYi Zen Hei", "ST Heiti", SimHei, "WenQuanYi Zen Hei Sharp", sans-serif;
  font-size: 18px;
  font-weight: 400;
}

.title-btn {
  color: #606266;
  font-size: 14px;
  line-height: 26px;
}

.title-btn button {
  display: inline-block;
  margin-left: 15px;
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  line-height: inherit;
}

.title-btn .refresh-btn.loading {
  color: #606266;
}

.title-btn button:disabled {
  cursor: default;
}

.toc-list {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  align-content: flex-start;
  justify-content: space-between;
  height: 300px;
  overflow: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
}

.toc-list::-webkit-scrollbar {
  width: 0;
}

.toc-item {
  box-sizing: content-box;
  display: block;
  width: 50%;
  height: 40px;
  padding: 0;
  color: var(--reader-list-text, inherit);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--reader-list-border, #eee);
  cursor: pointer;
  font: 16px / 40px PingFangSC-Regular, HelveticaNeue-Light, 'Helvetica Neue Light', 'Microsoft YaHei', sans-serif;
  text-align: inherit;
}

.toc-item .log-text {
  display: block;
  overflow: hidden;
  margin-right: 26px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Before the current chapter, which overrides it at the same specificity. */
.toc-item .log-text.cached {
  color: var(--reader-list-cached-text, #bbb);
}

.toc-item.active .log-text {
  color: #eb4259;
}

@media (max-width: 500px) {
  .toc-item {
    width: 100%;
  }

  .toc-item .log-text {
    margin-right: 0;
  }
}
</style>
