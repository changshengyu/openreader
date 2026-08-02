<template>
  <el-dialog
    v-if="isNormalPage"
    v-model="overlay.bookGroupVisible"
    :title="overlay.bookGroupMode === 'set' ? '设置分组' : '分组管理'"
    width="min(1000px, max(750px, 70vw))"
    top="max(15dvh, calc((100dvh - 584px) / 2))"
    :fullscreen="isMobile"
    destroy-on-close
    class="global-book-group-dialog"
    @opened="handleOpened"
    @closed="handleClosed"
  >
    <section class="book-group-dialog-body">
      <el-table
        ref="groupTableRef"
        :key="isSetMode"
        :data="groupRows"
        :height="isMobile ? 'calc(100dvh - 184px)' : 'min(400px, calc(70dvh - 184px))'"
        class="book-group-table"
        @selection-change="handleSelectionChange"
      >
        <el-table-column
          type="selection"
          width="25"
          v-if="isSetMode"
        />
        <el-table-column
          prop="name"
          label="分组名"
          min-width="100"
        >
          <template #default="{ row }">
            <span class="group-name-cell">
              <span class="group-drag-icon" aria-hidden="true">
                <el-icon><Rank /></el-icon>
              </span>
              <span>{{ displayBookGroupName(row) }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column
          prop="show"
          label="显示"
          min-width="80"
          v-if="!isSetMode"
        >
          <template #default="{ row }">
            <el-switch
              :model-value="row.show !== false"
              :loading="visibilitySavingId === (row.key || row.id)"
              @change="value => toggleVisibility(row, value)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100px">
          <template #default="{ row }">
            <el-button text @click="renameGroup(row)">编辑</el-button>
            <el-button
              v-if="!isSetMode && row.kind === 'category' && groupBookCount(row) === 0"
              text
              type="danger"
              @click="deleteGroup(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <template #footer>
      <div class="dialog-footer">
        <el-button
          type="primary"
          size="default"
          class="float-left"
          @click="createCategory"
        >
          添加分组
        </el-button>
        <el-button
          v-if="isGroupOrderDirty"
          type="primary"
          size="default"
          class="float-left"
          :loading="groupOrderSaving"
          @click="saveOrder"
        >
          保存排序
        </el-button>
        <el-button
          v-if="isSetMode"
          type="primary"
          size="default"
          :loading="settingCategorySaving"
          @click="saveSetting"
        >
          确认
        </el-button>
        <el-button size="default" @click="overlay.bookGroupVisible = false">
          取消
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, watch } from 'vue'
import Sortable from 'sortablejs'
import { Rank } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { updateBookCategory } from '../../api/books'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useOverlayBookGroups } from '../../composables/useOverlayBookGroups'
import { useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'
import { createBookCategoryNameResolver } from '../../utils/bookCategory'
import { newestBookProgress, sortByShelfOrder } from '../../utils/bookOrder'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const operations = useAuthenticatedOperationGuard()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const managedBooks = computed(() => (
  sortByShelfOrder(bookshelf.books, reader.progressByBook)
))
const isSetMode = computed(() => overlay.bookGroupMode === 'set')
const isNormalPage = computed(() => reader.pageType === 'normal')
const bookGroupProjectionRevision = computed(() => JSON.stringify(
  bookshelf.bookGroups.map(group => [group.key, group.name, group.show, group.sortOrder]),
))
const categoryProjectionRevision = computed(() => JSON.stringify(
  bookshelf.categories.map(category => [category.id, category.name, category.show, category.sortOrder]),
))

const {
  settingCategorySaving,
  visibilitySavingId,
  groupOrderSaving,
  groupTableRef,
  groupRows,
  isGroupOrderDirty,
  groupBookCount,
  displayBookGroupName,
  prepareOpen,
  handleBookGroupSelectionChange: handleSelectionChange,
  saveBookGroupSetting: saveSetting,
  createCategory,
  renameGroup,
  toggleGroupVisibility: toggleVisibility,
  deleteGroup,
  handleBookGroupOpened: handleOpened,
  destroyGroupSortable: destroySortable,
  handleModeChange,
  saveGroupOrderDraft: saveOrder,
} = useOverlayBookGroups({
  operationGuard: operations,
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  updateBookCategory,
  categoryName,
  getBookProgress: book => newestBookProgress(book, reader.progressByBook),
  emitBookInfoUpdated: data => {
    window.dispatchEvent(new CustomEvent('openreader:book-info-updated', {
      detail: { book: data },
    }))
  },
  prompt: (...args) => ElMessageBox.prompt(...args),
  confirm: (...args) => ElMessageBox.confirm(...args),
  createSortable: (...args) => Sortable.create(...args),
  nextFrame: nextTick,
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

watch(
  () => overlay.bookGroupVisible,
  async (visible) => {
    if (!visible) return
    const operation = operations.begin('open-book-groups')
    try {
      await Promise.all([
        bookshelf.ensureCategoriesLoaded(),
        overlay.bookGroupMode === 'manage'
          ? bookshelf.loadBookGroups({ force: true })
          : bookshelf.ensureBookGroupsLoaded(),
        bookshelf.ensureBooksLoaded({ all: true }),
      ])
    } catch (error) {
      if (!operations.canCommit(operation)) return
      ElMessage.error(readError(error, '加载分组失败'))
      return
    }
    if (!operations.canCommit(operation)) return
    await prepareOpen()
  },
)

watch(
  () => overlay.bookGroupMode,
  mode => handleModeChange(mode),
)

watch(bookGroupProjectionRevision, async (revision, previous) => {
  if (!previous || revision === previous || !overlay.bookGroupVisible || isSetMode.value) return
  await prepareOpen('manage')
  await handleOpened()
})

watch(categoryProjectionRevision, async (revision, previous) => {
  if (!previous || revision === previous || !overlay.bookGroupVisible || !isSetMode.value) return
  handleSelectionChange([])
  await nextTick()
  groupTableRef.value?.clearSelection?.()
  await handleOpened()
})

watch(isNormalPage, (normal) => {
  if (normal || !overlay.bookGroupVisible) return
  overlay.bookGroupVisible = false
})

onBeforeUnmount(destroySortable)

function handleClosed() {
  destroySortable()
  overlay.bookGroupMode = 'manage'
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.book-group-dialog-body {
  min-width: 0;
}

.group-name-cell {
  display: inline-flex;
  min-width: 0;
  align-items: center;
}

.group-drag-icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  margin-right: 5px;
  cursor: move;
  user-select: none;
}

.group-name-cell > span:last-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-footer {
  min-height: 32px;
  text-align: right;
}

.float-left {
  float: left;
}
</style>
