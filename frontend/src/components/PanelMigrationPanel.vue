<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { apiBaseURL } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'

const { t } = useI18n()

const loading = ref(false)
const exporting = ref(false)
const importing = ref(false)
const preview = ref<any>(null)
const includeLogs = ref(false)
const importMode = ref('replace')
const importFile = ref<File | null>(null)
const lastExport = ref<{ filename: string; size: number } | null>(null)

const cloudLoading = ref(false)
const cloudSaving = ref(false)
const cloudRunning = ref(false)
const cloudRestoring = ref(false)
const ossList = ref<any[]>([])
const cloudHistory = ref<any[]>([])
const cloudConfig = ref({
  enabled: false,
  schedule: '0 4 * * *',
  keep_count: 5,
  oss_storage_id: null as number | null,
  include_logs: false,
})
const cloudRestoreMode = ref('replace')

const countRows = computed(() => {
  const counts = preview.value?.manifest?.counts || {}
  return [
    { key: 'users', label: t('settings.migrationCountUsers') },
    { key: 'websites', label: t('settings.migrationCountWebsites') },
    { key: 'databases', label: t('settings.migrationCountDatabases') },
    { key: 'ssl_certificates', label: t('settings.migrationCountSSL') },
    { key: 'apps', label: t('settings.migrationCountApps') },
    { key: 'ftp_accounts', label: t('settings.migrationCountFTP') },
    { key: 'cron_jobs', label: t('settings.migrationCountCron') },
    { key: 'mail_domains', label: t('settings.migrationCountMailDomains') },
    { key: 'mailboxes', label: t('settings.migrationCountMailboxes') },
    { key: 'wordpress_sites', label: t('settings.migrationCountWordPress') },
    { key: 'extensions', label: t('settings.migrationCountExtensions') },
  ].map((row) => ({ ...row, value: counts[row.key] ?? 0 }))
})

async function loadPreview() {
  loading.value = true
  try {
    const res: any = await api.get('/settings/migration/preview')
    preview.value = res.data
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.migrationPreviewFailed'))
  } finally {
    loading.value = false
  }
}

async function runExport() {
  exporting.value = true
  try {
    const res: any = await api.post(
      '/settings/migration/export',
      { include_logs: includeLogs.value, include_secrets: true },
      { timeout: 600000 }
    )
    lastExport.value = { filename: res.data?.filename, size: res.data?.size || 0 }
    ElMessage.success(t('settings.migrationExportDone'))
    await downloadBundle(res.data?.filename)
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('settings.migrationExportFailed'))
  } finally {
    exporting.value = false
  }
}

async function downloadBundle(filename?: string) {
  const name = filename || lastExport.value?.filename
  if (!name) return
  const token = localStorage.getItem('token')
  const url = `${apiBaseURL()}/settings/migration/download?file=${encodeURIComponent(name)}`
  const res = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!res.ok) {
    ElMessage.error(t('settings.migrationDownloadFailed'))
    return
  }
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  a.click()
  URL.revokeObjectURL(a.href)
}

function onImportSelect(file: File) {
  importFile.value = file
  return false
}

async function runImport() {
  if (!importFile.value) {
    ElMessage.warning(t('settings.migrationImportNeedFile'))
    return
  }
  try {
    await ElMessageBox.confirm(
      importMode.value === 'replace'
        ? t('settings.migrationImportReplaceConfirm')
        : t('settings.migrationImportMergeConfirm'),
      t('common.warning'),
      { type: 'warning' }
    )
  } catch {
    return
  }
  importing.value = true
  try {
    const form = new FormData()
    form.append('file', importFile.value)
    form.append('mode', importMode.value)
    const token = localStorage.getItem('token')
    const res = await fetch(`${apiBaseURL()}/settings/migration/import`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    })
    const body = await res.json()
    if (!res.ok) {
      throw new Error(body?.error || body?.message || t('settings.migrationImportFailed'))
    }
    ElMessage.success(t('settings.migrationImportDone'))
    if (body?.data?.requires_restart) {
      ElMessage.warning(t('settings.migrationRestartHint'))
    }
    importFile.value = null
    await loadPreview()
  } catch (e: any) {
    ElMessage.error(e?.message || t('settings.migrationImportFailed'))
  } finally {
    importing.value = false
  }
}

