<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'

const props = defineProps<{
  visible: boolean
  databaseId: number | null
  dbName?: string
  dbType?: string
  initialTab?: string
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
const importing = ref(false)
const savingCred = ref(false)
const savingConfig = ref(false)
const activeTab = ref('backups')
const backups = ref<any[]>([])
const remotes = ref<any[]>([])
const ossList = ref<any[]>([])
const summary = ref<any>({})
const config = ref({
  auto_enabled: false,
  schedule: '0 3 * * *',
  keep_count: 5,
  remote_id: null as number | null,
  oss_storage_id: null as number | null,
  backup_dir: '',
})
const credForm = ref({ username: '', password: '' })
const importFile = ref<File | null>(null)

const remoteDialog = ref(false)
const remoteSaving = ref(false)
const remoteTesting = ref(false)
const editingRemoteId = ref<number | null>(null)
const remoteForm = ref({
  name: '',
  provider: 'ftp',
  host: '',
  port: 21,
  username: '',
  password: '',
  remote_path: '/backups',
  access_token: '',
  extra_config: '',
  enabled: true,
})

const providerOptions = [
  { value: 'ftp', label: 'FTP' },
  { value: 'sftp', label: 'SFTP' },
  { value: 'webdav', label: 'WebDAV' },
  { value: 'google_drive', label: 'Google Drive' },
  { value: 'onedrive', label: 'OneDrive / 微软网盘' },
  { value: 'oss', label: 'OSS / S3 对象存储' },
  { value: 'custom', label: t('siteBackup.providerCustom') },
]

watch(
  () => [props.visible, props.databaseId, props.initialTab] as const,
  async ([vis, id, tab]) => {
    if (vis && id) {
      activeTab.value = tab || 'backups'
      importFile.value = null
      credForm.value = { username: '', password: '' }
      await loadAll(id)
    }
  }
)

watch(
  () => remoteForm.value.provider,
  (p) => {
    if (p === 'sftp') remoteForm.value.port = 22
    else if (p === 'ftp') remoteForm.value.port = 21
    else if (p === 'google_drive' || p === 'onedrive') remoteForm.value.port = 443
  }
)

async function loadAll(id: number) {
  loading.value = true
  try {
    const [cfgRes, listRes, dbRes, ossRes, remoteRes]: any[] = await Promise.all([
      api.get(`/databases/${id}/backup/config`),
      api.get(`/databases/${id}/backups`),
      api.get(`/databases/${id}`),
      api.get('/oss/storages').catch(() => ({ data: [] })),
      api.get('/backup/remotes'),
    ])
    config.value = { ...config.value, ...(cfgRes.data?.config || {}) }
    summary.value = cfgRes.data?.summary || {}
    backups.value = listRes.data || []
    ossList.value = ossRes.data || []
    remotes.value = remoteRes.data || []
    if (dbRes.data?.username) {
      credForm.value.username = dbRes.data.username
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.loadFailed'))
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
  if (!props.databaseId) return
  backingUp.value = true
  try {
    await api.post(`/databases/${props.databaseId}/backups`, {
      remote_id: config.value.remote_id || undefined,
      oss_storage_id: config.value.oss_storage_id || undefined,
    })
    ElMessage.success(t('dbBackup.backupDone'))
    await loadAll(props.databaseId)
    emit('updated')
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.backupFailed'))
  } finally {
    backingUp.value = false
  }
}

async function saveConfig() {
  if (!props.databaseId) return
  savingConfig.value = true
  try {
    await api.patch(`/databases/${props.databaseId}/backup/config`, config.value)
    ElMessage.success(t('siteBackup.configSaved'))
    emit('updated')
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.saveFailed'))
  } finally {
    savingConfig.value = false
  }
}

async function saveCredentials() {
  if (!props.databaseId) return
  savingCred.value = true
  try {
    await api.patch(`/databases/${props.databaseId}`, credForm.value)
    ElMessage.success(t('dbBackup.credSaved'))
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.saveFailed'))
  } finally {
    savingCred.value = false
  }
}

function onImportSelect(file: File) {
  importFile.value = file
  return false
}

async function runImport() {
  if (!props.databaseId || !importFile.value) {
    ElMessage.warning(t('dbBackup.importSelectFile'))
    return
  }
  await ElMessageBox.confirm(t('dbBackup.importConfirm'), t('common.warning'), { type: 'warning' })
  importing.value = true
  try {
    const fd = new FormData()
    fd.append('file', importFile.value)
    await api.post(`/databases/${props.databaseId}/import`, fd)
    ElMessage.success(t('dbBackup.importDone'))
    importFile.value = null
  } catch (e: any) {
    ElMessage.error(e?.error || e?.response?.data?.error || t('dbBackup.importFailed'))
  } finally {
    importing.value = false
  }
}

async function deleteBackup(row: any) {
  if (!props.databaseId) return
  await ElMessageBox.confirm(t('dbBackup.deleteConfirm'), t('common.confirm'), { type: 'warning' })
  await api.delete(`/databases/${props.databaseId}/backups/${row.id}`)
  ElMessage.success(t('dbBackup.deleted'))
  await loadAll(props.databaseId)
  emit('updated')
}

async function downloadBackup(row: any) {
  if (!props.databaseId) return
  const token = localStorage.getItem('token')
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = w.__OWPANEL_BASE__ || '/'
  const prefix = base.endsWith('/') ? base : base + '/'
  const url = `${prefix}api/v1/databases/${props.databaseId}/backups/${row.id}/download`
  const res = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!res.ok) {
    ElMessage.error(t('dbBackup.downloadFailed'))
    return
  }
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = row.file_path?.split(/[/\\]/).pop() || 'backup.sql'
  a.click()
  URL.revokeObjectURL(a.href)
}

function openRemoteDialog(row?: any) {
  if (row) {
    editingRemoteId.value = row.id
    remoteForm.value = {
      name: row.name,
      provider: row.provider,
      host: row.host,
      port: row.port || 21,
      username: row.username || '',
      password: '',
      remote_path: row.remote_path || '/',
      access_token: '',
      extra_config: row.extra_config || '',
      enabled: row.enabled !== false,
    }
  } else {
    editingRemoteId.value = null
    remoteForm.value = {
      name: '',
      provider: 'ftp',
      host: '',
      port: 21,
      username: '',
      password: '',
      remote_path: '/backups',
      access_token: '',
      extra_config: '',
      enabled: true,
    }
  }
  remoteDialog.value = true
}

async function saveRemote() {
  remoteSaving.value = true
  try {
    if (editingRemoteId.value) {
      await api.put(`/backup/remotes/${editingRemoteId.value}`, remoteForm.value)
    } else {
      await api.post('/backup/remotes', remoteForm.value)
    }
    ElMessage.success(t('siteBackup.remoteSaved'))
    remoteDialog.value = false
    if (props.databaseId) await loadAll(props.databaseId)
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.error || t('dbBackup.saveFailed'))
  } finally {
    remoteSaving.value = false
  }
}

async function testRemote(id: number) {
  remoteTesting.value = true
  try {
    await api.post(`/backup/remotes/${id}/test`)
    ElMessage.success(t('siteBackup.remoteTestOk'))
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.error || t('siteBackup.remoteTestFailed'))
  } finally {
    remoteTesting.value = false
  }
}

