<template>
  <div class="rss-source-list-container">
    <div
      v-for="source in sources"
      :key="source.id || source.sourceUrl || source.url"
      class="rss-source"
      role="button"
      tabindex="0"
      @click="$emit('open', source)"
      @keydown.enter.prevent="$emit('open', source)"
      @keydown.space.prevent="$emit('open', source)"
    >
      <button
        v-if="editMode"
        type="button"
        class="rss-source-delete"
        aria-label="删除 RSS 源"
        @click.stop="$emit('remove', source)"
      >×</button>
      <button
        v-if="editMode"
        type="button"
        class="rss-source-edit"
        aria-label="编辑 RSS 源"
        @click.stop="$emit('edit', source)"
      >✎</button>
      <el-image
        :src="source.sourceIcon || source.icon || ''"
        class="rss-icon"
        fit="cover"
        lazy
      />
      <div class="rss-title">{{ source.sourceName || source.title }}</div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  sources: {
    type: Array,
    default: () => [],
  },
  editMode: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['open', 'edit', 'remove'])
</script>

<style scoped>
.rss-source-list-container {
  max-height: calc(var(--vh, 1vh) * 70 - 114px);
  overflow-y: auto;
}

.rss-source {
  position: relative;
  display: inline-block;
  width: 25%;
  box-sizing: border-box;
  margin-bottom: 10px;
  padding: 10px;
  color: var(--app-text);
  text-align: center;
  vertical-align: top;
  cursor: pointer;
}

.rss-icon {
  display: inline-block;
  width: 50px;
  height: 50px;
  border-radius: 5px;
}

.rss-title {
  margin-top: 5px;
  overflow-wrap: anywhere;
  text-align: center;
}

.rss-source-delete,
.rss-source-edit {
  position: absolute;
  right: 6px;
  z-index: 1;
  width: 22px;
  height: 22px;
  padding: 0;
  color: var(--app-text);
  font-size: 18px;
  line-height: 20px;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.rss-source-delete {
  top: 8px;
}

.rss-source-edit {
  top: 42px;
}

@media (max-width: 750px) {
  .rss-source-list-container {
    max-height: calc(var(--vh, 1vh) * 100 - 94px);
  }
}

@media (max-width: 480px) {
  .rss-source-delete,
  .rss-source-edit {
    right: -5px;
  }
}
</style>