onMounted(async () => {
  await loadPreview()
  await loadCloudSection()
})

async function loadCloudSection() {
  cloudLoading.value = true
  try {
    const [oss, cfg, hist]: any[] = await Promise.all([
      api.get('/oss/storages').catch(() => ({ data: [] })),
      api.get('/backup/panel/config').catch(() => ({ data: {} })),
      api.get('/backup/panel/history').catch(() => ({ data: [] })),
    ])
    ossList.value = (oss.data || []).filter((o: any) => o.provider !== 'local')
    const c = cfg.data || {}
    cloudConfig.value = {
      enabled: !!c.enabled,
      schedule: c.schedule || '0 4 * * *',
      keep_count: c.keep_count || 5,
      oss_storage_id: c.oss_storage_id ?? null,
      include_logs: !!c.include_logs,
    }
    cloudHistory.value = hist.data || []
  } finally {
    cloudLoading.value = false
  }
}

async function saveCloudConfig() {
  cloudSaving.value = true
  try {
    await api.put('/backup/panel/config', cloudConfig.value)
    ElMessage.success(t('panelBackup.configSaved'))
    await loadCloudSection()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.failed'))
  } finally {
    cloudSaving.value = false
  }
}

async function runCloudBackup() {
  if (!cloudConfig.value.oss_storage_id) {
    ElMessage.warning(t('panelBackup.needOss'))
    return
  }
  cloudRunning.value = true
  try {
    await api.post('/backup/panel/run', {
      oss_storage_id: cloudConfig.value.oss_storage_id,
      include_logs: cloudConfig.value.include_logs,
      keep_count: cloudConfig.value.keep_count,
    }, { timeout: 600000 })
    ElMessage.success(t('panelBackup.runDone'))
    await loadCloudSection()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.failed'))
  } finally {
    cloudRunning.value = false
  }
}

