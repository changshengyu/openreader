<template>
  <el-dialog
    v-model="overlay.bookGroupVisible"
    :title="overlay.bookGroupMode === 'set' ? '设置分组' : '分组管理'"
    width="min(760px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    destroy-on-close
    class="global-book-group-dialog"
    @opened="handleOpened"
    @closed="destroySortable"
  >
    <section class="book-group-dialog-body">
      <template v-if="overlay.bookGroupMode === 'set'">
        <el-table
          :data="groupSetRows"
          row-key="id"
          class="group-set-table"
          @row-click="toggleSelection"
        >
          <el-table-column width="46">
            <template #default="{ row }">
              <el-checkbox
                :model-value="isSelected(row)"
                @change="() => toggleSelection(row)"
                @click.stop
              />
            </template>
          </el-table-column>
          <el-table-column label="分组名">
            <template #default="{ row }">
              <span class="group-set-name">
                <span>{{ row.name }}</span>
                <small>{{ row.description }}</small>
              </span>
            </template>
          </el-table-column>
        </el-table>
        <div class="manage-footer group-set-footer">
          <el-button
            type="primary"
            :loading="settingCategorySaving"
            @click="saveSetting"
          >
            确认
          </el-button>
          <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
        </div>
      </template>

      <template v-else>
        <el-table
          ref="groupManageTableRef"
          :data="groupManageRows"
          row-key="id"
          class="group-manage-table"
        >
          <el-table-column width="46">
            <template #default>
              <button
                type="button"
                class="group-drag-handle"
                title="拖动排序"
              >
                <el-icon><Rank /></el-icon>
              </button>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="分组名" min-width="130">
            <template #default="{ row }">
              <span class="group-table-name">
                <span>{{ row.name }}</span>
                <small>{{ groupBookCount(row) }} 本</small>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="显示" width="120">
            <template #default="{ row }">
              <el-switch
                :model-value="row.show !== false"
                :loading="visibilitySavingId === row.id"
                active-text="显示"
                inactive-text="隐藏"
                @change="value => toggleVisibility(row, value)"
              />
            </template>
          </el-table-column>
          <el-table-column label="操作" min-width="180">
            <template #default="{ row }">
              <el-button size="small" text @click="renameGroup(row)">
                编辑
              </el-button>
              <el-button
                v-if="groupBookCount(row) === 0"
                size="small"
                text
                type="danger"
                @click="deleteGroup(row)"
              >
                删除
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty
          v-if="!bookshelf.categories.length"
          description="还没有自定义分组"
        />
        <div class="manage-footer group-manage-footer">
          <el-button type="primary" @click="createCategory">添加分组</el-button>
          <el-button
            v-if="isGroupOrderDirty"
            type="primary"
            :loading="groupOrderSaving"
            @click="saveOrder"
          >
            保存排序
          </el-button>
          <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
        </div>
      </template>
    </section>
  </el-dialog>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, watch } from 'vue'
import Sortable from 'sortablejs'
import { Rank } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { updateBookCategory } from '../../api/books'
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
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const managedBooks = computed(() => (
  sortByShelfOrder(bookshelf.books, reader.progressByBook)
))

const {
  settingCategorySaving,
  visibilitySavingId,
  groupOrderSaving,
  groupManageTableRef,
  groupSetRows,
  groupManageRows,
  isGroupOrderDirty,
  groupBookCount,
  prepareOpen,
  isBookGroupSelected: isSelected,
  toggleBookGroupSelection: toggleSelection,
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
    try {
      await bookshelf.ensureCategoriesLoaded()
    } catch (error) {
      ElMessage.error(readError(error, '加载分组失败'))
      return
    }
    prepareOpen()
  },
)

watch(
  () => overlay.bookGroupMode,
  mode => handleModeChange(mode),
)

onBeforeUnmount(destroySortable)

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

.manage-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--app-border);
}

.group-manage-table,
.group-set-table {
  margin-bottom: 12px;
}

.group-drag-handle {
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-muted);
  cursor: move;
}

.group-drag-handle:hover {
  background: var(--app-bg-soft);
  color: var(--app-text);
}

.group-table-name,
.group-set-name {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.group-table-name span,
.group-set-name span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-table-name small,
.group-set-name small {
  color: var(--app-text-muted);
  font-size: 12px;
}

.group-set-footer {
  margin-top: 12px;
}

@media (max-width: 750px) {
  .manage-footer {
    align-items: stretch;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .manage-footer :deep(.el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .group-set-footer {
    grid-template-columns: 1fr;
  }
}
</style>
