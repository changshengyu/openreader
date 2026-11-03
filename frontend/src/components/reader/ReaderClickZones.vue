<template>
  <div class="reader-tap-zones" :class="[mode, { 'document-scroll': documentScroll }]" aria-hidden="true">
    <button class="tap-zone tap-left" type="button" tabindex="-1" @click="emit('tap', 'left')" />
    <button class="tap-zone tap-center" type="button" tabindex="-1" @click="emit('tap', 'center')" />
    <button class="tap-zone tap-right" type="button" tabindex="-1" @click="emit('tap', 'right')" />
    <button class="tap-zone tap-upper" type="button" tabindex="-1" @click="emit('tap', 'upper')" />
    <button class="tap-zone tap-lower" type="button" tabindex="-1" @click="emit('tap', 'lower')" />
  </div>

  <!-- reader-dev Reader.vue `.click-zone`: shows the zones without catching taps; only 关闭 is pressable. -->
  <div v-if="showOverlay" class="click-zone-overlay" :class="{ flip: mode === 'flip' }">
    <div class="click-zone-piece click-zone-prev"><span>点击前一页</span></div>
    <div class="click-zone-piece click-zone-menu"><span>点击显示菜单</span></div>
    <div class="click-zone-piece click-zone-next"><span>点击后一页</span></div>
    <button class="click-zone-close" type="button" @click="emit('close-overlay')">关闭</button>
  </div>
</template>

<script setup>
defineProps({
  mode: {
    type: String,
    required: true,
  },
  documentScroll: {
    type: Boolean,
    default: false,
  },
  showOverlay: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['tap', 'close-overlay'])
</script>

<style scoped>
.reader-tap-zones {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: none;
  pointer-events: none;
}

.reader-tap-zones.document-scroll {
  position: fixed;
  top: 0;
  right: auto;
  bottom: 0;
  left: 50%;
  width: var(--reader-frame-width);
  transform: translateX(-50%);
}

.tap-zone {
  position: absolute;
  padding: 0;
  background: transparent;
  border: 0;
  cursor: pointer;
  pointer-events: auto;
}

.tap-left {
  top: 0;
  bottom: 0;
  left: 0;
  width: 30%;
}

.tap-right {
  top: 0;
  right: 0;
  bottom: 0;
  width: 30%;
}

.tap-center {
  top: 30%;
  right: 30%;
  bottom: 30%;
  left: 30%;
}

.tap-upper {
  top: 0;
  right: 30%;
  left: 30%;
  height: 30%;
}

.tap-lower {
  right: 30%;
  bottom: 0;
  left: 30%;
  height: 30%;
}

.reader-tap-zones.scroll .tap-left,
.reader-tap-zones.scroll .tap-right,
.reader-tap-zones.scroll2 .tap-left,
.reader-tap-zones.scroll2 .tap-right,
.reader-tap-zones.page .tap-left,
.reader-tap-zones.page .tap-right {
  display: none;
}

.reader-tap-zones.scroll .tap-upper,
.reader-tap-zones.scroll .tap-lower,
.reader-tap-zones.scroll2 .tap-upper,
.reader-tap-zones.scroll2 .tap-lower,
.reader-tap-zones.page .tap-upper,
.reader-tap-zones.page .tap-lower {
  right: 0;
  left: 0;
}

.reader-tap-zones.flip .tap-upper,
.reader-tap-zones.flip .tap-lower {
  display: none;
}

/* reader-dev Reader.vue `.click-zone`: fixed over the window (absolute inside the slide reader), the
   previous/next halves and the 30%–70% menu box over a #333 veil at 0.8 opacity. */
.click-zone-overlay {
  position: fixed;
  inset: 0;
  z-index: 11;
  color: #fff;
  background: #333;
  font-size: 14px;
  opacity: 0.8;
  pointer-events: none;
}

.click-zone-overlay.flip {
  position: absolute;
}

.click-zone-piece {
  position: absolute;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.click-zone-prev {
  top: 0;
  right: 0;
  bottom: 50%;
  left: 0;
  background: #43987324;
}

.click-zone-next {
  top: 50%;
  right: 0;
  bottom: 0;
  left: 0;
  background: #6b1a7324;
}

.click-zone-overlay.flip .click-zone-prev {
  right: 50%;
  bottom: 0;
  padding-right: 20%;
}

.click-zone-overlay.flip .click-zone-next {
  top: 0;
  left: 50%;
  padding-left: 20%;
}

.click-zone-menu {
  z-index: 10;
  top: 30%;
  right: 30%;
  bottom: 30%;
  left: 30%;
  background: #636060;
}

.click-zone-close {
  position: absolute;
  right: 0;
  bottom: 20px;
  left: 0;
  z-index: 10;
  height: 45px;
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  line-height: 45px;
  pointer-events: all;
}

@media (hover: hover) and (pointer: fine) {
  .reader-tap-zones {
    display: block;
  }

  .tap-zone {
    display: none;
  }

  .tap-center {
    display: block;
    pointer-events: none;
  }

  .reader-tap-zones.scroll .tap-upper,
  .reader-tap-zones.scroll .tap-lower,
  .reader-tap-zones.scroll2 .tap-upper,
  .reader-tap-zones.scroll2 .tap-lower,
  .reader-tap-zones.page .tap-upper,
  .reader-tap-zones.page .tap-lower {
    display: block;
  }

  .reader-tap-zones.flip .tap-left,
  .reader-tap-zones.flip .tap-right {
    display: block;
  }
}

@media (max-width: 750px) {
  .reader-tap-zones {
    display: none;
  }
}
</style>
