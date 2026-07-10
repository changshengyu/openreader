<template>
  <section
    class="reader-mobile-workspace"
    :class="{
      'reader-mobile-workspace-no-header': !showHeader,
      'reader-mobile-workspace-primary': primary,
    }"
    role="dialog"
    aria-modal="false"
    :aria-label="title"
    @click.stop
    @touchstart.stop
    @touchmove.stop
    @touchend.stop
  >
    <div v-if="showHeader" class="reader-mobile-workspace-head">
      <div class="reader-mobile-workspace-title">{{ title }}</div>
      <div class="reader-mobile-workspace-actions">
        <slot name="actions" />
        <button type="button" class="reader-mobile-workspace-close" @click="$emit('close')">关闭</button>
      </div>
    </div>
    <div class="reader-mobile-workspace-body">
      <slot />
    </div>
  </section>
</template>

<script setup>
defineProps({
  title: {
    type: String,
    required: true,
  },
  showHeader: {
    type: Boolean,
    default: true,
  },
  primary: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['close'])
</script>

<style scoped>
.reader-mobile-workspace {
  position: fixed;
  inset: 0;
  z-index: 7;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  box-sizing: border-box;
  width: 100vw;
  height: 100dvh;
  padding: calc(58px + env(safe-area-inset-top)) 12px calc(96px + env(safe-area-inset-bottom));
  color: var(--reader-text);
  background: color-mix(in srgb, var(--reader-popup-bg) 97%, transparent);
  backdrop-filter: blur(2px);
}

.reader-mobile-workspace-no-header {
  grid-template-rows: minmax(0, 1fr);
}

.reader-mobile-workspace-primary {
  display: block;
  padding: 0;
  background: var(--reader-popup-bg);
  backdrop-filter: none;
}

.reader-mobile-workspace-primary .reader-mobile-workspace-body {
  width: 100%;
  height: 100dvh;
  min-height: 100dvh;
  overflow: hidden;
}

.reader-mobile-workspace-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  min-width: 0;
  margin-bottom: 10px;
}

.reader-mobile-workspace-title {
  min-width: 0;
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
  line-height: 1.6;
  white-space: nowrap;
}

.reader-mobile-workspace-actions {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  min-width: 0;
}

.reader-mobile-workspace-actions :deep(button),
.reader-mobile-workspace-close {
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
  line-height: 28px;
}

.reader-mobile-workspace-actions :deep(button:disabled),
.reader-mobile-workspace-close:disabled {
  color: #8c8c8c;
  cursor: default;
}

.reader-mobile-workspace-body {
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
}
</style>
