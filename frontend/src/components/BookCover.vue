<template>
  <span class="book-cover-shared" :class="[`size-${size}`, { 'has-cover': hasCover }]" :style="coverStyle">{{ coverText }}</span>
</template>

<script setup>
import { computed } from 'vue'
import { bookCoverUrl, hasBookCover } from '../utils/bookCover'

const props = defineProps({
  book: {
    type: Object,
    default: () => ({}),
  },
  size: {
    type: String,
    default: 'md',
  },
})

const hasCover = computed(() => hasBookCover(props.book))
const coverText = computed(() => (hasCover.value ? '' : '暂无封面'))
const coverUrl = computed(() => bookCoverUrl(props.book))

const coverStyle = computed(() => {
  if (hasCover.value) {
    return {
      backgroundImage: `url(${coverUrl.value})`,
      backgroundPosition: 'center',
      backgroundSize: 'cover',
      color: 'transparent',
    }
  }
  return {}
})
</script>

<style scoped>
.book-cover-shared {
  display: grid;
  width: 72px;
  height: 96px;
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

.book-cover-shared.has-cover {
  border-color: transparent;
  writing-mode: initial;
}

.book-cover-shared.size-small {
  width: 44px;
  height: 58px;
  font-size: 13px;
}
</style>
