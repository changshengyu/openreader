<template>
  <div class="reader-tap-zones" :class="mode" aria-hidden="true">
    <button class="tap-zone tap-left" type="button" tabindex="-1" @click="emit('tap', 'left')" />
    <button class="tap-zone tap-center" type="button" tabindex="-1" @click="emit('tap', 'center')" />
    <button class="tap-zone tap-right" type="button" tabindex="-1" @click="emit('tap', 'right')" />
    <button class="tap-zone tap-upper" type="button" tabindex="-1" @click="emit('tap', 'upper')" />
    <button class="tap-zone tap-lower" type="button" tabindex="-1" @click="emit('tap', 'lower')" />
  </div>

  <div v-if="showOverlay" class="click-zone-overlay" :class="{ flip: mode === 'flip' }">
    <div class="click-zone-piece click-zone-prev">
      <span>{{ mode === 'flip' ? '点击前一页' : '点击向上翻页' }}</span>
    </div>
    <div class="click-zone-piece click-zone-menu"><span>点击显示菜单</span></div>
    <div class="click-zone-piece click-zone-next">
      <span>{{ mode === 'flip' ? '点击后一页' : '点击向下翻页' }}</span>
    </div>
    <button class="click-zone-close" type="button" @click="emit('close-overlay')">关闭</button>
  </div>
</template>

<script setup>
defineProps({
  mode: {
    type: String,
    required: true,
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
  width: 24%;
}

.tap-right {
  top: 0;
  right: 0;
  bottom: 0;
  width: 24%;
}

.tap-center {
  top: 35%;
  right: 24%;
  bottom: 35%;
  left: 24%;
}

.tap-upper {
  top: 0;
  right: 24%;
  left: 24%;
  height: 35%;
}

.tap-lower {
  right: 24%;
  bottom: 0;
  left: 24%;
  height: 35%;
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

.click-zone-overlay {
  position: absolute;
  inset: 0;
  z-index: 30;
  display: grid;
  grid-template-rows: 35% 30% 35%;
  background: rgba(20, 20, 20, 0.08);
}

.click-zone-overlay.flip {
  grid-template-columns: 24% 52% 24%;
  grid-template-rows: 1fr;
}

.click-zone-piece {
  display: grid;
  place-items: center;
  border: 1px dashed rgba(237, 66, 89, 0.55);
  background: rgba(237, 66, 89, 0.08);
  color: #ed4259;
  font-size: 16px;
  pointer-events: none;
}

.click-zone-piece span {
  border-radius: 999px;
  padding: 8px 14px;
  background: rgba(255, 255, 255, 0.82);
}

.click-zone-overlay.flip .click-zone-prev { grid-column: 1; }
.click-zone-overlay.flip .click-zone-menu { grid-column: 2; }
.click-zone-overlay.flip .click-zone-next { grid-column: 3; }

.click-zone-close {
  position: absolute;
  right: 18px;
  bottom: 18px;
  border: 0;
  border-radius: 999px;
  padding: 8px 16px;
  background: #ed4259;
  color: #fff;
  cursor: pointer;
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
