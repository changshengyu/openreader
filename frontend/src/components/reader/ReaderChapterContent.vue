<template>
  <p v-if="loading" class="empty-hint">正在加载章节...</p>

  <div v-else-if="error" class="chapter-load-error">
    <p>{{ error }}</p>
    <button type="button" @click="emit('reload')">重新加载</button>
  </div>

  <ReaderEpubContent
    v-else-if="epubResource?.url"
    :resource="epubResource"
    :style-text="epubStyle"
    :viewport-height="viewportHeight"
    @ready="emit('epub-ready')"
    @load="emit('epub-load', $event)"
    @height="emit('epub-height', $event)"
    @click-point="emit('epub-click', $event)"
    @hash="emit('epub-hash', $event)"
    @keydown="emit('epub-keydown', $event)"
    @preview="emit('epub-preview', $event)"
    @error="emit('epub-error', $event)"
  />

  <template v-else>
    <section
      v-for="block in blocks"
      :key="block.index"
      class="chapter-content reading-chapter"
      :class="[mode, { 'volume-chapter': block.isVolume, 'comic-chapter': block.isComic }]"
      :data-index="block.index"
    >
      <div v-if="block.isVolume" class="volume-content">
        <h1 data-pos="0">{{ block.title || '正文' }}</h1>
        <p v-if="block.volumeText" class="volume-tag" data-reader-block>{{ block.volumeText }}</p>
      </div>
      <template v-else>
        <h1 v-if="!block.hideTitle" data-pos="0">{{ block.title || '正文' }}</h1>
        <template v-for="(line, index) in block.paragraphs" :key="`${block.index}-${index}`">
        <figure
          v-if="line.type === 'image'"
          class="reader-content-image"
          :class="{ 'is-full': line.imageStyle === 'FULL' }"
          :data-pos="line.pos"
          data-reader-block
          @click.stop
        >
          <el-image
            :src="line.src"
            :alt="line.alt"
            :preview-src-list="block.imageUrls"
            :initial-index="Math.max(0, (block.imageUrls || []).indexOf(line.src))"
            fit="contain"
            lazy
            preview-teleported
            @load="emit('image-load', { blockIndex: block.index, pos: line.pos, src: line.src })"
          />
          <figcaption v-if="line.alt">{{ line.alt }}</figcaption>
        </figure>
        <p v-else-if="line.html" :data-pos="line.pos" data-reader-block v-html="line.html"></p>
        <p v-else :data-pos="line.pos" data-reader-block>{{ line.text }}</p>
        </template>
        <p v-if="loaded && block.paragraphs.length === 0" class="empty-hint">当前章节暂无正文内容</p>
      </template>
    </section>
  </template>
</template>

<script setup>
import ReaderEpubContent from './ReaderEpubContent.vue'

defineProps({
  blocks: {
    type: Array,
    default: () => [],
  },
  error: {
    type: String,
    default: '',
  },
  loaded: {
    type: Boolean,
    default: false,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  mode: {
    type: String,
    required: true,
  },
  epubResource: {
    type: Object,
    default: null,
  },
  epubStyle: {
    type: String,
    default: '',
  },
  viewportHeight: {
    type: Number,
    default: 0,
  },
})

const emit = defineEmits([
  'reload',
  'epub-ready',
  'epub-load',
  'epub-height',
  'epub-click',
  'epub-hash',
  'epub-keydown',
  'epub-preview',
  'epub-error',
  'image-load',
])
</script>

<style scoped>
.chapter-content {
  min-height: 1px;
}

.chapter-content.scroll + .chapter-content,
.chapter-content.scroll2 + .chapter-content {
  padding-top: 58px;
}

.chapter-content.volume-chapter {
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
}

.volume-content {
  width: 100%;
  text-align: center;
}

.volume-tag {
  text-align: right;
  text-indent: 0;
  white-space: pre-line;
}

h1 {
  font-size: var(--reader-heading-size);
  line-height: 1.35;
  margin: 0 0 76px;
  text-align: center;
}

p {
  margin: var(--reader-paragraph-space) 0;
  font-weight: var(--reader-font-weight);
  text-align: inherit;
  text-indent: 2em;
}

.chapter-content.flip h1,
.chapter-content.flip p {
  break-inside: avoid;
}

.reader-content-image {
  display: grid;
  width: 100%;
  margin: 0 auto var(--reader-paragraph-space);
  place-items: center;
  text-indent: 0;
}

.comic-chapter .reader-content-image {
  margin-bottom: 0;
}

.reader-content-image :deep(.el-image) {
  display: block;
  width: min(100%, 960px);
  min-height: 1px;
}

.comic-chapter .reader-content-image :deep(.el-image) {
  width: 100%;
  max-width: 100vw;
}

.reader-content-image.is-full :deep(.el-image) {
  width: 100%;
}

.reader-content-image :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0 auto;
}

.reader-content-image.is-full :deep(img) {
  width: 100%;
}

.comic-chapter .reader-content-image :deep(img) {
  width: 100%;
  max-width: 100vw;
}

.reader-content-image figcaption {
  margin-top: 8px;
  color: rgba(36, 40, 44, 0.55);
  font-size: 0.78em;
  text-align: center;
}

.chapter-load-error {
  display: grid;
  min-height: 180px;
  place-content: center;
  gap: 14px;
  text-align: center;
}

.chapter-load-error p {
  margin: 0;
  color: rgba(112, 48, 42, 0.8);
  text-indent: 0;
}

.chapter-load-error button {
  justify-self: center;
  padding: 8px 18px;
  border: 1px solid currentColor;
  border-radius: 999px;
  color: inherit;
  background: transparent;
  cursor: pointer;
}

p.reader-search-active {
  background: rgba(47, 111, 109, 0.16);
  box-shadow: -8px 0 0 rgba(47, 111, 109, 0.16), 8px 0 0 rgba(47, 111, 109, 0.16);
  transition: background 160ms ease, box-shadow 160ms ease;
}

.empty-hint {
  color: #999;
  text-align: center;
  padding-top: 40px;
  text-indent: 0;
}

@media (max-width: 750px) {
  .chapter-content {
    text-align: justify;
  }
  h1 {
    font-size: var(--reader-heading-size);
    margin-bottom: 28px;
  }
}
</style>
