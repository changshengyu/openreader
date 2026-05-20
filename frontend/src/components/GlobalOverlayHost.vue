<template>
  <BookInfoDialog
    v-model="overlay.bookInfoVisible"
    :book="overlay.bookInfoBook"
    :source-name="overlay.bookInfoOptions.sourceName"
    :category-name="bookInfoCategory"
    :progress="bookInfoProgress"
    :chapters="overlay.bookInfoBook?.chapterCount || 0"
    :status-label="overlay.bookInfoOptions.statusLabel || sourceStatusLabel"
    :status-type="overlay.bookInfoOptions.statusType || 'info'"
  >
    <div v-if="overlay.bookInfoOptions.actions?.length" class="overlay-actions">
      <el-button
        v-for="action in overlay.bookInfoOptions.actions"
        :key="action.label"
        :type="action.type || 'default'"
        :plain="action.plain"
        :loading="!!action.loading"
        :disabled="!!action.disabled"
        @click="action.handler?.(overlay.bookInfoBook)"
      >
        {{ action.label }}
      </el-button>
    </div>
    <div v-else-if="overlay.bookInfoBook?.id" class="overlay-actions">
      <el-button type="primary" @click="continueRead(overlay.bookInfoBook)">继续阅读</el-button>
      <el-button plain @click="openContentSearch(overlay.bookInfoBook)">搜正文</el-button>
      <el-button plain @click="openBookmarks(overlay.bookInfoBook)">书签</el-button>
      <el-button plain @click="goDetail(overlay.bookInfoBook)">详情</el-button>
      <el-button plain :loading="loadingUpdates" @click="refreshShelf">刷新书架</el-button>
    </div>
  </BookInfoDialog>

  <el-drawer v-model="overlay.bookManageVisible" title="书架管理" direction="rtl" size="82%" class="global-manage-drawer">
    <el-table
      :data="managedBooks"
      row-key="id"
      height="calc(100vh - 188px)"
      class="manage-table"
      @selection-change="onManageSelectionChange"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column prop="title" label="书名" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <el-button text class="text-button" @click="overlay.openBookInfo(row)">{{ row.title }}</el-button>
        </template>
      </el-table-column>
      <el-table-column prop="author" label="作者" min-width="120" show-overflow-tooltip />
      <el-table-column label="分组" min-width="120">
        <template #default="{ row }">{{ categoryName(row.categoryId) }}</template>
      </el-table-column>
      <el-table-column label="章节" min-width="150">
        <template #default="{ row }">
          <span>共 {{ row.chapterCount || 0 }} 章</span><br>
          <span>阅读进度：{{ progressLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button text class="text-button" @click="goDetail(row)">编辑</el-button>
          <el-button text class="text-button" @click="setBookGroup(row)">分组</el-button>
          <el-dropdown @command="cacheBook(row, $event)">
            <el-button text class="text-button" :loading="cachingBookId === row.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookCache">删除服务器缓存</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(row, $event)">
            <el-button text class="text-button">
              导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="json">导出书籍数据</el-dropdown-item>
                <el-dropdown-item disabled>导出为TXT（后端未实现）</el-dropdown-item>
                <el-dropdown-item disabled>导出为Epub（后端未实现）</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>
    <div class="manage-footer">
      <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchDeleteBooks">批量删除</el-button>
      <el-dropdown @command="batchAddCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量添加分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-dropdown @command="batchRemoveCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量移除分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="check-tip">已选择 {{ selectedBookIds.length }} 个</span>
      <el-button :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchCacheBooks">批量缓存</el-button>
      <el-button :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchClearCache">批量清缓存</el-button>
    </div>
  </el-drawer>

  <el-drawer
    v-model="overlay.bookGroupVisible"
    :title="overlay.bookGroupMode === 'set' ? '设置分组' : '分组管理'"
    direction="rtl"
    size="360px"
  >
    <template v-if="overlay.bookGroupMode === 'set'">
      <el-table :data="bookshelf.categories" row-key="id" class="group-set-table" @row-click="selectBookGroup">
        <el-table-column width="46">
          <template #default="{ row }">
            <span class="radio-cell" :class="{ active: String(settingCategoryId) === String(row.id) }" />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="分组名" />
      </el-table>
      <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
      <div class="manage-footer group-set-footer">
        <el-button @click="settingCategoryId = ''">未分组</el-button>
        <el-button type="primary" :loading="settingCategorySaving" @click="saveBookGroupSetting">确认</el-button>
        <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
      </div>
    </template>
    <template v-else>
      <div class="group-create">
        <el-input v-model="newGroupName" placeholder="新增分组" size="small" @keyup.enter="createCategory" />
        <el-button size="small" type="primary" :disabled="!newGroupName.trim()" @click="createCategory">新增</el-button>
      </div>
      <div class="group-list">
        <div v-for="category in bookshelf.categories" :key="category.id" class="group-row">
          <span>{{ category.name }}</span>
          <span class="group-actions">
            <el-button size="small" text @click="moveGroup(category, -1)">上移</el-button>
            <el-button size="small" text @click="moveGroup(category, 1)">下移</el-button>
            <el-button size="small" text @click="renameGroup(category)">重命名</el-button>
            <el-button size="small" text type="danger" @click="deleteGroup(category)">删除</el-button>
          </span>
        </div>
      </div>
      <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
    </template>
  </el-drawer>

  <el-drawer
    v-model="overlay.searchBookContentVisible"
    :title="`搜索正文${overlay.searchBook?.title ? ` · ${overlay.searchBook.title}` : ''}`"
    direction="rtl"
    size="420px"
    class="global-search-drawer"
  >
    <ReaderSearchPanel
      v-model="contentKeyword"
      :results="contentResults"
      :loading="contentSearching"
      :searched="contentSearched"
      :has-more="contentHasMore"
      :status-text="contentSearchStatus"
      @search="searchCurrentBookContent"
      @load-more="loadMoreCurrentBookContent"
      @jump="jumpToContentResult"
    />
  </el-drawer>

  <el-drawer
    v-model="overlay.bookmarkVisible"
    :title="`书签${overlay.bookmarkBook?.title ? ` · ${overlay.bookmarkBook.title}` : ''}`"
    direction="rtl"
    size="420px"
    class="global-bookmark-drawer"
  >
    <div v-loading="bookmarkLoading">
      <ReaderBookmarkPanel
        :bookmarks="bookmarkItems"
        :show-add="false"
        @jump="jumpToBookmark"
        @edit="openBookmarkEditor"
        @remove="removeBookmarkItem"
      />
    </div>
  </el-drawer>

  <el-dialog v-model="bookmarkEditorVisible" title="编辑书签" width="380px">
    <div class="bookmark-editor">
      <el-input v-model="bookmarkDraft.title" placeholder="标题" />
      <el-input v-model="bookmarkDraft.excerpt" type="textarea" :rows="3" placeholder="摘录" />
      <el-input v-model="bookmarkDraft.note" type="textarea" :rows="4" placeholder="笔记" />
    </div>
    <template #footer>
      <el-button @click="bookmarkEditorVisible = false">取消</el-button>
      <el-button type="primary" :loading="bookmarkSaving" @click="saveBookmarkEdit">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import { cacheBookContent, checkBookUpdates, deleteBookmark, listBookmarks, searchBookContent, updateBookCategory, updateBookmark } from '../api/books'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import BookInfoDialog from './BookInfoDialog.vue'
import ReaderBookmarkPanel from './reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from './reader/ReaderSearchPanel.vue'

const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const selectedBookIds = ref([])
const batchBusy = ref(false)
const cachingBookId = ref(null)
const settingCategoryId = ref('')
const settingCategorySaving = ref(false)
const loadingUpdates = ref(false)
const newGroupName = ref('')
const contentKeyword = ref('')
const contentResults = ref([])
const contentSearching = ref(false)
const contentSearched = ref(false)
const contentLastIndex = ref(-1)
const contentHasMore = ref(false)
const contentTotal = ref(0)
const bookmarkItems = ref([])
const bookmarkLoading = ref(false)
const bookmarkEditorVisible = ref(false)
const bookmarkSaving = ref(false)
const editingBookmark = ref(null)
const bookmarkDraft = reactive({ title: '', excerpt: '', note: '' })

const bookInfoCategory = computed(() => overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook?.categoryId))
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return book ? (reader.progressByBook[book.id]?.percent || book.progress?.percent || 0) : 0
})
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => [...bookshelf.books].sort(compareByReadingOrder))
const contentSearchStatus = computed(() => {
  if (!contentSearched.value) return ''
  const scanned = contentLastIndex.value >= 0 ? contentLastIndex.value + 1 : 0
  if (!contentTotal.value) return `${contentResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, contentTotal.value)} / ${contentTotal.value} 章，${contentResults.value.length} 条结果`
})

