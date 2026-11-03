<template>
  <button
    class="reader-workspace-dismiss"
    type="button"
    aria-label="关闭阅读工具面板"
    @pointerdown.stop
    @click.stop="$emit('close')"
  />
  <section
    class="reader-desktop-workspace"
    :class="`workspace-panel-${panel}`"
    :aria-label="title || '阅读设置'"
  >
    <slot />
  </section>
  <!-- OpenReader keeps one visible close button for touch iPads (user decision D2), beside the panel. -->
  <button
    class="reader-desktop-workspace-close"
    type="button"
    aria-label="关闭阅读工具面板"
    @pointerdown.stop
    @click.stop="$emit('close')"
  >
    ×
  </button>
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
  z-index: 3;
  inset: 0;
  padding: 0;
  background: transparent;
  border: 0;
}

/* reader-dev's desktop popover face: the panels' `.popup-wrapper` overhangs the popover by 4px, so it
   covers the reading frame inside its border (x = center − readWidthConfig/2, readWidthConfig − 1 wide)
   from the top; text starts 20px down and 24px in, with el-popover's text defaults. */
.reader-desktop-workspace {
  position: fixed;
  z-index: 4;
  top: 0;
  left: 50%;
  box-sizing: border-box;
  width: calc(var(--reader-frame-width) - 1px);
  max-height: 100dvh;
  margin-left: calc(var(--reader-frame-width) / -2);
  padding: 20px 24px 24px;
  color: #606266;
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  font-family: initial;
  font-size: 14px;
  line-height: 1.4;
  text-align: justify;
  word-break: break-all;
}

/* Over the right tool column's empty top, never over the panel. */
.reader-desktop-workspace-close {
  position: fixed;
  top: 8px;
  left: 50%;
  z-index: 5;
  display: flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  margin-left: calc(var(--reader-frame-width) / 2 + 8px);
  padding: 0;
  color: var(--reader-chrome-text, #121212);
  background-color: var(--reader-popup-bg);
  background-image: var(--reader-popup-bg-image);
  background-repeat: repeat;
  border: 0;
  border-radius: 100%;
  cursor: pointer;
  font: inherit;
  font-size: 26px;
  line-height: 1;
  touch-action: manipulation;
}

.reader-desktop-workspace-close:focus-visible {
  outline: 2px solid currentColor;
  outline-offset: 2px;
}

@media (max-width: 750px) {
  .reader-workspace-dismiss,
  .reader-desktop-workspace,
  .reader-desktop-workspace-close {
    display: none;
  }
}
</style>
