<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const { t } = useI18n()

const tab = ref('tasks')
const list = ref<any[]>([])
const remotes = ref<any[]>([])
const ossList = ref<any[]>([])
const websites = ref<any[]>([])
const databases = ref<any[]>([])
const dialogVisible = ref(false)
const applyingPreset = ref('')
const defaultOssId = ref<number | null>(null)

const quickTemplates = [
  { key: 'websites', title: 'backup.templateAllSites', desc: 'backup.templateAllSitesDesc', schedule: '0 2 * * *', icon: '🌐' },
  { key: 'databases', title: 'backup.templateAllDbs', desc: 'backup.templateAllDbsDesc', schedule: '0 3 * * *', icon: '🗄️' },
]

const form = ref({
  name: '',
  type: 'website',
  target: '',
  schedule: '0 2 * * *',
  enabled: true,
  website_id: null as number | null,
  database_id: null as number | null,
  remote_id: null as number | null,
  oss_storage_id: null as number | null,
})

const remoteDialog = ref(false)
const remoteForm = ref({
  name: '',
  provider: 'ftp',
  host: '',
  port: 21,
  username: '',
  password: '',
  remote_path: '/backups',
  oss_storage_id: null as number | null,
  enabled: true,
})

const presets = [
  { label: 'backup.presetDaily', value: '0 2 * * *' },
  { label: 'backup.presetWeekly', value: '0 3 * * 0' },
  { label: 'backup.presetHourly', value: '0 * * * *' },
]

async function load() {
  const [tasks, rem, oss, sites, dbs]: any[] = await Promise.all([
    api.get('/backup'),
    api.get('/backup/remotes').catch(() => ({ data: [] })),
    api.get('/oss/storages').catch(() => ({ data: [] })),
    api.get('/websites').catch(() => ({ data: [] })),
    api.get('/databases').catch(() => ({ data: [] })),
  ])
  list.value = tasks.data || []
  remotes.value = rem.data || []
  ossList.value = oss.data || []
  websites.value = sites.data || []
  databases.value = dbs.data || []
  if (!defaultOssId.value && ossList.value.length) {
    const cloud = ossList.value.find((o: any) => o.provider !== 'local')
    defaultOssId.value = cloud?.id ?? ossList.value[0]?.id ?? null
  }
}

async function applyPreset(key: string, schedule: string) {
  applyingPreset.value = key
  try {
    const res: any = await api.post('/backup/presets', {
      preset: key,
      schedule,
      oss_storage_id: defaultOssId.value || undefined,
    })
    const d = res.data || {}
    ElMessage.success(t('backup.presetApplied', { created: d.created || 0, skipped: d.skipped || 0 }))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.failed'))
  } finally {
    applyingPreset.value = ''
  }
}

async function handleCreate() {
  await api.post('/backup', form.value)
  ElMessage.success(t('backup.created'))
  dialogVisible.value = false
  load()
}

async function handleRun(row: any) {
  await api.post(`/backup/${row.id}/run`)
  ElMessage.success(t('backup.runStarted'))
  setTimeout(load, 2000)
}

async function toggle(row: any) {
  await api.patch(`/backup/${row.id}/toggle`, { enabled: !row.enabled })
  load()
}

