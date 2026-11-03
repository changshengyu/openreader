<template>
  <section
    class="reader-mobile-workspace reader-mobile-workspace-primary"
    :class="`reader-mobile-primary-${panel}`"
    role="dialog"
    aria-modal="false"
    :aria-label="title"
    @click.stop
    @touchstart.stop
    @touchmove.stop
    @touchend.stop
  >
    <slot />
  </section>
</template>

<script setup>
defineProps({
  panel: {
    type: String,
    required: true,
  },
  title: {
    type: String,
    required: true,
  },
})
</script>

<style scoped>
/* reader-dev App.vue mini `.popper-component`: pinned at the top over the toolbar, 100vw wide, no border
   or shadow, with el-popover's text defaults. The inset is the panels' `.popup-wrapper` 24px less its
   4px overhang, measured at 20px around and 23px below. */
.reader-mobile-workspace-primary {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 13;
  box-sizing: border-box;
  width: 100vw;
  max-height: 100dvh;
  padding: calc(20px + env(safe-area-inset-top)) 20px 23px;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
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
</style>