watch(
  () => overlay.bookManageVisible || overlay.bookGroupVisible,
  async (visible) => {
    if (!visible) return
    try {
      await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
      if (overlay.bookGroupVisible && overlay.bookGroupMode === 'set') {
        settingCategoryId.value = overlay.bookInfoBook?.categoryId ? String(overlay.bookInfoBook.categoryId) : ''
      }
    } catch (err) {
      ElMessage.error(readError(err, '加载书架数据失败'))
    }
  },
)

watch(
  () => overlay.searchBookContentVisible,
  (visible) => {
    if (visible) return
    contentKeyword.value = ''
    contentResults.value = []
    contentSearched.value = false
    contentLastIndex.value = -1
    contentHasMore.value = false
    contentTotal.value = 0
  },
)

watch(contentKeyword, () => {
  contentResults.value = []
  contentSearched.value = false
  contentLastIndex.value = -1
  contentHasMore.value = false
  contentTotal.value = 0
})

watch(
  () => overlay.bookmarkVisible,
  async (visible) => {
    if (!visible) {
      bookmarkItems.value = []
      return
    }
    await loadBookmarkItems()
  },
)

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function progressLabel(book) {
  const progress = reader.progressByBook[book.id] || book.progress
  return `${Math.round((progress?.percent || 0) * 100)}%`
}

