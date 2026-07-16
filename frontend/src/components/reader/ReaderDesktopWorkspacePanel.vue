<template>
  <button
    class="reader-workspace-dismiss"
    type="button"
    aria-label="关闭阅读工具面板"
    @click="$emit('close')"
  />
  <section
    class="reader-desktop-workspace"
    :class="[{ 'without-head': !title }, `workspace-panel-${panel}`]"
    :aria-label="title || '阅读设置'"
  >
    <header v-if="title" class="reader-workspace-head">
      <strong>{{ title }}</strong>
      <div class="reader-workspace-actions">
        <slot name="actions" />
      </div>
    </header>
    <div class="reader-workspace-body">
      <slot />
    </div>
  </section>
</template>

<script setup>
defineProps({
  panel: {
    type: String,
    default: '',
  },
  title: {
    type: String,
    default: '',
  },
})

defineEmits(['close'])
</script>

<style scoped>
.reader-workspace-dismiss {
  position: fixed;
  z-index: 2;
  inset: 0;
  padding: 0;
  background: transparent;
  border: 0;
}

.reader-desktop-workspace {
  position: fixed;
  z-index: 3;
  top: 0;
  left: calc(50vw - var(--reader-frame-width) / 2 + 4px);
  width: calc(var(--reader-frame-width) - 9px);
  height: auto;
  min-height: 0;
  max-height: 100dvh;
  box-sizing: border-box;
  padding: 24px;
  color: var(--reader-text);
  background-color: var(--reader-bg);
  background-image: var(--reader-bg-image, var(--paper-texture));
  background-size: cover;
  border-right: 1px solid rgba(109, 95, 55, 0.28);
  border-left: 1px solid rgba(109, 95, 55, 0.28);
  box-shadow:
    inset 24px 0 44px rgba(90, 71, 28, 0.05),
    inset -24px 0 44px rgba(90, 71, 28, 0.05);
  filter: brightness(var(--reader-brightness));
  overflow: visible;
}

.reader-workspace-head {
  display: flex;
  min-height: 34px;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 10px;
  color: #ed4259;
}

.reader-workspace-head strong {
  font-size: 18px;
  font-weight: 500;
}

.reader-workspace-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 14px;
}

.reader-workspace-actions :deep(button) {
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
}

.reader-workspace-actions :deep(button:disabled) {
  color: #aaa;
  cursor: default;
}

.reader-workspace-body {
  height: auto;
  min-height: 0;
  max-height: calc(100dvh - 48px);
  overflow: visible;
}

.reader-desktop-workspace.workspace-panel-shelf :deep(.reader-shelf-list),
.reader-desktop-workspace.workspace-panel-toc :deep(.toc-list),
.reader-desktop-workspace.workspace-panel-source :deep(.source-switch-list) {
  height: 300px;
  max-height: 300px;
  min-height: 0;
}

.reader-desktop-workspace.workspace-panel-settings .reader-workspace-body {
  max-height: none;
  overflow: visible;
}

@media (max-width: 750px) {
  .reader-workspace-dismiss,
  .reader-desktop-workspace {
    display: none;
  }
}
</style>
