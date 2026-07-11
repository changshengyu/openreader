<template>
  <el-drawer
    v-model="overlay.replaceRulesVisible"
    title="替换规则"
    :direction="direction"
    :size="size"
    class="global-replace-drawer"
    @open="loadReplaceRules"
  >
    <section class="replace-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>全局替换规则</strong>
          <span>阅读器会按启用规则处理正文内容</span>
        </div>
        <div class="file-actions">
          <el-button size="small" type="primary" :icon="Edit" @click="openReplaceRuleEditor()">新增规则</el-button>
          <el-button size="small" :icon="Upload" :loading="replaceRuleImporting" @click="triggerReplaceRuleImport">导入</el-button>
          <el-button
            size="small"
            type="danger"
            plain
            :icon="Delete"
            :disabled="!selectedReplaceRuleIds.length"
            @click="deleteSelectedReplaceRules"
          >
            批量删除
          </el-button>
          <el-button size="small" :icon="Refresh" :loading="replaceRulesLoading" @click="loadReplaceRules">刷新</el-button>
          <input
            ref="replaceRuleFileInput"
            class="visually-hidden-file"
            type="file"
            accept=".json,application/json"
            @change="importReplaceRuleFile"
          />
        </div>
      </header>

      <el-table
        :data="replaceRules"
        stripe
        v-loading="replaceRulesLoading"
        class="desktop-replace-table"
        @selection-change="onReplaceRuleSelectionChange"
      >
        <el-table-column type="selection" width="44" />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="scope" label="替换范围" min-width="150" show-overflow-tooltip />
        <el-table-column prop="pattern" label="匹配" min-width="180" show-overflow-tooltip />
        <el-table-column prop="replacement" label="替换为" min-width="160" show-overflow-tooltip />
        <el-table-column label="正则" width="80">
          <template #default="{ row }">
            {{ normalizeReplaceRule(row).isRegex ? '是' : '否' }}
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch
              :model-value="normalizeReplaceRule(row).enabled"
              size="small"
              @change="value => toggleReplaceRule(row, value)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button text @click="openReplaceRuleEditor(row)">编辑</el-button>
            <el-button text type="danger" @click="removeReplaceRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="replaceRules.length" v-loading="replaceRulesLoading" class="mobile-rule-list">
        <article v-for="rule in replaceRules" :key="rule.id" class="mobile-rule-card">
          <header>
            <el-checkbox
              :model-value="selectedReplaceRuleIds.includes(rule.id)"
              @change="toggleReplaceRuleSelection(rule.id, $event)"
            />
            <div>
              <strong>{{ rule.name || '未命名规则' }}</strong>
              <em>{{ normalizeReplaceRule(rule).scope }}</em>
              <span>{{ rule.pattern }}</span>
            </div>
            <el-switch
              :model-value="normalizeReplaceRule(rule).enabled"
              size="small"
              @change="value => toggleReplaceRule(rule, value)"
            />
          </header>
          <p>替换为：{{ rule.replacement || '空' }}</p>
          <p>模式：{{ normalizeReplaceRule(rule).isRegex ? '正则表达式' : '普通文本' }}</p>
          <footer>
            <el-button size="small" text @click="openReplaceRuleEditor(rule)">编辑</el-button>
            <el-button size="small" text type="danger" @click="removeReplaceRule(rule)">删除</el-button>
          </footer>
        </article>
      </div>
      <el-empty v-if="!replaceRulesLoading && !replaceRules.length" description="暂无全局替换规则" />
    </section>
  </el-drawer>

  <el-dialog
    v-model="replaceRuleDialog"
    :title="editingReplaceRuleId ? '编辑替换规则' : '新增替换规则'"
    width="520px"
    :fullscreen="isMobile"
    @closed="overlay.clearReplaceRuleEditor()"
  >
    <el-form label-position="top">
      <el-form-item label="名称">
        <el-input v-model="replaceRuleDraft.name" />
      </el-form-item>
      <el-form-item label="匹配正则或文本">
        <el-input v-model="replaceRuleDraft.pattern" />
      </el-form-item>
      <el-form-item label="替换为">
        <el-input v-model="replaceRuleDraft.replacement" />
      </el-form-item>
      <el-form-item label="替换范围">
        <el-input v-model="replaceRuleDraft.scope" placeholder="* 或 书名 或 书名;书籍地址" />
      </el-form-item>
      <el-form-item>
        <el-switch v-model="replaceRuleDraft.isRegex" active-text="使用正则表达式" inactive-text="普通文本" />
      </el-form-item>
      <el-form-item>
        <el-switch v-model="replaceRuleDraft.enabled" active-text="启用" inactive-text="停用" />
      </el-form-item>
      <el-form-item label="测试文本">
        <el-input v-model="replaceRuleTestText" type="textarea" :rows="3" />
      </el-form-item>
      <div class="replace-test-actions">
        <el-button size="small" :loading="replaceRuleTesting" @click="runReplaceRuleTest">测试规则</el-button>
        <span
          v-if="replaceRuleTestResult"
          :class="replaceRuleTestResult.changed ? 'msg-success' : 'msg-muted'"
        >
          {{ replaceRuleTestResult.changed ? '已发生替换' : '未匹配' }}
        </span>
      </div>
      <pre v-if="replaceRuleTestResult" class="replace-test-output">{{ replaceRuleTestResult.output }}</pre>
    </el-form>
    <template #footer>
      <el-button @click="replaceRuleDialog = false">取消</el-button>
      <el-button type="primary" :loading="replaceRuleSaving" @click="saveReplaceRule">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Refresh, Upload } from '@element-plus/icons-vue'