async function restoreFromCloud(row: any) {
  try {
    await ElMessageBox.confirm(
      cloudRestoreMode.value === 'replace'
        ? t('settings.migrationImportReplaceConfirm')
        : t('settings.migrationImportMergeConfirm'),
      t('common.warning'),
      { type: 'warning' }
    )
  } catch {
    return
  }
  cloudRestoring.value = true
  try {
    const res: any = await api.post('/backup/panel/restore', {
      record_id: row.id,
      mode: cloudRestoreMode.value,
    }, { timeout: 600000 })
    ElMessage.success(t('panelBackup.restoreDone'))
    if (res.data?.requires_restart) {
      ElMessage.warning(t('settings.migrationRestartHint'))
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.failed'))
  } finally {
    cloudRestoring.value = false
  }
}
</script>

<template>
  <el-card shadow="hover" class="settings-card" v-loading="loading">
    <template #header>{{ t('settings.migrationSection') }}</template>

    <el-alert type="info" show-icon :closable="false" :title="t('settings.migrationIntro')" class="migration-alert" />

    <div class="migration-actions">
      <el-checkbox v-model="includeLogs">{{ t('settings.migrationIncludeLogs') }}</el-checkbox>
      <el-button type="primary" :loading="exporting" @click="runExport">
        {{ t('settings.migrationExport') }}
      </el-button>
      <el-button v-if="lastExport?.filename" :disabled="exporting" @click="downloadBundle()">
        {{ t('settings.migrationDownloadAgain') }}
      </el-button>
    </div>

    <div v-if="lastExport" class="hint">
      {{ t('settings.migrationLastExport', { name: lastExport.filename, size: lastExport.size }) }}
    </div>

    <el-descriptions v-if="preview?.manifest" :column="2" border size="small" class="migration-counts">
      <el-descriptions-item v-for="row in countRows" :key="row.key" :label="row.label">
        {{ row.value }}
      </el-descriptions-item>
    </el-descriptions>

    <el-divider>{{ t('settings.migrationImportTitle') }}</el-divider>
    <p class="hint">{{ t('settings.migrationImportHint') }}</p>

    <el-form label-width="120px" class="import-form">
      <el-form-item :label="t('settings.migrationImportMode')">
        <el-radio-group v-model="importMode">
          <el-radio value="replace">{{ t('settings.migrationImportReplace') }}</el-radio>
          <el-radio value="merge">{{ t('settings.migrationImportMerge') }}</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="t('settings.migrationImportFile')">
        <el-upload drag :auto-upload="false" :show-file-list="true" :limit="1" accept=".tar.gz,.tgz" :before-upload="onImportSelect">
          <el-icon class="upload-icon"><UploadFilled /></el-icon>
          <div>{{ t('settings.migrationImportDrop') }}</div>
        </el-upload>
      </el-form-item>
      <el-form-item>
        <el-button type="warning" :loading="importing" @click="runImport">
          {{ t('settings.migrationImport') }}
        </el-button>
      </el-form-item>
    </el-form>

    <el-alert type="warning" show-icon :closable="false" :title="t('settings.migrationLimitations')" class="migration-alert" />

    <el-divider>{{ t('panelBackup.section') }}</el-divider>
    <el-alert type="info" show-icon :closable="false" :title="t('panelBackup.intro')" class="migration-alert" />

    <el-form v-loading="cloudLoading" label-width="130px" class="cloud-form">
      <el-form-item :label="t('panelBackup.ossTarget')">
        <el-select v-model="cloudConfig.oss_storage_id" clearable style="width: 100%">
          <el-option v-for="o in ossList" :key="o.id" :label="`${o.name} (${o.provider})`" :value="o.id" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('cron.schedule')">
        <el-input v-model="cloudConfig.schedule" />
      </el-form-item>
      <el-form-item :label="t('panelBackup.keepCount')">
        <el-input-number v-model="cloudConfig.keep_count" :min="1" :max="30" />
      </el-form-item>
      <el-form-item :label="t('common.status')">
        <el-switch v-model="cloudConfig.enabled" :active-text="t('panelBackup.scheduledOn')" />
      </el-form-item>
      <el-form-item>
        <el-checkbox v-model="cloudConfig.include_logs">{{ t('settings.migrationIncludeLogs') }}</el-checkbox>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="cloudRunning" @click="runCloudBackup">{{ t('panelBackup.runNow') }}</el-button>
        <el-button :loading="cloudSaving" @click="saveCloudConfig">{{ t('panelBackup.saveSchedule') }}</el-button>
      </el-form-item>
    </el-form>

    <el-table v-if="cloudHistory.length" :data="cloudHistory" size="small" stripe class="cloud-history">
      <el-table-column prop="filename" :label="t('common.name')" show-overflow-tooltip />
      <el-table-column prop="size" :label="t('common.size')" width="100">
        <template #default="{ row }">{{ row.size }} B</template>
      </el-table-column>
      <el-table-column prop="status" :label="t('common.status')" width="90" />
      <el-table-column :label="t('common.time')" width="170">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="120">
        <template #default="{ row }">
          <el-button text type="primary" :loading="cloudRestoring" @click="restoreFromCloud(row)">{{ t('panelBackup.restore') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-form-item v-if="cloudHistory.length" :label="t('settings.migrationImportMode')" label-width="130px">
      <el-radio-group v-model="cloudRestoreMode">
        <el-radio value="replace">{{ t('settings.migrationImportReplace') }}</el-radio>
        <el-radio value="merge">{{ t('settings.migrationImportMerge') }}</el-radio>
      </el-radio-group>
    </el-form-item>
  </el-card>
</template>

<style scoped>
.migration-alert {
  margin-bottom: 12px;
}

.migration-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.migration-counts {
  margin-top: 12px;
}

.import-form {
  margin-top: 8px;
}

.upload-icon {
  font-size: 42px;
  color: var(--cf-text-muted);
}

.cloud-form {
  margin-top: 8px;
}

.cloud-history {
  margin-bottom: 12px;
}
</style>
