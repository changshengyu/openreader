<template>
  <el-input v-model="keyword" placeholder="搜索章节..." clearable size="small" class="toc-search" />
  <div ref="tocListRef" class="toc-list">
    <button
      v-for="item in filteredChapters"
      :key="item.id"
      class="toc-item"
      :class="{ active: item.index === currentIndex }"
      :data-chapter-index="item.index"
      type="button"
      @click="$emit('jump', item.index)"
    >
      <span class="toc-main">
        <span>{{ item.title }}</span>
        <small>第 {{ item.index + 1 }} 章<template v-if="showMeta"> · {{ isCached(item) ? '已缓存' : '未缓存' }}</template></small>
      </span>
      <el-tag v-if="!showMeta && isCached(item)" size="small" type="success" effect="plain">{{ browserCachedMap[item.index] ? '本地' : '已缓存' }}</el-tag>
    </button>
    <el-empty v-if="keyword && !filteredChapters.length" description="没有匹配章节" />
  </div>
</template>

<script setup>
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
  modelValue: {
    type: String,
    default: '',
  },
  reverse: {
    type: Boolean,
    default: false,
  },
  showMeta: {
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
})

const emit = defineEmits(['update:modelValue', 'jump'])

const keyword = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})

const filteredChapters = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  const list = value
    ? props.chapters.filter(chapter => String(chapter.title || '').toLowerCase().includes(value))
    : props.chapters
  return props.reverse ? [...list].reverse() : list
})

function isCached(item) {
  return Boolean(item?.cachePath || props.browserCachedMap?.[item?.index])
}

const tocListRef = ref(null)
let locateTimer = 0

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
    const targetTop = active.offsetTop - Math.max(0, (list.clientHeight - active.clientHeight) / 2)
    const nextTop = Math.max(0, targetTop)
    list.scrollTo({ top: nextTop, behavior: 'auto' })
    requestAnimationFrame(() => {
      list.scrollTop = nextTop
      active.scrollIntoView({ block: 'center', inline: 'nearest' })
      requestAnimationFrame(() => {
        list.scrollTop = nextTop
      })
    })
  })
}

defineExpose({
  locateCurrentChapter,
})

onMounted(locateCurrentChapter)
onBeforeUnmount(() => {
  if (locateTimer) window.clearTimeout(locateTimer)
})

watch(
  () => [props.currentIndex, props.locateKey, filteredChapters.value.length],
  () => locateCurrentChapter(),
)
</script>

<style scoped>
.toc-search {
  margin-bottom: 12px;
}

.toc-list {
  max-height: calc(100vh - 160px);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.toc-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: 52px;
  padding: 9px 8px;
  color: inherit;
  background: transparent;
  cursor: pointer;
  border: 0;
  border-bottom: 1px solid #f0f0f0;
  font-size: 14px;
  text-align: left;
}

.toc-main {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.toc-main span,
.toc-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.toc-item small {
  flex: 0 0 auto;
  color: var(--app-text-muted, #909399);
  font-size: 12px;
}

.toc-item:hover {
  color: #409eff;
  background: #f5f7fa;
}

.toc-item.active {
  color: #409eff;
  font-weight: 600;
  background: #ecf5ff;
}

@media (max-width: 1180px), (hover: none) and (pointer: coarse), (any-pointer: coarse) {
  .toc-search {
    margin-bottom: 8px;
  }

  .toc-list {
    max-height: calc(82vh - 76px - env(safe-area-inset-bottom));
    padding-bottom: max(8px, env(safe-area-inset-bottom));
  }

  .toc-item {
    min-height: 48px;
    padding: 8px 2px;
  }
}
</style>
