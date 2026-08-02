<template>
  <span
    class="book-cover-shared"
    :class="[`size-${size}`, { 'has-cover': imageLoaded }]"
  >
    <img
      v-if="coverUrl && !imageFailed"
      :key="coverUrl"
      :src="coverUrl"
      :alt="decorative ? '' : coverAlt"
      :aria-hidden="decorative ? 'true' : undefined"
      :class="{ loaded: imageLoaded }"
      @load="handleImageLoad"
      @error="handleImageError"
    />
    <span v-if="!imageLoaded && !decorative" class="cover-fallback">{{ fallbackText }}</span>
  </span>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { bookCoverUrl } from '../utils/bookCover'

const props = defineProps({
  book: {
    type: Object,
    default: () => ({}),
  },
  size: {
    type: String,
    default: 'md',
  },
  fallbackText: {
    type: String,
    default: '暂无封面',
  },
  decorative: {
    type: Boolean,
    default: false,
  },
})

const coverUrl = computed(() => bookCoverUrl(props.book))
const imageLoaded = ref(false)
const imageFailed = ref(false)
const coverAlt = computed(() => String(props.book?.title || props.book?.name || props.book?.bookName || '书籍封面'))

watch(coverUrl, () => {
  imageLoaded.value = false
  imageFailed.value = false
})

function handleImageLoad(event) {
  if (event.currentTarget?.getAttribute('src') !== coverUrl.value) return
  imageLoaded.value = true
  imageFailed.value = false
}

function handleImageError(event) {
  if (event.currentTarget?.getAttribute('src') !== coverUrl.value) return
  imageLoaded.value = false
  imageFailed.value = true
}
</script>

<style scoped>
.book-cover-shared {
  position: relative;
  display: grid;
  width: 72px;
  height: 96px;
  overflow: hidden;
  place-items: center;
  flex: 0 0 auto;
  border-radius: 5px;
  color: #8f866f;
  background:
    radial-gradient(circle at 76% 18%, rgba(203, 186, 132, 0.22), transparent 24%),
    linear-gradient(135deg, #fbfaf4 0%, #f4f0df 100%);
  border: 1px solid rgba(190, 178, 142, 0.32);
  box-shadow: 0 10px 24px rgba(58, 41, 10, 0.12);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.35;
  text-align: center;
  writing-mode: vertical-rl;
}

.book-cover-shared img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  opacity: 0;
  transition: opacity 120ms ease;
}

.book-cover-shared img.loaded {
  opacity: 1;
}

.cover-fallback {
  position: relative;
  z-index: 1;
}

.book-cover-shared.has-cover {
  border-color: transparent;
  color: transparent;
  writing-mode: initial;
}

.book-cover-shared.size-small {
  width: 44px;
  height: 58px;
  font-size: 13px;
}

.book-cover-shared.size-book-info {
  width: auto;
  min-width: 100px;
  max-width: 100%;
  height: 150px;
  overflow: hidden;
  border-radius: 0;
}

.book-cover-shared.size-book-info img {
  position: relative;
  inset: auto;
  width: auto;
  max-width: 100%;
  height: 150px;
  object-fit: contain;
}
</style>