async function handleDelete(id: number) {
  await ElMessageBox.confirm(t('backup.deleteConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/backup/${id}`)
  ElMessage.success(t('common.deleted'))
  load()
}

async function createRemote() {
  await api.post('/backup/remotes', remoteForm.value)
  ElMessage.success(t('backup.remoteCreated'))
  remoteDialog.value = false
  load()
}

async function testRemote(id: number) {
  await api.post(`/backup/remotes/${id}/test`)
  ElMessage.success(t('backup.remoteTestOk'))
}

async function deleteRemote(id: number) {
  await ElMessageBox.confirm(t('backup.deleteRemoteConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/backup/remotes/${id}`)
  load()
}

function statusType(s: string) {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'running') return 'warning'
  if (s === 'partial') return 'warning'
  return 'info'
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('backup.title') }}</h2>
        <p class="subtitle">{{ t('backup.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <el-button v-if="tab === 'tasks'" type="primary" @click="dialogVisible = true">{{ t('backup.add') }}</el-button>
        <el-button v-if="tab === 'remotes'" type="primary" @click="remoteDialog = true">{{ t('backup.addRemote') }}</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon class="hint">
      <template #title>{{ t('backup.whatIsTitle') }}</template>
      <template #default>{{ t('backup.whatIsBody') }}</template>
    </el-alert>

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('backup.tabTasks')" name="tasks">
        <h3 class="section-title">{{ t('backup.quickTitle') }}</h3>
        <div v-if="ossList.length" class="oss-pick-row">
          <span class="oss-pick-label">{{ t('backup.defaultOss') }}</span>
          <el-select v-model="defaultOssId" clearable :placeholder="t('backup.defaultOssHint')" style="width: 280px">
            <el-option v-for="o in ossList" :key="o.id" :label="`${o.name} (${o.provider})`" :value="o.id" />
          </el-select>
        </div>
        <div class="template-grid">
          <el-card v-for="tpl in quickTemplates" :key="tpl.key" shadow="never" class="template-card">
            <div class="template-head">
              <span class="template-icon">{{ tpl.icon }}</span>
              <strong>{{ t(tpl.title) }}</strong>
            </div>
            <p class="template-desc">{{ t(tpl.desc) }}</p>
            <el-button type="primary" size="small" :loading="applyingPreset === tpl.key" @click="applyPreset(tpl.key, tpl.schedule)">
              {{ t('backup.templateApply') }}
            </el-button>
          </el-card>
        </div>
        <el-table :data="list" stripe>
          <el-table-column prop="name" :label="t('common.name')" width="140" />
          <el-table-column prop="type" :label="t('common.type')" width="100" />
          <el-table-column prop="target" :label="t('backup.target')" show-overflow-tooltip />
          <el-table-column prop="schedule" :label="t('cron.schedule')" width="130" />
          <el-table-column :label="t('backup.lastRun')" width="160">
            <template #default="{ row }">
              <span v-if="row.last_run">{{ new Date(row.last_run).toLocaleString() }}</span>
              <span v-else>—</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('backup.lastStatus')" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.last_status" size="small" :type="statusType(row.last_status)">{{ row.last_status }}</el-tag>
              <span v-else>—</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.status')" width="80">
            <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggle(row)" /></template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="180" fixed="right">
            <template #default="{ row }">
              <el-button text type="primary" @click="handleRun(row)">{{ t('backup.runNow') }}</el-button>
              <el-button text type="danger" @click="handleDelete(row.id)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('backup.tabRemotes')" name="remotes">
        <el-table :data="remotes" stripe>
          <el-table-column prop="name" :label="t('common.name')" />
          <el-table-column prop="provider" :label="t('backup.provider')" width="120" />
          <el-table-column prop="host" :label="t('databases.host')" />
          <el-table-column :label="t('common.actions')" width="200">
            <template #default="{ row }">
              <el-button text @click="testRemote(row.id)">{{ t('backup.test') }}</el-button>
              <el-button text type="danger" @click="deleteRemote(row.id)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="dialogVisible" :title="t('backup.addTitle')" width="580px">
      <el-form :model="form" label-width="110px">
        <el-form-item :label="t('common.name')"><el-input v-model="form.name" /></el-form-item>
        <el-form-item :label="t('common.type')">
          <el-select v-model="form.type">
            <el-option :label="t('backup.typeWebsite')" value="website" />
            <el-option :label="t('backup.typeDatabase')" value="database" />
            <el-option :label="t('backup.typeDirectory')" value="directory" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'website'" :label="t('menu.website')">
          <el-select v-model="form.website_id" filterable clearable style="width:100%">
            <el-option v-for="w in websites" :key="w.id" :label="w.domain" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'database'" :label="t('menu.database')">
          <el-select v-model="form.database_id" filterable clearable style="width:100%">
            <el-option v-for="d in databases" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'directory'" :label="t('backup.target')">
          <el-input v-model="form.target" :placeholder="t('backup.dirPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('cron.schedule')">
          <el-input v-model="form.schedule" />
          <div class="preset-row">
            <el-button v-for="p in presets" :key="p.value" size="small" @click="form.schedule = p.value">{{ t(p.label) }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('backup.remoteTarget')">
          <el-select v-model="form.remote_id" clearable style="width:100%">
            <el-option v-for="r in remotes" :key="r.id" :label="r.name" :value="r.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('backup.ossTarget')">
          <el-select v-model="form.oss_storage_id" clearable style="width:100%">
            <el-option v-for="o in ossList" :key="o.id" :label="o.name" :value="o.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="handleCreate">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remoteDialog" :title="t('backup.addRemote')" width="520px">
      <el-form :model="remoteForm" label-width="100px">
        <el-form-item :label="t('common.name')"><el-input v-model="remoteForm.name" /></el-form-item>
        <el-form-item :label="t('backup.provider')">
          <el-select v-model="remoteForm.provider">
            <el-option label="FTP" value="ftp" />
            <el-option label="SFTP" value="sftp" />
            <el-option label="WebDAV" value="webdav" />
            <el-option label="OSS / S3" value="oss" />
          </el-select>
        </el-form-item>
        <template v-if="remoteForm.provider !== 'oss'">
          <el-form-item :label="t('databases.host')"><el-input v-model="remoteForm.host" /></el-form-item>
          <el-form-item :label="t('common.port')"><el-input-number v-model="remoteForm.port" :min="1" :max="65535" /></el-form-item>
          <el-form-item :label="t('common.username')"><el-input v-model="remoteForm.username" /></el-form-item>
          <el-form-item :label="t('common.password')"><el-input v-model="remoteForm.password" type="password" show-password /></el-form-item>
        </template>
        <el-form-item v-else :label="t('menu.oss')">
          <el-select v-model="remoteForm.oss_storage_id" style="width:100%">
            <el-option v-for="o in ossList" :key="o.id" :label="o.name" :value="o.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('backup.remotePath')"><el-input v-model="remoteForm.remote_path" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="remoteDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="createRemote">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.subtitle { margin: 4px 0 0; font-size: 13px; color: var(--el-text-color-secondary); }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.hint { margin-bottom: 16px; }
.section-title { margin: 0 0 12px; font-size: 15px; font-weight: 600; }
.oss-pick-row { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.oss-pick-label { font-size: 13px; color: var(--el-text-color-secondary); }
.template-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 10px; margin-bottom: 16px; }
.template-card { border: 1px solid var(--el-border-color-lighter); }
.template-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.template-icon { font-size: 22px; }
.template-desc { margin: 0 0 12px; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; }
.preset-row { margin-top: 8px; display: flex; gap: 6px; flex-wrap: wrap; }
</style>
