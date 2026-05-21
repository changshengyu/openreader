<template>
  <el-alert
    class="source-alert"
    type="info"
    :closable="false"
    show-icon
    title="按当前书名搜索候选书源，切换时会使用候选书籍地址重新抓取目录。"
  />
  <div class="drawer-actions">
    <el-select
      :model-value="group"
      size="small"
      placeholder="全部分组"
      clearable
      class="source-group-select"
      @update:model-value="$emit('groupChange', $event || '')"
    >
      <el-option v-for="item in groups" :key="item" :label="item" :value="item" />
    </el-select>
    <el-button size="small" :loading="loading" @click="$emit('refresh')">刷新</el-button>
    <el-button size="small" :loading="loading" @click="$emit('loadMore')">加载更多</el-button>
    <el-button v-if="showInfoButton" size="small" @click="$emit('showInfo')">书籍信息</el-button>
  </div>
  <section v-if="book" class="current-source-card">
    <div>
      <strong>{{ currentSourceName || '当前来源' }}</strong>
      <span>{{ book.title }}<template v-if="book.author"> · {{ book.author }}</template></span>
    </div>
    <el-tag size="small" effect="plain" type="success">当前</el-tag>
  </section>
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
      <small>{{ source.current ? '当前来源' : (changingSource === source.sourceId ? '切换中...' : '点击切换') }}</small>
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
  group: {
    type: String,
    default: '',
  },
  groups: {
    type: Array,
    default: () => [],
  },
  changingSource: {
    type: [Number, String],
    default: null,
  },
  currentSourceName: {
    type: String,
    default: '',
  },
  showInfoButton: {
    type: Boolean,
    default: true,
  },
})

defineEmits(['refresh', 'loadMore', 'groupChange', 'showInfo', 'change'])
</script>

<style scoped>
.source-alert {
  margin-bottom: 12px;
}

.drawer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}

.source-group-select {
  width: 132px;
}

.source-switch-list {
  display: grid;
  gap: 10px;
}

.current-source-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  padding: 10px 12px;
  color: #24282c;
  background: #fff7dc;
  border: 1px solid #dfc98a;
  border-radius: 6px;
}

.current-source-card div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.current-source-card strong,
.current-source-card span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.current-source-card span {
  color: #7b715e;
  font-size: 12px;
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