function compareByReadingOrder(a, b) {
  const aProgress = reader.progressByBook[a.id] || a.progress
  const bProgress = reader.progressByBook[b.id] || b.progress
  const aReadAt = new Date(aProgress?.updatedAt || 0).getTime()
  const bReadAt = new Date(bProgress?.updatedAt || 0).getTime()
  if (aReadAt !== bReadAt) return bReadAt - aReadAt
  return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime()
}

function onManageSelectionChange(rows) {
  selectedBookIds.value = rows.map(row => row.id)
}

function continueRead(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id } })
}

function goDetail(book) {
  overlay.closeBookInfo()
  overlay.bookManageVisible = false
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book.categoryId),
    progress: (reader.progressByBook[book.id]?.percent || book.progress?.percent || 0),
  })
}

function selectBookGroup(category) {
  settingCategoryId.value = String(category.id)
}

async function saveBookGroupSetting() {
  const book = overlay.bookInfoBook
  if (!book?.id) return
  settingCategorySaving.value = true
  try {
    const categoryId = settingCategoryId.value ? Number(settingCategoryId.value) : null
    const { data } = await updateBookCategory(book.id, categoryId)
    const index = bookshelf.books.findIndex(item => item.id === book.id)
    if (index >= 0) bookshelf.books[index] = data
    overlay.bookInfoBook = data
    overlay.bookInfoOptions = {
      ...overlay.bookInfoOptions,
      categoryName: categoryName(data.categoryId),
      progress: reader.progressByBook[data.id]?.percent || data.progress?.percent || 0,
    }
    overlay.bookGroupVisible = false
    ElMessage.success('分组已设置')
  } catch (err) {
    ElMessage.error(readError(err, '设置分组失败'))
  } finally {
    settingCategorySaving.value = false
  }
}

function openContentSearch(book) {
  overlay.closeBookInfo()
  overlay.openSearchBookContent(book)
}

function openBookmarks(book) {
  overlay.closeBookInfo()
  overlay.openBookmark(book)
}

async function refreshShelf() {
  loadingUpdates.value = true
  try {
    const { data } = await checkBookUpdates()
    await bookshelf.loadBooks()
    ElMessage.success(data?.newChapters ? `发现 ${data.newChapters} 个新章节` : '暂未发现新章节')
  } catch (err) {
    ElMessage.error(readError(err, '刷新失败'))
  } finally {
    loadingUpdates.value = false
  }
}

