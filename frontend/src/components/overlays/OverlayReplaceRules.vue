<template>
  <el-dialog
    v-model="overlay.replaceRulesVisible"
    title="替换规则管理"
    width="min(1000px, max(750px, 70vw))"
    top="max(15dvh, calc((100dvh - 584px) / 2))"
    :fullscreen="isMobile"
    class="global-replace-dialog"
    destroy-on-close
    @open="loadReplaceRules"
    @closed="resetManager"
  >
    <template #header>
      <div class="replace-dialog-header">
        <span class="el-dialog__title">替换规则管理</span>
        <el-button
          text
          :loading="replaceRuleImporting"
          @click="triggerReplaceRuleImport"
        >
          导入
        </el-button>
        <input
          ref="replaceRuleFileInput"
          class="visually-hidden-file"
          type="file"
          accept=".json,application/json"
          @change="importReplaceRuleFile"
        />
      </div>
    </template>

    <section class="replace-overlay">
      <el-table
        :data="replaceRules"
        :height="isMobile ? 'calc(100dvh - 184px)' : 'min(400px, calc(70dvh - 184px))'"
        v-loading="replaceRulesLoading"
        class="replace-rule-table"
        @selection-change="onReplaceRuleSelectionChange"
      >
        <el-table-column type="selection" width="25" :fixed="isMobile" />
        <el-table-column
          prop="name"
          label="规则名称"
          min-width="150"
          :fixed="isMobile"
          show-overflow-tooltip
        />
        <el-table-column
          prop="scope"
          label="替换范围"
          min-width="150"
          show-overflow-tooltip
        />
        <el-table-column label="是否启用" min-width="80">
          <template #default="{ row }">
            <el-switch
              :model-value="normalizeReplaceRule(row).enabled"
              active-color="#13ce66"
              inactive-color="#ff4949"
              @change="value => toggleReplaceRule(row, value)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button text @click="openReplaceRuleEditor(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <template #footer>
      <div class="replace-manager-footer">
        <div>
          <el-button type="primary" @click="deleteSelectedReplaceRules">
            批量删除
          </el-button>
          <span>已选择 {{ selectedReplaceRuleIds.length }} 个</span>
        </div>
        <el-button @click="overlay.replaceRulesVisible = false">取消</el-button>
      </div>
    </template>
  </el-dialog>

  <el-dialog
    v-model="replaceRuleDialog"
    title="替换规则"
    width="min(1000px, max(750px, 70vw))"
    top="max(15dvh, calc((100dvh - 584px) / 2))"
    :fullscreen="isMobile"
    class="replace-rule-editor-dialog"
    @closed="overlay.clearReplaceRuleEditor()"
  >
    <el-form :model="replaceRuleDraft">
      <el-form-item label="名称">
        <el-input v-model="replaceRuleDraft.name" />
      </el-form-item>
      <el-form-item label="规则">
        <el-input v-model="replaceRuleDraft.pattern" />
      </el-form-item>
      <el-form-item label="替换为">
        <el-input v-model="replaceRuleDraft.replacement" />
      </el-form-item>
      <el-form-item label="替换范围">
        <el-input
          v-model="replaceRuleDraft.scope"
          placeholder="* 或 书名 或 书名;书籍地址"
        />
      </el-form-item>
      <div class="replace-rule-checks">
        <el-checkbox v-model="replaceRuleDraft.isRegex">
          使用正则表达式
        </el-checkbox>
        <el-checkbox v-model="replaceRuleDraft.enabled">
          是否启用
        </el-checkbox>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="replaceRuleDialog = false">取 消</el-button>
      <el-button
        type="primary"
        :loading="replaceRuleSaving"
        @click="saveReplaceRule"
      >
        确 定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as replaceRulesApi from '../../api/replaceRules'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useOverlayReplaceRules } from '../../composables/useOverlayReplaceRules'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const managerOperations = useAuthenticatedOperationGuard()
const editorOperations = useAuthenticatedOperationGuard()

const {
  rules: replaceRules,
  loading: replaceRulesLoading,
  importing: replaceRuleImporting,
  selectedIds: selectedReplaceRuleIds,
  fileInput: replaceRuleFileInput,
  dialogVisible: replaceRuleDialog,
  saving: replaceRuleSaving,
  draft: replaceRuleDraft,
  load: loadReplaceRules,
  resetManager,
  handleUpdated: handleReplaceRulesUpdated,
  clearRefresh: clearReplaceRulesRefreshTimer,
  changeSelection: onReplaceRuleSelectionChange,
  triggerImport: triggerReplaceRuleImport,
  importFile: importReplaceRuleFile,
  normalize: normalizeReplaceRule,
  openEditor: openReplaceRuleEditor,
  save: saveReplaceRule,
  toggle: toggleReplaceRule,
  removeSelected: deleteSelectedReplaceRules,
} = useOverlayReplaceRules({
  managerOperationGuard: managerOperations,
  editorOperationGuard: editorOperations,
  isActive: () => overlay.replaceRulesVisible,
  ...replaceRulesApi,
  confirm: (...args) => ElMessageBox.confirm(...args),
  notifyUpdated: () => {
    window.dispatchEvent(new CustomEvent(
      'openreader:replace-rules-updated',
      { detail: { local: true } },
    ))
  },
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

onMounted(() => {
  window.addEventListener(
    'openreader:replace-rules-updated',
    handleReplaceRulesUpdated,
  )
})

watch(
  () => overlay.replaceRuleEditorRequest,
  request => {
    if (request > 0) openReplaceRuleEditor(overlay.replaceRuleEditorDraft || {})
  },
)

onBeforeUnmount(() => {
  window.removeEventListener(
    'openreader:replace-rules-updated',
    handleReplaceRulesUpdated,
  )
  clearReplaceRulesRefreshTimer()
})

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.replace-overlay {
  min-height: 0;
}

.replace-dialog-header,
.replace-manager-footer,
.replace-manager-footer > div,
.replace-rule-checks {
  display: flex;
  align-items: center;
}

.replace-dialog-header,
.replace-manager-footer {
  justify-content: space-between;
}

.replace-dialog-header {
  min-width: 0;
  padding-right: 30px;
}

.replace-dialog-header .el-dialog__title {
  min-width: 0;
}

.replace-manager-footer > div {
  gap: 14px;
}

.replace-manager-footer span {
  color: var(--app-text-muted);
}

.replace-rule-checks {
  flex-wrap: wrap;
  gap: 24px;
}

.visually-hidden-file {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
  padding: 0;
  margin: -1px;
}

@media (max-width: 750px) {
  .replace-dialog-header {
    padding-right: max(30px, env(safe-area-inset-right));
  }

  .replace-manager-footer {
    gap: 8px;
  }

  .replace-manager-footer > div {
    min-width: 0;
    gap: 8px;
  }

  .replace-manager-footer span {
    white-space: nowrap;
  }
}
</style>
