<template>
  <el-drawer
    v-model="overlay.backupVisible"
    title="备份恢复"
    :direction="direction"
    :size="size"
    class="global-backup-drawer"
    @open="loadBackups"
  >
    <section class="backup-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>备份恢复</strong>
          <span>保存当前数据到 WebDAV，或从备份包恢复</span>
        </div>
        <div class="file-actions">
          <el-button
            size="small"
            type="primary"
            :icon="Upload"
            :loading="backupLoading"
            @click="runBackup"
          >
            保存到 WebDAV
          </el-button>
          <el-upload
            :show-file-list="false"
            :auto-upload="false"
            accept=".zip"
            @change="restoreBackup"
          >
            <el-button size="small" :icon="Refresh" :loading="restoreLoading">
              恢复备份包
            </el-button>
          </el-upload>
          <el-button
            size="small"
            :icon="Refresh"
            :loading="backupListLoading"
            @click="loadBackups"
          >
            刷新列表
          </el-button>
        </div>
      </header>

      <el-table
        :data="backups"
        stripe
        v-loading="backupListLoading"
        class="desktop-backup-table"
      >
        <el-table-column prop="name" label="文件名" min-width="220" show-overflow-tooltip />
        <el-table-column label="大小" width="110">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="时间" width="190">
          <template #default="{ row }">{{ formatDate(row.time) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button text type="primary" @click="downloadBackupFile(row)">下载</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div
        v-if="backups.length"
        v-loading="backupListLoading"
        class="mobile-backup-list"
      >
        <article v-for="row in backups" :key="row.name" class="mobile-backup-card">
          <div>
            <strong>{{ row.name }}</strong>
            <span>{{ formatDate(row.time) }} · {{ formatSize(row.size) }}</span>
          </div>
          <el-button
            size="small"
            text
            type="primary"
            @click="downloadBackupFile(row)"
          >
            下载
          </el-button>
        </article>
      </div>
      <el-empty
        v-if="!backups.length && !backupListLoading"
        description="暂无备份文件"
      />
    </section>
  </el-drawer>
</template>

<script setup>
import { Refresh, Upload } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as backupApi from '../../api/backup'
import { useOverlayBackups } from '../../composables/useOverlayBackups'
import { useOverlayStore } from '../../stores/overlay'
import { applyRestoreResult } from '../../utils/restoreSync'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
  },
})

const overlay = useOverlayStore()

const {
  backups,
  backupLoading,
  listLoading: backupListLoading,
  restoreLoading,
  load: loadBackups,
  run: runBackup,
  download: downloadBackupFile,
  restore: restoreBackup,
} = useOverlayBackups({
  ...backupApi,
  restoreBackup: backupApi.restoreLegadoBackup,
  applyRestoreResult,
  saveBlob: downloadBlob,
  createFormData: () => new FormData(),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

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

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.backup-overlay {
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

.file-overlay-head span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.file-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.mobile-backup-list {
  display: none;
}

.mobile-backup-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-backup-card div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.mobile-backup-card strong,
.mobile-backup-card span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-backup-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

@media (max-width: 750px) {
  .file-overlay-head {
    align-items: flex-start;
    display: grid;
  }

  .file-actions {
    justify-content: flex-start;
  }

  .desktop-backup-table {
    display: none;
  }

  .mobile-backup-list {
    display: grid;
    gap: 10px;
  }
}
</style>