async function batchAddCategory(category) {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory([...selectedBookIds.value], category.id)
    ElMessage.success(`已添加到“${category.name}”分组`)
  } catch (err) {
    ElMessage.error(readError(err, '批量添加分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchRemoveCategory(category) {
  if (!selectedBookIds.value.length) return
  const targetIds = managedBooks.value
    .filter(book => selectedBookIds.value.includes(book.id) && String(book.categoryId) === String(category.id))
    .map(book => book.id)
  if (!targetIds.length) {
    ElMessage.info('选中书籍不在该分组中')
    return
  }
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory(targetIds, null)
    ElMessage.success(`已从“${category.name}”分组移除`)
  } catch (err) {
    ElMessage.error(readError(err, '批量移除分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchCacheBooks() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    const data = await bookshelf.batchCacheBooks([...selectedBookIds.value])
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '批量缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchClearCache() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定清理选中 ${selectedBookIds.value.length} 本书的章节缓存吗？`, '清理缓存', { type: 'warning' })
    batchBusy.value = true
    const data = await bookshelf.batchClearCache([...selectedBookIds.value])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchDeleteBooks() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selectedBookIds.value.length} 本书吗？`, '批量删除', { type: 'warning' })
    batchBusy.value = true
    await bookshelf.batchDeleteBooks([...selectedBookIds.value])
    selectedBookIds.value = []
    ElMessage.success('已批量删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  } finally {
    batchBusy.value = false
  }
}

async function cacheBook(book, command) {
  if (command === 'deleteBookCache') {
    await clearBookCache(book)
    return
  }
  cachingBookId.value = book.id
  try {
    const { data } = await cacheBookContent(book.id, { all: true })
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function clearBookCache(book) {
  cachingBookId.value = book.id
  try {
    const data = await bookshelf.batchClearCache([book.id])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function exportBook(book) {
  batchBusy.value = true
  try {
    const blob = await bookshelf.exportSelectedBooks([book.id])
    downloadBlob(blob, `openreader-book-${book.id}.json`)
    ElMessage.success(`已导出《${book.title}》`)
  } catch (err) {
    ElMessage.error(readError(err, '导出失败'))
  } finally {
    batchBusy.value = false
  }
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function searchCurrentBookContent() {
  return runCurrentBookContentSearch({ append: false })
}

async function loadMoreCurrentBookContent() {
  return runCurrentBookContentSearch({ append: true })
}

async function runCurrentBookContentSearch({ append = false } = {}) {
  const book = overlay.searchBook
  const keyword = contentKeyword.value.trim()
  if (!book?.id || !keyword) return
  contentSearching.value = true
  contentSearched.value = true
  try {
    const { data } = await searchBookContent(book.id, keyword, {
      paged: 1,
      lastIndex: append ? contentLastIndex.value : -1,
      chapterLimit: 30,
      matchLimit: 80,
    })
    const rows = data?.list || []
    contentResults.value = append ? contentResults.value.concat(rows) : rows
    contentLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
    contentHasMore.value = Boolean(data?.hasMore)
    contentTotal.value = Number(data?.total || 0)
  } catch (err) {
    ElMessage.error(readError(err, '搜索正文失败'))
  } finally {
    contentSearching.value = false
  }
}

function jumpToContentResult(result) {
  const book = overlay.searchBook
  if (!book?.id) return
  overlay.searchBookContentVisible = false
  router.push({
    name: 'reader',
    params: { id: book.id },
    query: {
      chapter: Number(result.chapterIndex || 0),
      line: Number.isInteger(result.lineIndex) ? result.lineIndex : undefined,
      q: contentKeyword.value.trim() || undefined,
    },
  })
}

async function loadBookmarkItems() {
  const book = overlay.bookmarkBook
  if (!book?.id) return
  bookmarkLoading.value = true
  try {
    const { data } = await listBookmarks(book.id)
    bookmarkItems.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载书签失败'))
  } finally {
    bookmarkLoading.value = false
  }
}

function jumpToBookmark(bookmark) {
  const book = overlay.bookmarkBook
  if (!book?.id) return
  overlay.bookmarkVisible = false
  router.push({
    name: 'reader',
    params: { id: book.id },
    query: {
      chapter: bookmark.chapterIndex,
      offset: bookmark.offset || 0,
    },
  })
}

function openBookmarkEditor(bookmark) {
  editingBookmark.value = bookmark
  Object.assign(bookmarkDraft, {
    title: bookmark.title || '',
    excerpt: bookmark.excerpt || '',
    note: bookmark.note || '',
  })
  bookmarkEditorVisible.value = true
}

async function saveBookmarkEdit() {
  if (!editingBookmark.value) return
  bookmarkSaving.value = true
  try {
    const { data } = await updateBookmark(editingBookmark.value.id, {
      title: bookmarkDraft.title,
      excerpt: bookmarkDraft.excerpt,
      note: bookmarkDraft.note,
    })
    const index = bookmarkItems.value.findIndex(item => item.id === data.id)
    if (index >= 0) bookmarkItems.value[index] = data
    bookmarkEditorVisible.value = false
    ElMessage.success('书签已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新书签失败'))
  } finally {
    bookmarkSaving.value = false
  }
}

async function removeBookmarkItem(bookmark) {
  try {
    await deleteBookmark(bookmark.id)
    bookmarkItems.value = bookmarkItems.value.filter(item => item.id !== bookmark.id)
    ElMessage.success('书签已删除')
  } catch (err) {
    ElMessage.error(readError(err, '删除书签失败'))
  }
}

async function createCategory() {
  const name = newGroupName.value.trim()
  if (!name) return
  try {
    await bookshelf.addCategory({ name })
    newGroupName.value = ''
    ElMessage.success('分组已创建')
  } catch (err) {
    ElMessage.error(readError(err, '创建分组失败'))
  }
}

async function renameGroup(category) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的分组名称', '重命名分组', {
      inputValue: category.name,
      inputValidator: value => !!value?.trim() || '分组名称不能为空',
    })
    const name = value.trim()
    if (!name || name === category.name) return
    await bookshelf.renameCategory(category.id, { name })
    ElMessage.success('分组已重命名')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

async function deleteGroup(category) {
  try {
    await ElMessageBox.confirm(`确定删除分组“${category.name}”吗？`, '删除分组', { type: 'warning' })
    await bookshelf.removeCategory(category.id)
    ElMessage.success('分组已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除分组失败'))
  }
}

async function moveGroup(category, direction) {
  const categories = [...bookshelf.categories]
  const index = categories.findIndex(item => item.id === category.id)
  const targetIndex = index + direction
  if (index < 0 || targetIndex < 0 || targetIndex >= categories.length) return
  const [moved] = categories.splice(index, 1)
  categories.splice(targetIndex, 0, moved)
  try {
    await bookshelf.reorderCategoryIds(categories.map(item => item.id))
    ElMessage.success('分组排序已更新')
  } catch (err) {
    ElMessage.error(readError(err, '分组排序失败'))
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.overlay-actions,
.manage-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.overlay-actions {
  margin-top: 4px;
}

.group-list {
  display: grid;
  gap: 10px;
}

.group-row,
.group-create {
  display: grid;
  align-items: center;
  gap: 10px;
}

.manage-table {
  margin-bottom: 12px;
}

.text-button {
  padding: 0;
}

.manage-footer {
  align-items: center;
  padding-top: 10px;
  border-top: 1px solid var(--app-border);
}

.check-tip {
  color: var(--app-text-muted);
  font-size: 13px;
}

.group-create {
  grid-template-columns: minmax(0, 1fr) auto;
  margin-bottom: 12px;
}

.group-row {
  grid-template-columns: minmax(0, 1fr) auto;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.group-set-table {
  margin-bottom: 12px;
}

.group-set-footer {
  margin-top: 12px;
}

.radio-cell {
  display: inline-flex;
  width: 14px;
  height: 14px;
  border: 1px solid var(--app-border);
  border-radius: 50%;
}

.radio-cell.active {
  border-color: var(--el-color-primary);
  box-shadow: inset 0 0 0 4px #fff;
  background: var(--el-color-primary);
}

.group-actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.bookmark-editor {
  display: grid;
  gap: 10px;
}

</style>
