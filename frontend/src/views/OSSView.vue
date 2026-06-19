<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const tab = ref('storages')
const loading = ref(false)
const providers = ref<any[]>([])
const storages = ref<any[]>([])
const tasks = ref<any[]>([])

const storageDialog = ref(false)
const editingStorageId = ref<number | null>(null)
const storageForm = ref({
  name: '',
  provider: 'local',
  endpoint: '',
  region: '',
  bucket: '',
  access_key: '',
  secret_key: '',
  local_path: '',
  path_prefix: '',
  use_path_style: false,
  enabled: true,
  remark: '',
})

const taskDialog = ref(false)
const editingTaskId = ref<number | null>(null)
const taskForm = ref({
  name: '',
  mode: 'upload',
  source_storage_id: null as number | null,
  target_storage_id: null as number | null,
  extra_target_ids: [] as number[],
  source_path: '',
  target_path: '',
  local_path: '',
  delete_extra: false,
  schedule: '',
  enabled: true,
})

const importDialog = ref(false)
const importText = ref('')
const importMode = ref('merge')

const logDialog = ref(false)
const logTask = ref<any>(null)
let logTimer: ReturnType<typeof setInterval> | null = null

const browseStorageId = ref<number | null>(null)
const browsePrefix = ref('')
const browseItems = ref<any[]>([])

const isLocalProvider = computed(() => storageForm.value.provider === 'local')

const modeOptions = computed(() => [
  { value: 'upload', label: t('ossPage.modeUpload') },
  { value: 'download', label: t('ossPage.modeDownload') },
  { value: 'sync', label: t('ossPage.modeSync') },
  { value: 'migrate', label: t('ossPage.modeMigrate') },
])

function providerLabel(key: string) {
  return providers.value.find(p => p.key === key)?.name || key
}

function statusTag(status: string) {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'running') return 'warning'
  return 'info'
}

async function loadAll() {
  loading.value = true
  try {
    const [p, s, tk]: any[] = await Promise.all([
      api.get('/oss/providers'),
      api.get('/oss/storages'),
      api.get('/oss/sync-tasks'),
    ])
    providers.value = p.data || []
    storages.value = s.data || []
    tasks.value = tk.data || []
  } finally {
    loading.value = false
  }
}

function resetStorageForm() {
  editingStorageId.value = null
  storageForm.value = {
    name: '',
    provider: 'local',
    endpoint: '',
    region: '',
    bucket: '',
    access_key: '',
    secret_key: '',
    local_path: '',
    path_prefix: '',
    use_path_style: false,
    enabled: true,
    remark: '',
  }
}

function openStorage(row?: any) {
  resetStorageForm()
  if (row) {
    editingStorageId.value = row.id
    storageForm.value = {
      name: row.name,
      provider: row.provider,
      endpoint: row.endpoint || '',
      region: row.region || '',
      bucket: row.bucket || '',
      access_key: '',
      secret_key: '',
      local_path: row.local_path || '',
      path_prefix: row.path_prefix || '',
      use_path_style: row.use_path_style,
      enabled: row.enabled,
      remark: row.remark || '',
    }
  } else {
    applyPresetDefaults('local')
  }
  storageDialog.value = true
}

function applyPresetDefaults(key: string) {
  const preset = providers.value.find(p => p.key === key)
  if (!preset) return
  storageForm.value.use_path_style = preset.use_path_style
  if (!storageForm.value.endpoint && preset.endpoint_hint) {
    storageForm.value.endpoint = preset.endpoint_hint
  }
  if (!storageForm.value.region && preset.region_hint) {
    storageForm.value.region = preset.region_hint
  }
}

watch(() => storageForm.value.provider, (key) => {
  if (!editingStorageId.value) applyPresetDefaults(key)
})

async function saveStorage() {
  const payload = { ...storageForm.value }
  if (editingStorageId.value) {
    await api.put(`/oss/storages/${editingStorageId.value}`, payload)
  } else {
    await api.post('/oss/storages', payload)
  }
  ElMessage.success(t('common.success'))
  storageDialog.value = false
  loadAll()
}

async function testStorage(row: any) {
  try {
    await api.post(`/oss/storages/${row.id}/test`)
    ElMessage.success(t('ossPage.testOk'))
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.failed'))
  }
}

async function deleteStorage(row: any) {
  await ElMessageBox.confirm(t('ossPage.deleteStorageConfirm'), t('common.confirm'), { type: 'warning' })
  await api.delete(`/oss/storages/${row.id}`)
  ElMessage.success(t('common.deleted'))
  loadAll()
}