import * as replaceRulesApi from '../../api/replaceRules'
import { useOverlayReplaceRules } from '../../composables/useOverlayReplaceRules'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  direction: {
    type: String,
    default: 'rtl',
  },
  size: {
    type: [String, Number],
    default: '82%',
  },
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()

const {
  rules: replaceRules,
  loading: replaceRulesLoading,
  importing: replaceRuleImporting,
  selectedIds: selectedReplaceRuleIds,
  fileInput: replaceRuleFileInput,
  dialogVisible: replaceRuleDialog,
  saving: replaceRuleSaving,
  testing: replaceRuleTesting,
  editingId: editingReplaceRuleId,
  draft: replaceRuleDraft,
  testText: replaceRuleTestText,
  testResult: replaceRuleTestResult,
  load: loadReplaceRules,
  handleUpdated: handleReplaceRulesUpdated,
  clearRefresh: clearReplaceRulesRefreshTimer,
  changeSelection: onReplaceRuleSelectionChange,
  toggleSelection: toggleReplaceRuleSelection,
  triggerImport: triggerReplaceRuleImport,
  importFile: importReplaceRuleFile,
  normalize: normalizeReplaceRule,
  openEditor: openReplaceRuleEditor,
  save: saveReplaceRule,
  toggle: toggleReplaceRule,
  runTest: runReplaceRuleTest,
  remove: removeReplaceRule,
  removeSelected: deleteSelectedReplaceRules,
} = useOverlayReplaceRules({
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
  display: grid;
  gap: 12px;
}

.file-overlay-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-overlay-head > div:first-child {
  display: grid;
  gap: 2px;
}

.file-overlay-head span,
.mobile-rule-card em,
.mobile-rule-card span,
.mobile-rule-card p,
.msg-muted {
  color: var(--app-text-muted);
  font-size: 12px;
}

.file-actions,
.mobile-rule-card header,
.mobile-rule-card footer,
.replace-test-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
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

.mobile-rule-list {
  display: none;
}

.mobile-rule-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-rule-card header {
  justify-content: space-between;
}

.mobile-rule-card header > div {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 2px;
}

.mobile-rule-card strong,
.mobile-rule-card em,
.mobile-rule-card span,
.mobile-rule-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-rule-card em {
  font-style: normal;
}

.mobile-rule-card p {
  margin: 0;
}

.replace-test-actions {
  margin-bottom: 8px;
}

.msg-success {
  color: var(--el-color-success);
  font-size: 12px;
}

.replace-test-output {
  max-height: 180px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: rgba(255, 255, 255, 0.68);
  color: var(--app-text);
  white-space: pre-wrap;
}

@media (max-width: 750px) {
  .file-overlay-head {
    align-items: flex-start;
    display: grid;
  }

  .file-actions {
    justify-content: flex-start;
  }

  .desktop-replace-table {
    display: none;
  }

  .mobile-rule-list {
    display: grid;
    gap: 10px;
  }
}
</style>
