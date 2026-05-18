<template>
  <el-alert
    class="source-alert"
    type="info"
    :closable="false"
    show-icon
    title="按当前书名搜索候选书源，切换时会使用候选书籍地址重新抓取目录。"
  />
  <div class="drawer-actions">
    <el-button size="small" :loading="loading" @click="$emit('refresh')">搜索更多来源</el-button>
    <el-button size="small" @click="$emit('showInfo')">书籍信息</el-button>
  </div>
  <div class="source-switch-list">
    <button
      v-for="source in sources"
      :key="`${source.sourceId}-${source.bookUrl}`"
      class="source-switch-card"
      :class="{ active: source.current }"
      type="button"
      :disabled="source.current || changingSource === source.sourceId"
      @click="$emit('change', source)"
    >
      <strong>{{ source.title || book?.title }}</strong>
      <span>{{ source.sourceName }} · {{ source.author || '未知作者' }}</span>
      <small>{{ source.current ? '当前来源' : '点击切换' }}</small>
    </button>
    <el-empty v-if="!loading && !sources.length" description="没有找到可用来源" />
  </div>
</template>

<script setup>
defineProps({
  book: {
    type: Object,
    default: null,
  },
  sources: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
  changingSource: {
    type: [Number, String],
    default: null,
  },
})

defineEmits(['refresh', 'showInfo', 'change'])
</script>

<style scoped>
.source-alert {
  margin-bottom: 12px;
}

.drawer-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 14px;
}

.source-switch-list {
  display: grid;
  gap: 10px;
}

.source-switch-card {
  display: grid;
  gap: 5px;
  width: 100%;
  padding: 12px;
  color: #24282c;
  background: #fffaf0;
  border: 1px solid #eee4c9;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
}

.source-switch-card:hover,
.source-switch-card.active {
  border-color: #0f5451;
  background: #fff7dc;
}

.source-switch-card:disabled {
  cursor: progress;
  opacity: 0.7;
}

.source-switch-card span,
.source-switch-card small {
  color: #7b715e;
  font-size: 12px;
}
</style>