function resetTaskForm() {
  editingTaskId.value = null
  taskForm.value = {
    name: '',
    mode: 'upload',
    source_storage_id: null,
    target_storage_id: null,
    extra_target_ids: [],
    source_path: '',
    target_path: '',
    local_path: '',
    delete_extra: false,
    schedule: '',
    enabled: true,
  }
}

function parseExtraTargets(raw: unknown): number[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw.map(Number).filter(n => n > 0)
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed.map(Number).filter(n => n > 0) : []
    } catch { return [] }
  }
  return []
}

async function exportConfig(includeSecrets = false) {
  const res: any = await api.get('/oss/export', { params: { include_secrets: includeSecrets } })
  const blob = new Blob([JSON.stringify(res.data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `oss-config-${new Date().toISOString().slice(0, 10)}.json`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success(t('ossPage.exportDone'))
}

async function runImport() {
  let payload: any
  try {
    payload = JSON.parse(importText.value)
  } catch {
    ElMessage.error(t('ossPage.importInvalid'))
    return
  }
  payload.mode = importMode.value
  const res: any = await api.post('/oss/import', payload)
  ElMessage.success(t('ossPage.importDone', {
    created: res.data?.storages_created ?? 0,
    tasks: res.data?.tasks_created ?? 0,
  }))
  importDialog.value = false
  importText.value = ''
  loadAll()
}

function onImportFile(file: File) {
  const reader = new FileReader()
  reader.onload = () => { importText.value = String(reader.result || '') }
  reader.readAsText(file)
  return false
}

function openTask(row?: any) {
  resetTaskForm()
  if (row) {
    editingTaskId.value = row.id
    taskForm.value = {
      name: row.name,
      mode: row.mode,
      source_storage_id: row.source_storage_id ?? null,
      target_storage_id: row.target_storage_id ?? null,
      extra_target_ids: parseExtraTargets(row.extra_target_ids),
      source_path: row.source_path || '',
      target_path: row.target_path || '',
      local_path: row.local_path || '',
      delete_extra: row.delete_extra,
      schedule: row.schedule || '',
      enabled: row.enabled,
    }
  }
  taskDialog.value = true
}

async function saveTask() {
  const payload = { ...taskForm.value }
  if (editingTaskId.value) {
    await api.put(`/oss/sync-tasks/${editingTaskId.value}`, payload)
  } else {
    await api.post('/oss/sync-tasks', payload)
  }
  ElMessage.success(t('common.success'))
  taskDialog.value = false
  loadAll()
}

async function runTask(row: any) {
  await api.post(`/oss/sync-tasks/${row.id}/run`)
  ElMessage.info(t('ossPage.taskStarted'))
  openLogs(row)
  loadAll()
}

async function deleteTask(row: any) {
  await ElMessageBox.confirm(t('ossPage.deleteTaskConfirm'), t('common.confirm'), { type: 'warning' })
  await api.delete(`/oss/sync-tasks/${row.id}`)
  ElMessage.success(t('common.deleted'))
  loadAll()
}

function openLogs(row: any) {
  logTask.value = row
  logDialog.value = true
  refreshLogs()
  logTimer = setInterval(refreshLogs, 1500)
}

async function refreshLogs() {
  if (!logTask.value) return
  const res: any = await api.get(`/oss/sync-tasks/${logTask.value.id}/logs`)
  logTask.value = res.data
}

function closeLogs() {
  if (logTimer) clearInterval(logTimer)
  logTimer = null
  logDialog.value = false
  loadAll()
}

async function loadBrowse() {
  if (!browseStorageId.value) return
  const res: any = await api.get(`/oss/storages/${browseStorageId.value}/browse`, {
    params: { prefix: browsePrefix.value, limit: 300 },
  })
  browseItems.value = res.data || []
}

function formatSize(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

onMounted(async () => {
  await loadAll()
  const provider = String(route.query.provider || '').toLowerCase()
  if (route.query.add === '1' && provider) {
    resetStorageForm()
    storageForm.value.provider = provider
    applyPresetDefaults(provider)
    storageDialog.value = true
    router.replace({ path: route.path, query: {} })
  }
})
onUnmounted(() => { if (logTimer) clearInterval(logTimer) })
</script>

<template>
  <div v-loading="loading">
    <div class="page-header">
      <h2>{{ t('ossPage.title') }}</h2>
      <div class="header-actions">
        <el-button @click="exportConfig(false)">{{ t('ossPage.export') }}</el-button>
        <el-button @click="exportConfig(true)">{{ t('ossPage.exportWithSecrets') }}</el-button>
        <el-button @click="importDialog = true">{{ t('ossPage.import') }}</el-button>
        <el-button @click="loadAll">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon class="hint">
      {{ t('ossPage.hint') }}
    </el-alert>

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('ossPage.tabStorages')" name="storages">
        <div class="toolbar">
          <el-button type="primary" @click="openStorage()">{{ t('ossPage.addStorage') }}</el-button>
        </div>
        <el-table :data="storages" stripe>
          <el-table-column prop="name" :label="t('common.name')" min-width="140" />
          <el-table-column :label="t('ossPage.provider')" width="140">
            <template #default="{ row }">{{ providerLabel(row.provider) }}</template>
          </el-table-column>
          <el-table-column prop="bucket" label="Bucket" width="120" show-overflow-tooltip />
          <el-table-column prop="endpoint" :label="t('ossPage.endpoint')" show-overflow-tooltip />
          <el-table-column prop="local_path" :label="t('ossPage.localPath')" show-overflow-tooltip />
          <el-table-column :label="t('common.status')" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="220" fixed="right">
            <template #default="{ row }">
              <el-button text type="primary" @click="openStorage(row)">{{ t('common.edit') }}</el-button>
              <el-button text type="success" @click="testStorage(row)">{{ t('ossPage.test') }}</el-button>
              <el-button text type="danger" @click="deleteStorage(row)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('ossPage.tabTasks')" name="tasks">
        <div class="toolbar">
          <el-button type="primary" @click="openTask()">{{ t('ossPage.addTask') }}</el-button>
        </div>
        <el-table :data="tasks" stripe>
          <el-table-column prop="name" :label="t('common.name')" min-width="140" />
          <el-table-column :label="t('ossPage.mode')" width="100">
            <template #default="{ row }">{{ modeOptions.find(m => m.value === row.mode)?.label || row.mode }}</template>
          </el-table-column>
          <el-table-column prop="local_path" :label="t('ossPage.localPath')" show-overflow-tooltip />
          <el-table-column prop="schedule" :label="t('ossPage.schedule')" width="120" />
          <el-table-column :label="t('common.status')" width="100">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.running ? 'running' : row.last_status)" size="small">
                {{ row.running ? t('ossPage.running') : (row.last_status || 'idle') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="260" fixed="right">
            <template #default="{ row }">
              <el-button text type="primary" :disabled="row.running" @click="runTask(row)">{{ t('ossPage.runNow') }}</el-button>
              <el-button text @click="openLogs(row)">{{ t('ossPage.viewLog') }}</el-button>
              <el-button text type="primary" @click="openTask(row)">{{ t('common.edit') }}</el-button>
              <el-button text type="danger" @click="deleteTask(row)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('ossPage.tabBrowse')" name="browse">
        <div class="toolbar browse-bar">
          <el-select v-model="browseStorageId" :placeholder="t('ossPage.selectStorage')" style="width: 220px" @change="loadBrowse">
            <el-option v-for="s in storages" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
          <el-input v-model="browsePrefix" :placeholder="t('ossPage.prefix')" style="width: 260px" clearable @keyup.enter="loadBrowse" />
          <el-button type="primary" @click="loadBrowse">{{ t('ossPage.browse') }}</el-button>
        </div>
        <el-table :data="browseItems" stripe max-height="480">
          <el-table-column prop="key" :label="t('ossPage.objectKey')" min-width="280" show-overflow-tooltip />
          <el-table-column :label="t('ossPage.size')" width="100">
            <template #default="{ row }">{{ formatSize(row.size) }}</template>
          </el-table-column>
          <el-table-column prop="last_modified" :label="t('ossPage.modified')" width="180" />
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="storageDialog" :title="editingStorageId ? t('ossPage.editStorage') : t('ossPage.addStorage')" width="560px">
      <el-form label-width="110px">
        <el-form-item :label="t('common.name')"><el-input v-model="storageForm.name" /></el-form-item>
        <el-form-item :label="t('ossPage.provider')">
          <el-select v-model="storageForm.provider" style="width: 100%">
            <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="isLocalProvider" :label="t('ossPage.localPath')">
          <el-input v-model="storageForm.local_path" :placeholder="t('ossPage.localPathHint')" />
        </el-form-item>
        <template v-else>
          <el-form-item label="Endpoint"><el-input v-model="storageForm.endpoint" /></el-form-item>
          <el-form-item :label="t('ossPage.region')"><el-input v-model="storageForm.region" /></el-form-item>
          <el-form-item label="Bucket"><el-input v-model="storageForm.bucket" /></el-form-item>
          <el-form-item label="Access Key"><el-input v-model="storageForm.access_key" /></el-form-item>
          <el-form-item label="Secret Key"><el-input v-model="storageForm.secret_key" type="password" show-password /></el-form-item>
          <el-form-item :label="t('ossPage.pathPrefix')"><el-input v-model="storageForm.path_prefix" /></el-form-item>
          <el-form-item :label="t('ossPage.pathStyle')"><el-switch v-model="storageForm.use_path_style" /></el-form-item>
        </template>
        <el-form-item :label="t('common.description')"><el-input v-model="storageForm.remark" type="textarea" :rows="2" /></el-form-item>
        <el-form-item :label="t('common.enabled')"><el-switch v-model="storageForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="storageDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveStorage">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="taskDialog" :title="editingTaskId ? t('ossPage.editTask') : t('ossPage.addTask')" width="580px">
      <el-form label-width="120px">
        <el-form-item :label="t('common.name')"><el-input v-model="taskForm.name" /></el-form-item>
        <el-form-item :label="t('ossPage.mode')">
          <el-select v-model="taskForm.mode" style="width: 100%">
            <el-option v-for="m in modeOptions" :key="m.value" :label="m.label" :value="m.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('ossPage.localPath')"><el-input v-model="taskForm.local_path" :placeholder="t('ossPage.localPathHint')" /></el-form-item>
        <el-form-item :label="t('ossPage.sourceStorage')">
          <el-select v-model="taskForm.source_storage_id" clearable style="width: 100%">
            <el-option v-for="s in storages" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('ossPage.sourcePath')"><el-input v-model="taskForm.source_path" /></el-form-item>
        <el-form-item :label="t('ossPage.targetStorage')">
          <el-select v-model="taskForm.target_storage_id" clearable style="width: 100%">
            <el-option v-for="s in storages" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('ossPage.extraTargets')">
          <el-select v-model="taskForm.extra_target_ids" multiple clearable style="width: 100%" :placeholder="t('ossPage.extraTargetsHint')">
            <el-option v-for="s in storages" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('ossPage.targetPath')"><el-input v-model="taskForm.target_path" /></el-form-item>
        <el-form-item :label="t('ossPage.schedule')">
          <el-input v-model="taskForm.schedule" :placeholder="t('ossPage.scheduleHint')" />
        </el-form-item>
        <el-form-item :label="t('common.enabled')"><el-switch v-model="taskForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="taskDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveTask">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialog" :title="t('ossPage.import')" width="560px">
      <el-form label-width="100px">
        <el-form-item :label="t('ossPage.importMode')">
          <el-radio-group v-model="importMode">
            <el-radio value="merge">{{ t('ossPage.importMerge') }}</el-radio>
            <el-radio value="replace">{{ t('ossPage.importReplace') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('ossPage.importFile')">
          <el-upload :auto-upload="false" :show-file-list="false" accept=".json,application/json" :before-upload="onImportFile">
            <el-button>{{ t('ossPage.importSelect') }}</el-button>
          </el-upload>
        </el-form-item>
        <el-form-item :label="t('ossPage.importJson')">
          <el-input v-model="importText" type="textarea" :rows="12" :placeholder="t('ossPage.importPlaceholder')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!importText.trim()" @click="runImport">{{ t('ossPage.importRun') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logDialog" :title="t('ossPage.taskLog')" width="680px" @close="closeLogs">
      <div class="log-meta">
        <el-tag :type="statusTag(logTask?.running ? 'running' : logTask?.last_status)" size="small">
          {{ logTask?.running ? t('ossPage.running') : (logTask?.last_status || 'idle') }}
        </el-tag>
        <span v-if="logTask?.last_error" class="log-err">{{ logTask.last_error }}</span>
      </div>
      <div class="log-box">
        <pre>{{ logTask?.last_log || t('ossPage.noLog') }}</pre>
      </div>
      <template #footer>
        <el-button type="primary" @click="closeLogs">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.hint { margin-bottom: 16px; }
.toolbar { margin-bottom: 12px; }
.browse-bar { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.log-meta { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.log-err { color: var(--el-color-danger); font-size: 12px; }
.log-box {
  height: 360px; overflow: auto; background: #0f172a; border-radius: 8px; padding: 12px;
}
.log-box pre {
  margin: 0; color: #e2e8f0; font-family: Consolas, Monaco, monospace; font-size: 12px;
  white-space: pre-wrap; word-break: break-all;
}
</style>