async function deleteRemote(id: number) {
  await ElMessageBox.confirm(t('siteBackup.deleteRemoteConfirm'), t('common.confirm'), { type: 'warning' })
  await api.delete(`/backup/remotes/${id}`)
  ElMessage.success(t('dbBackup.deleted'))
  if (props.databaseId) await loadAll(props.databaseId)
}

const needsToken = computed(() => ['google_drive', 'onedrive'].includes(remoteForm.value.provider))
const needsHost = computed(() => !needsToken.value || remoteForm.value.provider === 'webdav' || remoteForm.value.provider === 'custom')
</script>

<template>
  <el-dialog
    v-model="dialogVisible"
    :title="t('dbBackup.title', { name: dbName || '' })"
    width="760px"
    destroy-on-close
  >
    <div v-loading="loading">
      <div class="info-row">
        <el-tag size="small">{{ dbType || 'mysql' }}</el-tag>
        <span class="hint">{{ t('dbBackup.typeHint') }}</span>
      </div>
      <div class="backup-dir">
        <span class="label">{{ t('dbBackup.localDir') }}</span>
        <code>{{ config.backup_dir || '—' }}</code>
      </div>

      <el-tabs v-model="activeTab">
        <el-tab-pane :label="t('dbBackup.tabBackups')" name="backups">
          <div class="toolbar">
            <el-select v-model="config.remote_id" clearable :placeholder="t('siteBackup.remoteTarget')" style="width: 200px">
              <el-option v-for="r in remotes" :key="r.id" :label="`${r.name} (${r.provider})`" :value="r.id" />
            </el-select>
            <el-select v-model="config.oss_storage_id" clearable :placeholder="t('dbBackup.ossUpload')" style="width: 180px">
              <el-option v-for="o in ossList" :key="o.id" :label="o.name" :value="o.id" />
            </el-select>
            <el-button type="primary" :loading="backingUp" @click="runBackup">{{ t('dbBackup.runNow') }}</el-button>
            <span v-if="summary.count" class="hint">{{ t('siteBackup.summary', { count: summary.count }) }}</span>
          </div>
          <el-table :data="backups" stripe size="small">
            <el-table-column prop="created_at" :label="t('dbBackup.time')" width="170">
              <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column prop="file_path" :label="t('dbBackup.path')" min-width="180" show-overflow-tooltip />
            <el-table-column :label="t('dbBackup.size')" width="90">
              <template #default="{ row }">{{ formatSize(row.size) }}</template>
            </el-table-column>
            <el-table-column :label="t('siteBackup.remote')" width="100">
              <template #default="{ row }">
                <el-tag v-if="row.remote_status === 'synced'" size="small" type="success">OK</el-tag>
                <el-tag v-else-if="row.remote_status === 'failed'" size="small" type="danger" :title="row.remote_error">Fail</el-tag>
                <span v-else class="muted">—</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.actions')" width="160">
              <template #default="{ row }">
                <el-button v-if="row.status === 'done'" text size="small" @click="downloadBackup(row)">{{ t('dbBackup.download') }}</el-button>
                <el-button text type="danger" size="small" @click="deleteBackup(row)">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('siteBackup.tabAuto')" name="auto">
          <el-form label-width="120px">
            <el-form-item :label="t('siteBackup.autoEnable')">
              <el-switch v-model="config.auto_enabled" />
            </el-form-item>
            <el-form-item :label="t('siteBackup.schedule')">
              <el-input v-model="config.schedule" placeholder="0 3 * * *" />
              <div class="form-hint">{{ t('siteBackup.scheduleHint') }}</div>
            </el-form-item>
            <el-form-item :label="t('siteBackup.keepCount')">
              <el-input-number v-model="config.keep_count" :min="1" :max="100" />
            </el-form-item>
            <el-form-item :label="t('siteBackup.remoteTarget')">
              <el-select v-model="config.remote_id" clearable style="width: 100%">
                <el-option :label="t('siteBackup.remoteNone')" :value="null" />
                <el-option v-for="r in remotes" :key="r.id" :label="`${r.name} (${r.provider})`" :value="r.id" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('dbBackup.ossUpload')">
              <el-select v-model="config.oss_storage_id" clearable style="width: 100%">
                <el-option :label="t('siteBackup.remoteNone')" :value="null" />
                <el-option v-for="o in ossList" :key="o.id" :label="o.name" :value="o.id" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="savingConfig" @click="saveConfig">{{ t('siteModify.save') }}</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="t('siteBackup.tabRemotes')" name="remotes">
          <div class="toolbar">
            <el-button type="primary" @click="openRemoteDialog()">{{ t('siteBackup.addRemote') }}</el-button>
          </div>
          <el-table :data="remotes" stripe size="small">
            <el-table-column prop="name" :label="t('siteBackup.remoteName')" />
            <el-table-column prop="provider" :label="t('siteBackup.provider')" width="120" />
            <el-table-column prop="host" :label="t('siteBackup.host')" min-width="140" show-overflow-tooltip />
            <el-table-column prop="remote_path" :label="t('siteBackup.remotePath')" min-width="120" show-overflow-tooltip />
            <el-table-column :label="t('common.actions')" width="180">
              <template #default="{ row }">
                <el-button text size="small" :loading="remoteTesting" @click="testRemote(row.id)">{{ t('siteBackup.test') }}</el-button>
                <el-button text size="small" @click="openRemoteDialog(row)">{{ t('common.edit') }}</el-button>
                <el-button text type="danger" size="small" @click="deleteRemote(row.id)">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
          <p class="form-hint">{{ t('dbBackup.remoteHint') }}</p>
        </el-tab-pane>

        <el-tab-pane :label="t('dbBackup.tabImport')" name="import">
          <el-alert type="warning" :closable="false" show-icon class="import-alert">
            {{ t('dbBackup.importWarn') }}
          </el-alert>
          <el-upload drag :auto-upload="false" :show-file-list="true" accept=".sql,.gz,.zip" :before-upload="onImportSelect">
            <el-icon class="upload-icon"><UploadFilled /></el-icon>
            <div class="el-upload__text">{{ t('dbBackup.importDrop') }}</div>
            <template #tip>
              <div class="form-hint">{{ t('dbBackup.importTip') }}</div>
            </template>
          </el-upload>
          <div class="toolbar">
            <el-button type="primary" :loading="importing" :disabled="!importFile" @click="runImport">
              {{ t('dbBackup.importRun') }}
            </el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('dbBackup.tabCredentials')" name="credentials">
          <el-form label-width="100px">
            <el-form-item :label="t('databases.username')">
              <el-input v-model="credForm.username" />
            </el-form-item>
            <el-form-item :label="t('dbBackup.password')">
              <el-input v-model="credForm.password" type="password" show-password :placeholder="t('dbBackup.passwordHint')" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="savingCred" @click="saveCredentials">{{ t('siteModify.save') }}</el-button>
            </el-form-item>
          </el-form>
          <p class="form-hint">{{ t('dbBackup.credHint') }}</p>
        </el-tab-pane>
      </el-tabs>
    </div>

    <el-dialog v-model="remoteDialog" :title="editingRemoteId ? t('siteBackup.editRemote') : t('siteBackup.addRemote')" width="520px" append-to-body>
      <el-form label-width="110px">
        <el-form-item :label="t('siteBackup.remoteName')"><el-input v-model="remoteForm.name" /></el-form-item>
        <el-form-item :label="t('siteBackup.provider')">
          <el-select v-model="remoteForm.provider" style="width: 100%">
            <el-option v-for="o in providerOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="needsHost" :label="t('siteBackup.host')">
          <el-input v-model="remoteForm.host" />
        </el-form-item>
        <el-form-item v-if="needsHost && remoteForm.provider !== 'webdav' && remoteForm.provider !== 'custom'" :label="t('siteBackup.port')">
          <el-input-number v-model="remoteForm.port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item v-if="['ftp','sftp','webdav'].includes(remoteForm.provider)" :label="t('siteBackup.username')">
          <el-input v-model="remoteForm.username" />
        </el-form-item>
        <el-form-item v-if="['ftp','sftp','webdav'].includes(remoteForm.provider)" :label="t('siteBackup.password')">
          <el-input v-model="remoteForm.password" type="password" show-password :placeholder="t('siteBackup.passwordKeep')" />
        </el-form-item>
        <el-form-item v-if="needsToken" :label="t('siteBackup.accessToken')">
          <el-input v-model="remoteForm.access_token" type="textarea" :rows="2" />
          <div class="form-hint">{{ t('siteBackup.tokenHint') }}</div>
        </el-form-item>
        <el-form-item :label="t('siteBackup.remotePath')">
          <el-input v-model="remoteForm.remote_path" :placeholder="remoteForm.provider === 'google_drive' ? t('siteBackup.gdriveFolderHint') : '/backups'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="remoteDialog = false">{{ t('websites.cancel') }}</el-button>
        <el-button type="primary" :loading="remoteSaving" @click="saveRemote">{{ t('siteModify.save') }}</el-button>
      </template>
    </el-dialog>
  </el-dialog>
</template>

<style scoped>
.info-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.hint {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.muted {
  color: var(--el-text-color-secondary);
}
.backup-dir {
  margin-bottom: 12px;
  padding: 10px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  font-size: 13px;
}
.backup-dir .label {
  color: var(--el-text-color-secondary);
  margin-right: 8px;
}
.backup-dir code {
  word-break: break-all;
}
.toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0;
}
.form-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
.import-alert {
  margin-bottom: 12px;
}
.upload-icon {
  font-size: 48px;
  color: var(--el-color-primary);
}
</style>
