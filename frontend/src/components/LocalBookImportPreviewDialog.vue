<template>
  <el-dialog
    :model-value="modelValue"
    :title="title"
    width="760px"
    :fullscreen="isMobile"
    class="local-book-preview-dialog"
    @update:model-value="emit('update:modelValue', $event)"
    @open="resetDrafts"
  >
    <div class="preview-toolbar">
      <el-checkbox
        :model-value="allSelected"
        :indeterminate="someSelected"
        @change="toggleAll"
      >
        选择可导入书籍
      </el-checkbox>
      <span>已选择 {{ selectedCount }} / {{ importableCount }} 本</span>
      <el-select v-model="selectedCategoryIds" multiple clearable placeholder="统一设置分组（可多选）">
        <el-option
          v-for="category in categories"
          :key="category.id"
          :label="category.name"
          :value="String(category.id)"
        />
      </el-select>
    </div>

    <div class="preview-list">
      <article
        v-for="row in drafts"
        :key="row.path"
        class="preview-row"
        :class="{ failed: !!row.error, selected: row.selected }"
      >
        <el-checkbox v-if="!row.error" v-model="row.selected" />
        <el-tag v-else type="danger" effect="plain">失败</el-tag>
        <div class="preview-fields">
          <strong>{{ row.path }}</strong>
          <template v-if="!row.error">
            <el-input v-model="row.title" placeholder="书名" />
            <el-input v-model="row.author" placeholder="作者（可选）" />
            <el-input
              v-if="isTextLocalPath(row.path)"
              v-model="row.tocRule"
              placeholder="TXT 目录规则（可选，导入时重新解析）"
            />
            <el-select v-if="isEPUBLocalPath(row.path)" v-model="row.tocRule" placeholder="EPUB 目录规则">
              <el-option v-for="rule in epubTocRuleOptions" :key="rule.value" :label="rule.label" :value="rule.value" />
            </el-select>
            <div class="preview-meta">
              <span>共 {{ row.chapterCount }} 章</span>
              <el-button v-if="row.chapters.length" text @click="row.expanded = !row.expanded">
                {{ row.expanded ? '收起目录' : '查看目录' }}
              </el-button>
            </div>
            <div v-if="row.expanded" class="chapter-preview">
              <span v-for="chapter in row.chapters" :key="chapter.index">{{ chapter.title }}</span>
            </div>
          </template>
          <p v-else>{{ row.error }}</p>
        </div>
      </article>
    </div>

    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="loading" :disabled="!selectedCount" @click="confirmImport">
        确认导入 {{ selectedCount || '' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useReaderStore } from '../stores/reader'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { epubTocRuleOptions, isEPUBLocalPath, isTextLocalPath } from '../utils/localBookToc'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  title: {
    type: String,
    default: '导入本地书籍',
  },
  items: {
    type: Array,
    default: () => [],
  },
  categories: {
    type: Array,
    default: () => [],
  },
  categoryIds: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'confirm'])
const reader = useReaderStore()
const windowWidth = ref(currentViewportWidth())
const drafts = ref([])
const selectedCategoryIds = ref([])

const isMobile = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const importableRows = computed(() => drafts.value.filter(row => !row.error))
const importableCount = computed(() => importableRows.value.length)
const selectedCount = computed(() => importableRows.value.filter(row => row.selected).length)
const allSelected = computed(() => importableCount.value > 0 && selectedCount.value === importableCount.value)
const someSelected = computed(() => selectedCount.value > 0 && !allSelected.value)

watch(
  () => props.items,
  () => {
    if (props.modelValue) resetDrafts()
  },
  { deep: true },
)

onMounted(() => window.addEventListener('resize', updateWindowWidth, { passive: true }))
onBeforeUnmount(() => window.removeEventListener('resize', updateWindowWidth))

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

function resetDrafts() {
  drafts.value = props.items.map(item => ({
    path: item.path || '',
    error: item.error || '',
    title: item.book?.title || '',
    author: item.book?.author || '',
    chapterCount: Number(item.book?.chapterCount || 0),
    chapters: Array.isArray(item.book?.chapters) ? item.book.chapters : [],
    tocRule: item.tocRule || (isEPUBLocalPath(item.path) ? 'spin+toc' : ''),
    selected: !item.error,
    expanded: false,
  }))
  selectedCategoryIds.value = props.categoryIds.map(id => String(id))
}

function toggleAll(value) {
  importableRows.value.forEach((row) => {
    row.selected = Boolean(value)
  })
}

function confirmImport() {
  const items = importableRows.value
    .filter(row => row.selected)
    .map(row => ({
      path: row.path,
      title: row.title.trim(),
      author: row.author.trim(),
      tocRule: row.tocRule || '',
    }))
  emit('confirm', {
    items,
    categoryIds: selectedCategoryIds.value.map(id => Number(id)).filter(Boolean),
  })
}
</script>

<style scoped>
.preview-toolbar {
  display: grid;
  grid-template-columns: auto auto minmax(220px, 1fr);
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.preview-toolbar > span {
  color: var(--app-text-muted);
  font-size: 13px;
}

.preview-list {
  display: grid;
  max-height: min(62vh, 620px);
  overflow: auto;
  gap: 10px;
}

.preview-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.preview-row.selected {
  border-color: color-mix(in srgb, var(--app-primary) 42%, var(--app-border));
}

.preview-row.failed {
  background: var(--el-color-danger-light-9);
}

.preview-fields {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr) minmax(160px, 0.55fr);
  gap: 8px;
}

.preview-fields > strong,
.preview-fields > p,
.preview-meta,
.chapter-preview {
  grid-column: 1 / -1;
}

.preview-fields > strong {
  overflow-wrap: anywhere;
  font-size: 13px;
}

.preview-fields > p {
  margin: 0;
  color: var(--el-color-danger);
}

.preview-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--app-text-muted);
  font-size: 13px;
}

.chapter-preview {
  display: grid;
  max-height: 180px;
  overflow: auto;
  gap: 5px;
  padding: 8px 10px;
  background: var(--app-bg-soft);
  border-radius: var(--app-radius-sm);
  color: var(--app-text-muted);
  font-size: 12px;
}

@media (max-width: 750px) {
  .preview-toolbar,
  .preview-fields {
    grid-template-columns: 1fr;
  }

  .preview-toolbar > span,
  .preview-fields > strong,
  .preview-fields > p,
  .preview-meta,
  .chapter-preview {
    grid-column: 1;
  }

  .preview-list {
    max-height: none;
  }
}
</style>
