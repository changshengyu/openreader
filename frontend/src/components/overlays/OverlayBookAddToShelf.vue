<template>
  <el-dialog
    v-model="overlay.bookAddCategoryVisible"
    title="设置分组"
    width="440px"
    :fullscreen="isMobile"
    class="book-add-category-dialog"
    @closed="overlay.finishBookAddCategories()"
  >
    <div class="book-add-category-body">
      <p>加入书架前选择分组（可多选，留空则不分组）。</p>
      <el-select
        v-model="overlay.bookAddCategoryIds"
        multiple
        clearable
        filterable
        placeholder="未分组"
      >
        <el-option
          v-for="category in bookshelf.categories"
          :key="category.id"
          :label="category.name"
          :value="Number(category.id)"
        />
      </el-select>
    </div>

    <template #footer>
      <el-button @click="overlay.finishBookAddCategories()">取消</el-button>
      <el-button type="primary" @click="overlay.finishBookAddCategories(overlay.bookAddCategoryIds)">
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()

watch(
  () => overlay.bookAddCategoryVisible,
  visible => {
    if (!visible) return
    bookshelf.ensureCategoriesLoaded().catch(() => {
      ElMessage.warning('分组加载失败，仍可选择未分组加入书架')
    })
  },
)
</script>

<style scoped>
.book-add-category-body {
  display: grid;
  gap: 12px;
}

.book-add-category-body p {
  margin: 0;
  color: var(--app-text-muted);
  line-height: 1.6;
}

.book-add-category-body :deep(.el-select) {
  width: 100%;
}
</style>
