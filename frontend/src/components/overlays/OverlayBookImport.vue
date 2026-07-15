<template>
  <el-dialog
    v-model="overlay.importBookVisible"
    title="导入本地书籍"
    width="520px"
    class="import-book-dialog"
    :fullscreen="isMobile"
    @open="open"
  >
    <div class="import-form">
      <el-upload
        drag
        :show-file-list="false"
        :auto-upload="false"
        accept=".txt,.text,.md,.epub,.pdf,.umd,.cbz"
        @change="pickFile"
      >
        <el-icon class="upload-icon"><UploadFilled /></el-icon>
        <div class="upload-text">
          {{ draft.file
            ? draft.file.name
            : '拖入或选择 TXT / EPUB / PDF / UMD / CBZ 文件' }}
        </div>
      </el-upload>
      <el-input
        v-model="draft.title"
        placeholder="书名（可选，不填则使用文件名）"
      />
      <el-input v-model="draft.author" placeholder="作者（可选）" />
      <el-select
        v-model="draft.categoryIds"
        placeholder="分组（可多选）"
        multiple
        clearable
      >
        <el-option
          v-for="category in bookshelf.categories"
          :key="category.id"
          :label="category.name"
          :value="String(category.id)"
        />
      </el-select>

      <el-select
        v-if="isText"
        v-model="draft.tocRule"
        filterable
        allow-create
        clearable
        default-first-option
        :loading="tocRulesLoading"
        placeholder="目录规则（可选，留空自动识别）"
      >
        <el-option
          v-for="rule in tocRuleOptions"
          :key="rule.id"
          :label="rule.name"
          :value="rule.rule"
        >
          <div class="toc-rule-option">
            <strong>{{ rule.name }}</strong>
            <span>{{ rule.rule }}</span>
          </div>
        </el-option>
      </el-select>
      <el-input
        v-if="isText"
        v-model="draft.tocRule"
        type="textarea"
        :rows="2"
        placeholder="TXT目录规则（可选，留空使用默认规则，例如：^第.+章.*$）"
      />
      <el-select
        v-if="isEPUB"
        v-model="draft.tocRule"
        placeholder="EPUB 目录规则"
      >
        <el-option
          v-for="rule in epubTocRuleOptions"
          :key="rule.value"
          :label="rule.label"
          :value="rule.value"
        />
      </el-select>

      <div v-if="draft.file" class="direct-import-preview">
        <div>
          <strong>
            {{ previewData
              ? `已解析 ${previewData.chapterCount || 0} 章`
              : '尚未解析目录' }}
          </strong>
          <el-button
            size="small"
            text
            :loading="previewing"
            @click="preview"
          >
            重新解析
          </el-button>
        </div>
        <div
          v-if="previewData?.chapters?.length"
          class="direct-import-chapters"
        >
          <span
            v-for="chapter in previewData.chapters"
            :key="chapter.index"
          >
            {{ chapter.title }}
          </span>
        </div>
        <div v-else-if="previewData?.chapterCount === 0" class="direct-import-preview-empty">
          未匹配到目录。你可以修改目录规则后重新解析，或保留空目录导入，之后再从书籍信息中刷新目录。
        </div>
        <div v-else-if="previewError" class="direct-import-preview-error">
          {{ previewError }}
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="overlay.importBookVisible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="importing"
        :disabled="!draft.file || !previewData"
        @click="importBook"
      >
        导入
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  listTXTTocRules,
  previewLocalBook,
} from '../../api/books'
import { useOverlayBookImport } from '../../composables/useOverlayBookImport'
import { useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { epubTocRuleOptions } from '../../utils/localBookToc'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()

const {
  importing,
  previewing,
  previewData,
  previewError,
  draft,
  tocRuleOptions,
  tocRulesLoading,
  isText,
  isEPUB,
  open,
  pickFile,
  preview,
  importBook,
} = useOverlayBookImport({
  visible: computed(() => overlay.importBookVisible),
  loadCategories: () => bookshelf.ensureCategoriesLoaded(),
  listTocRules: () => listTXTTocRules(),
  previewBook: (...args) => previewLocalBook(...args),
  importBook: payload => bookshelf.importTXT(payload),
  close: () => {
    overlay.importBookVisible = false
  },
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

function readError(error, fallback) {
  const message = error?.response?.data?.error?.message ||
    error?.response?.data?.error
  if (String(message || '').includes('no readable chapters')) return fallback
  return message || fallback
}
</script>

<style scoped>
.import-form {
  display: grid;
  gap: 12px;
}

.toc-rule-option {
  display: grid;
  gap: 2px;
  min-width: 0;
  line-height: 1.25;
}

.toc-rule-option strong,
.toc-rule-option span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.toc-rule-option span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.upload-icon {
  color: var(--app-primary);
  font-size: 32px;
}

.upload-text {
  color: var(--app-text-muted);
}

.direct-import-preview {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.direct-import-preview > div:first-child {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.direct-import-chapters {
  display: grid;
  max-height: 180px;
  overflow: auto;
  gap: 5px;
  padding: 8px;
  background: var(--app-bg-soft);
  color: var(--app-text-muted);
  font-size: 12px;
}

.direct-import-preview-error {
  color: var(--el-color-warning);
  font-size: 13px;
  line-height: 1.5;
}

.direct-import-preview-empty {
  color: var(--app-text-muted);
  font-size: 13px;
  line-height: 1.5;
}
</style>
