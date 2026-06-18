<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const props = defineProps<{
  visible: boolean
  siteId: number | null
  domain?: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  updated: []
}>()

const { t } = useI18n()

const dialogVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const loading = ref(false)
const backingUp = ref(false)
const backups = ref<any[]>([])
const config = ref({ backup_dir: '' })

watch(
  () => [props.visible, props.siteId] as const,
  async ([vis, id]) => {
    if (vis && id) await loadAll(id)
  }
)

async function loadAll(id: number) {
  loading.value = true
  try {
    const [cfgRes, listRes]: any[] = await Promise.all([
      api.get(`/wordpress/${id}/backup/config`),
      api.get(`/wordpress/${id}/backups`),
    ])
    config.value = cfgRes.data || {}
    backups.value = listRes.data || []
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('wpBackup.loadFailed'))
  } finally {
    loading.value = false
  }
}

function formatSize(n: number) {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

async function runBackup() {
  if (!props.siteId) return
  backingUp.value = true
  try {
    const res: any = await api.post(`/wordpress/${props.siteId}/backup`)
    const rec = res.data
    if (rec?.status === 'failed') {
      ElMessage.warning(rec.error_msg || t('wpBackup.partialFailed'))
    } else {
      ElMessage.success(t('wpBackup.backupDone'))
    }
    await loadAll(props.siteId)
    emit('updated')
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('wpBackup.backupFailed'))
  } finally {
    backingUp.value = false
  }
}

async function deleteBackup(row: any) {
  if (!props.siteId) return
  await ElMessageBox.confirm(t('wpBackup.deleteConfirm'), t('common.confirm'), { type: 'warning' })
  await api.delete(`/wordpress/${props.siteId}/backups/${row.id}`)
  ElMessage.success(t('wpBackup.deleted'))
  await loadAll(props.siteId)
  emit('updated')
}

async function downloadBackup(row: any) {
  if (!props.siteId) return
  const token = localStorage.getItem('token')
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || '/'
  const prefix = base.endsWith('/') ? base : base + '/'
  const url = `${prefix}api/v1/wordpress/${props.siteId}/backups/${row.id}/download`
  const res = await fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
  if (!res.ok) {
    ElMessage.error(t('wpBackup.downloadFailed'))
    return
  }
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = row.file_path?.split(/[/\\]/).pop() || 'wordpress-backup.zip'
  a.click()
  URL.revokeObjectURL(a.href)
}
</script>

<template>
  <el-dialog
    v-model="dialogVisible"
    :title="t('wpBackup.title', { domain: domain || '' })"
    width="720px"
    destroy-on-close
  >
    <div v-loading="loading">
      <el-alert type="info" :closable="false" show-icon class="hint-alert">
        {{ t('wpBackup.hint') }}
      </el-alert>
      <div class="backup-dir">
        <span class="label">{{ t('wpBackup.localDir') }}</span>
        <code>{{ config.backup_dir || '—' }}</code>
      </div>
      <div class="toolbar">
        <el-button type="primary" :loading="backingUp" @click="runBackup">{{ t('wpBackup.runNow') }}</el-button>
      </div>
      <el-table :data="backups" stripe size="small">
        <el-table-column prop="created_at" :label="t('wpBackup.time')" width="170">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="file_path" :label="t('wpBackup.path')" min-width="200" show-overflow-tooltip />
        <el-table-column :label="t('wpBackup.size')" width="90">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column :label="t('wpBackup.database')" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.has_database" size="small" type="success">{{ row.db_name || 'SQL' }}</el-tag>
            <el-tag v-else size="small" type="warning">{{ t('wpBackup.filesOnly') }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.actions')" width="160">
          <template #default="{ row }">
            <el-button v-if="row.status === 'done'" text size="small" @click="downloadBackup(row)">{{ t('wpBackup.download') }}</el-button>
            <el-button text type="danger" size="small" @click="deleteBackup(row)">{{ t('common.delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <p class="form-hint">{{ t('wpBackup.packageHint') }}</p>
    </div>
  </el-dialog>
</template>

<style scoped>
.hint-alert { margin-bottom: 12px; }
.backup-dir {
  margin-bottom: 12px;
  padding: 10px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  font-size: 13px;
}
.backup-dir .label { color: var(--el-text-color-secondary); margin-right: 8px; }
.backup-dir code { word-break: break-all; }
.toolbar { margin-bottom: 12px; }
.form-hint {
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
</style>
