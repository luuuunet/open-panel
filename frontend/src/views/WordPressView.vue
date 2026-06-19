<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Download, EditPen, FolderOpened, Promotion, RefreshRight, Setting } from '@element-plus/icons-vue'
import WordPressBackupDialog from '@/components/WordPressBackupDialog.vue'
import WordPressSEOPushDialog from '@/components/WordPressSEOPushDialog.vue'

const { t } = useI18n()
const router = useRouter()

const sites = ref<any[]>([])
const phpVersions = ref<any[]>([])
const dialogVisible = ref(false)
const deployLogVisible = ref(false)
const deployLogs = ref<string[]>([])
const deployStatus = ref<'running' | 'success' | 'failed'>('running')
const deployDomain = ref('')
const deploying = ref(false)
const credDialog = ref(false)
const deployCredentials = ref<any>(null)
const logBoxRef = ref<HTMLElement | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

const fileDrawer = ref(false)
const domainDrawer = ref(false)
const fileEntries = ref<any[]>([])
const fileContent = ref('')
const editingFile = ref('')
const currentSite = ref<any>(null)
const domainList = ref<any[]>([])
const newDomain = ref('')

const backupDialogVisible = ref(false)
const backupSiteId = ref<number | null>(null)
const backupDomain = ref('')

const seoDialogVisible = ref(false)
const seoSiteId = ref<number | null>(null)
const seoDomain = ref('')

const settingsVisible = ref(false)
const settingsSaving = ref(false)
const settingsForm = ref({
  id: 0,
  domain: '',
  root_path: '',
  path: '',
  php_version: '',
  version: '',
  remark: '',
  auto_ssl: false,
  ssl_email: '',
  cloudflare_cdn: false,
})

const form = ref({
  domain: '',
  path: '',
  version: '6.7',
  php_version: '8.3',
  domains_text: '',
  auto_ssl: true,
  ssl_email: '',
  cloudflare_cdn: false,
  seo_push_enabled: true,
  database_mode: 'auto' as 'auto' | 'custom' | 'existing',
  database_id: undefined as number | undefined,
  db_name: '',
  db_user: '',
  db_password: '',
  db_host: '127.0.0.1',
  db_port: 3306,
})

const mysqlDatabases = ref<any[]>([])

async function load() {
  const [wpRes, phpRes, dbRes]: any[] = await Promise.all([
    api.get('/wordpress'),
    api.get('/php/versions'),
    api.get('/databases').catch(() => ({ data: [] })),
  ])
  sites.value = wpRes.data || []
  phpVersions.value = phpRes.data || []
  mysqlDatabases.value = (dbRes.data || []).filter((d: any) => d.type === 'mysql' || d.type === 'mariadb' || !d.type)
  if (phpVersions.value.length && !form.value.php_version) {
    form.value.php_version = phpVersions.value.find((p: any) => p.default)?.version || phpVersions.value[0].version
  }
}

async function handleCreate() {
  if (deploying.value) return
  if (!form.value.domain.trim()) {
    ElMessage.warning(t('wp.domainPlaceholder'))
    return
  }
  if (form.value.database_mode === 'custom') {
    if (!form.value.db_name.trim() || !form.value.db_user.trim() || !form.value.db_password) {
      ElMessage.warning(t('wp.dbRequired'))
      return
    }
  }
  if (form.value.database_mode === 'existing' && !form.value.database_id) {
    ElMessage.warning(t('wp.selectDatabase'))
    return
  }
  deploying.value = true
  try {
    const checkRes: any = await api.post('/domains/check', {
      domains: [form.value.domain],
      domains_text: form.value.domains_text,
    })
    if (!checkRes.data?.available) {
      const c = checkRes.data?.conflicts?.[0]
      ElMessage.error(c ? `${c.domain}: ${c.owner}` : t('wp.domainTaken'))
      return
    }
    const res: any = await api.post('/wordpress', form.value)
    const data = res.data || {}
    dialogVisible.value = false
    deployDomain.value = data.domain || form.value.domain
    deployLogs.value = data.logs || []
    deployStatus.value = 'running'
    deployLogVisible.value = true
    form.value.domain = ''
    form.value.domains_text = ''
    await scrollLogToBottom()
    if (data.job_id) {
      startDeployPoll(data.job_id)
    }
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('wp.createFailed')))
  } finally {
    deploying.value = false
  }
}

function startDeployPoll(jobId: string) {
  stopDeployPoll()
  pollTimer = setInterval(async () => {
    try {
      const res: any = await api.get(`/wordpress/deploy/${jobId}`)
      const job = res.data
      if (!job) return
      deployLogs.value = job.logs || []
      deployDomain.value = job.domain || deployDomain.value
      await scrollLogToBottom()
      if (job.status === 'success') {
        deployStatus.value = 'success'
        stopDeployPoll()
        ElMessage.success(t('wp.created'))
        if (job.ftp_password || job.db_password) {
          deployCredentials.value = job
          credDialog.value = true
        }
        load()
      } else if (job.status === 'failed') {
        deployStatus.value = 'failed'
        stopDeployPoll()
        ElMessage.error(job.error || t('wp.deployFailed'))
        load()
      }
    } catch {
      /* keep polling */
    }
  }, 600)
}

function stopDeployPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function scrollLogToBottom() {
  await nextTick()
  const el = logBoxRef.value
  if (el) el.scrollTop = el.scrollHeight
}

function closeDeployLog() {
  if (deployStatus.value === 'running') return
  deployLogVisible.value = false
}

onUnmounted(stopDeployPoll)

async function openDomainManage(row: any) {
  currentSite.value = row
  domainDrawer.value = true
  newDomain.value = ''
  await refreshDomains(row.id)
}

async function refreshDomains(siteId: number) {
  const res: any = await api.get(`/wordpress/${siteId}/domains`)
  domainList.value = res.data || []
}

async function addDomain() {
  if (!currentSite.value || !newDomain.value.trim()) return
  try {
    const checkRes: any = await api.post('/domains/check', {
      domains: [newDomain.value.trim()],
      exclude_wp_site_id: currentSite.value.id,
    })
    if (!checkRes.data?.available) {
      const c = checkRes.data?.conflicts?.[0]
      ElMessage.error(c ? `${c.domain}: ${c.owner}` : t('wp.domainTaken'))
      return
    }
    await api.post(`/wordpress/${currentSite.value.id}/domains`, { domain: newDomain.value.trim() })
    ElMessage.success(t('wp.domainAdded'))
    newDomain.value = ''
    await refreshDomains(currentSite.value.id)
    load()
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  }
}

async function removeDomain(row: any) {
  if (row.type === 'primary') {
    ElMessage.warning(t('wp.cannotRemovePrimary'))
    return
  }
  await ElMessageBox.confirm(t('common.confirmDelete'), t('common.warning'), { type: 'warning' })
  await api.delete(`/wordpress/${currentSite.value.id}/domains/${row.id}`)
  ElMessage.success(t('wp.domainRemoved'))
  await refreshDomains(currentSite.value.id)
  load()
}

async function applyDomains() {
  if (!currentSite.value) return
  await api.post(`/wordpress/${currentSite.value.id}/domains/apply`)
  ElMessage.success(t('wp.domainsApplied'))
  load()
}

function allDomains(row: any) {
  const list = row.domains || []
  if (list.length) return list
  return [{ domain: row.domain, type: 'primary' }]
}

async function showSiteInfo(row: any) {
  try {
    const res: any = await api.get(`/wordpress/${row.id}/credentials`)
    deployCredentials.value = res.data || {}
    credDialog.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('wp.loadCredentialsFailed')))
  }
}

async function repair(row: any) {
  try {
    const res: any = await api.post(`/wordpress/${row.id}/repair`)
    ElMessage.success(t('wp.repaired'))
    const data = res.data || {}
    if (data.ftp_password || data.ftp_user) {
      deployCredentials.value = data
      credDialog.value = true
    }
    load()
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  }
}

function openSettings(row: any) {
  settingsForm.value = {
    id: row.id,
    domain: row.domain,
    root_path: row.root_path || '',
    path: row.path || '',
    php_version: row.php_version || '8.3',
    version: row.version || '6.7',
    remark: row.remark || '',
    auto_ssl: !!row.auto_ssl,
    ssl_email: row.ssl_email || '',
    cloudflare_cdn: !!row.cloudflare_cdn,
  }
  settingsVisible.value = true
}

async function issueSSL(row: any) {
  try {
    await api.post(`/wordpress/${row.id}/ssl`, { email: row.ssl_email || '' })
    ElMessage.success(t('wp.sslIssued'))
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('wp.sslIssueFailed')))
  }
}

async function saveSettings() {
  if (!settingsForm.value.root_path.trim()) {
    ElMessage.warning(t('wp.rootPathRequired'))
    return
  }
  settingsSaving.value = true
  try {
    await api.patch(`/wordpress/${settingsForm.value.id}`, {
      root_path: settingsForm.value.root_path.trim(),
      path: settingsForm.value.path.trim(),
      php_version: settingsForm.value.php_version,
      version: settingsForm.value.version,
      remark: settingsForm.value.remark.trim(),
      auto_ssl: settingsForm.value.auto_ssl,
      ssl_email: settingsForm.value.ssl_email.trim(),
      cloudflare_cdn: settingsForm.value.cloudflare_cdn,
    })
    ElMessage.success(t('wp.siteUpdated'))
    settingsVisible.value = false
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    settingsSaving.value = false
  }
}

async function backup(row: any) {
  backupSiteId.value = row.id
  backupDomain.value = row.domain
  backupDialogVisible.value = true
}

function openSEOPush(row: any) {
  seoSiteId.value = row.id
  seoDomain.value = row.domain
  seoDialogVisible.value = true
}

function seoPushLabel(row: any) {
  if (row.last_seo_push_status) return row.last_seo_push_status
  if (row.seo_push_enabled) return t('wp.seoPushOn')
  return t('wp.seoPushOff')
}

function backupLabel(row: any) {
  if (row.backup_status && row.backup_status !== 'none') return row.backup_status
  return t('wpBackup.none')
}

async function handleDelete(id: number) {
  await ElMessageBox.confirm(t('common.confirmDelete'), t('common.warning'), { type: 'warning' })
  await api.delete(`/wordpress/${id}`)
  ElMessage.success(t('common.deleted'))
  load()
}

function openFiles(row: any) {
  router.push({ path: '/files', query: { path: row.root_path || row.path } })
}

async function openFileManager(row: any) {
  currentSite.value = row
  fileDrawer.value = true
  editingFile.value = ''
  fileContent.value = ''
  await loadSiteFiles(row.root_path || row.path)
}

async function loadSiteFiles(dir: string) {
  const res: any = await api.get('/files', { params: { path: dir } })
  fileEntries.value = res.data || []
}

async function openSiteFile(path: string) {
  const res: any = await api.get('/files/content', { params: { path } })
  editingFile.value = path
  fileContent.value = res.data?.content ?? ''
}

async function saveSiteFile() {
  await api.put('/files/content', { path: editingFile.value, content: fileContent.value })
  ElMessage.success(t('common.saved'))
}

function statusTag(row: any) {
  if (row.status === 'running') return 'success'
  if (row.status === 'deploying') return 'warning'
  if (row.status === 'error') return 'danger'
  return 'info'
}

function statusLabel(row: any) {
  if (row.status === 'deploying') return t('wp.deploying')
  return row.status
}

function sslLabel(row: any) {
  if (row.ssl || row.ssl_status === 'active') return t('wp.sslActive')
  if (row.ssl_status === 'failed') return t('wp.sslFailed')
  if (row.ssl_status === 'skipped') return t('wp.sslSkipped')
  if (row.auto_ssl) return t('wp.sslPending')
  return t('wp.sslNone')
}

function sslTagType(row: any) {
  if (row.ssl || row.ssl_status === 'active') return 'success'
  if (row.ssl_status === 'failed') return 'danger'
  if (row.ssl_status === 'skipped') return 'info'
  if (row.auto_ssl) return 'warning'
  return 'info'
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-header">
      <h2>{{ t('page.wordpress') }}</h2>
      <el-button type="primary" @click="dialogVisible = true">{{ t('wp.deploy') }}</el-button>
    </div>

    <el-alert type="info" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>{{ t('wp.hint') }}</template>
      <template #default>{{ t('wp.seoPushDocHint') }}</template>
    </el-alert>

    <el-table :data="sites" stripe>
      <el-table-column :label="t('wp.primaryDomain')" min-width="140">
        <template #default="{ row }">
          <el-tooltip :content="t('wp.siteInfo')" placement="top">
            <span class="site-info-link" @click="showSiteInfo(row)">{{ row.domain }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column :label="t('wp.boundDomains')" min-width="220">
        <template #default="{ row }">
          <el-tag
            v-for="d in allDomains(row)"
            :key="d.domain"
            size="small"
            :type="d.type === 'primary' ? 'primary' : 'info'"
            style="margin: 2px"
          >
            {{ d.domain }}
          </el-tag>
          <el-button text type="primary" size="small" @click="openDomainManage(row)">{{ t('wp.domainManage') }}</el-button>
        </template>
      </el-table-column>
      <el-table-column :label="t('wp.rootPath')" min-width="220">
        <template #default="{ row }">
          <div class="root-path-cell" @click="openSettings(row)">
            <span class="root-path-text" :title="row.root_path">{{ row.root_path }}</span>
            <el-icon class="root-path-edit"><EditPen /></el-icon>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="version" :label="t('wp.wpVersion')" width="80" />
      <el-table-column :label="t('wp.phpVersion')" width="90">
        <template #default="{ row }">
          <el-tag size="small" type="warning">PHP {{ row.php_version || '-' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('wp.nginxVersion')" width="100">
        <template #default="{ row }">
          <el-tag size="small">{{ row.nginx_version || '-' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="status" :label="t('common.status')" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTag(row)" size="small">{{ statusLabel(row) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('wp.ssl')" width="120" align="center">
        <template #default="{ row }">
          <div class="ssl-tags">
            <el-tag v-if="row.cloudflare_cdn" size="small" type="warning" effect="plain">CDN</el-tag>
            <el-tag :type="sslTagType(row)" size="small" effect="plain">{{ sslLabel(row) }}</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="remark" :label="t('wp.remark')" min-width="120" show-overflow-tooltip />
      <el-table-column :label="t('wp.backup')" width="90" align="center">
        <template #default="{ row }">
          <el-tag
            size="small"
            class="backup-tag-clickable"
            :type="row.backup_status && row.backup_status !== 'none' ? 'success' : 'warning'"
            effect="plain"
            :title="t('wpBackup.clickHint')"
            @click="backup(row)"
          >
            {{ backupLabel(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('wp.seoPush')" width="90" align="center">
        <template #default="{ row }">
          <el-tag
            size="small"
            class="backup-tag-clickable"
            :type="row.last_seo_push_status === 'success' ? 'success' : row.seo_push_enabled ? 'primary' : 'info'"
            effect="plain"
            :title="t('wp.seoPushClickHint')"
            @click="openSEOPush(row)"
          >
            {{ seoPushLabel(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="230" fixed="right" align="center">
        <template #default="{ row }">
          <div class="wp-actions">
            <el-tooltip :content="t('wp.siteSettings')" placement="top">
              <el-button text type="primary" :icon="Setting" @click="openSettings(row)" />
            </el-tooltip>
            <el-tooltip :content="t('wp.editFiles')" placement="top">
              <el-button text type="primary" :icon="EditPen" @click="openFileManager(row)" />
            </el-tooltip>
            <el-tooltip :content="t('wp.openInFiles')" placement="top">
              <el-button text :icon="FolderOpened" @click="openFiles(row)" />
            </el-tooltip>
            <el-tooltip :content="t('wp.repair')" placement="top">
              <el-button text type="warning" :icon="RefreshRight" @click="repair(row)" />
            </el-tooltip>
            <el-tooltip v-if="!row.ssl && row.ssl_status !== 'active'" :content="t('wp.issueSSL')" placement="top">
              <el-button text type="success" @click="issueSSL(row)">SSL</el-button>
            </el-tooltip>
            <el-tooltip :content="t('wpBackup.runNow')" placement="top">
              <el-button text type="success" :icon="Download" @click="backup(row)" />
            </el-tooltip>
            <el-tooltip :content="t('wp.seoPushNow')" placement="top">
              <el-button text type="primary" :icon="Promotion" @click="openSEOPush(row)" />
            </el-tooltip>
            <el-tooltip :content="t('common.delete')" placement="top">
              <el-button text type="danger" :icon="Delete" @click="handleDelete(row.id)" />
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="t('wp.deploy')" width="560px">
      <el-form :model="form" label-width="110px">
        <el-form-item :label="t('wp.primaryDomain')"><el-input v-model="form.domain" placeholder="example.com" /></el-form-item>
        <el-form-item :label="t('wp.extraDomains')">
          <el-input v-model="form.domains_text" type="textarea" :rows="3" :placeholder="t('wp.extraDomainsHint')" />
        </el-form-item>
        <el-form-item :label="t('wp.basePath')">
          <el-input v-model="form.path" :placeholder="t('wp.basePathHint')" />
        </el-form-item>
        <el-form-item :label="t('wp.wpVersion')"><el-input v-model="form.version" placeholder="6.7" /></el-form-item>
        <el-form-item :label="t('wp.phpVersion')">
          <el-select v-model="form.php_version" style="width: 100%">
            <el-option v-for="p in phpVersions" :key="p.key" :label="`PHP ${p.version} (${p.status})`" :value="p.version" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('wp.cloudflareCDN')">
          <el-switch v-model="form.cloudflare_cdn" @change="(v: boolean) => { if (v) form.auto_ssl = false }" />
          <p class="form-hint">{{ t('wp.cloudflareCDNHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('wp.ssl')">
          <el-switch v-model="form.auto_ssl" :disabled="form.cloudflare_cdn" />
          <p class="form-hint">{{ form.cloudflare_cdn ? t('wp.cloudflareCDNSSLHint') : t('wp.autoSSLHint') }}</p>
        </el-form-item>
        <el-form-item v-if="form.auto_ssl" :label="t('wp.sslEmail')">
          <el-input v-model="form.ssl_email" placeholder="admin@example.com" />
        </el-form-item>
        <el-form-item :label="t('wp.seoPush')">
          <el-switch v-model="form.seo_push_enabled" />
          <p class="form-hint">{{ t('wp.seoPushDeployHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('wp.database')">
          <el-radio-group v-model="form.database_mode">
            <el-radio value="auto">{{ t('wp.databaseAuto') }}</el-radio>
            <el-radio value="custom">{{ t('wp.databaseCustom') }}</el-radio>
            <el-radio value="existing">{{ t('wp.databaseExisting') }}</el-radio>
          </el-radio-group>
          <p v-if="form.database_mode === 'auto'" class="form-hint">{{ t('wp.databaseAutoHint') }}</p>
        </el-form-item>
        <template v-if="form.database_mode === 'existing'">
          <el-form-item :label="t('wp.selectDatabase')">
            <el-select v-model="form.database_id" style="width: 100%" filterable :placeholder="t('wp.selectDatabase')">
              <el-option
                v-for="db in mysqlDatabases"
                :key="db.id"
                :label="`${db.name} (${db.username}@${db.host || '127.0.0.1'})`"
                :value="db.id"
              />
            </el-select>
          </el-form-item>
        </template>
        <template v-if="form.database_mode === 'custom'">
          <el-form-item :label="t('wp.dbHost')"><el-input v-model="form.db_host" placeholder="127.0.0.1" /></el-form-item>
          <el-form-item :label="t('wp.dbPort')"><el-input-number v-model="form.db_port" :min="1" :max="65535" style="width: 100%" /></el-form-item>
          <el-form-item :label="t('wp.dbName')"><el-input v-model="form.db_name" /></el-form-item>
          <el-form-item :label="t('wp.dbUser')"><el-input v-model="form.db_user" /></el-form-item>
          <el-form-item :label="t('wp.dbPassword')"><el-input v-model="form.db_password" type="password" show-password /></el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false" :disabled="deploying">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="deploying" @click="handleCreate">{{ t('wp.deploy') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="`${t('wp.siteSettings')} — ${settingsForm.domain}`" width="560px">
      <el-form :model="settingsForm" label-width="110px">
        <el-form-item :label="t('wp.rootPath')">
          <el-input v-model="settingsForm.root_path" :placeholder="t('wp.rootPathHint')" />
          <p class="form-hint">{{ t('wp.rootPathEditHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('wp.basePath')">
          <el-input v-model="settingsForm.path" :placeholder="t('wp.basePathHint')" />
        </el-form-item>
        <el-form-item :label="t('wp.phpVersion')">
          <el-select v-model="settingsForm.php_version" style="width: 100%">
            <el-option v-for="p in phpVersions" :key="p.key" :label="`PHP ${p.version}`" :value="p.version" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('wp.wpVersion')">
          <el-input v-model="settingsForm.version" />
        </el-form-item>
        <el-form-item :label="t('wp.remark')">
          <el-input v-model="settingsForm.remark" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="t('wp.cloudflareCDN')">
          <el-switch v-model="settingsForm.cloudflare_cdn" @change="(v: boolean) => { if (v) settingsForm.auto_ssl = false }" />
          <p class="form-hint">{{ t('wp.cloudflareCDNHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('wp.ssl')">
          <el-switch v-model="settingsForm.auto_ssl" :disabled="settingsForm.cloudflare_cdn" />
          <p class="form-hint">{{ settingsForm.cloudflare_cdn ? t('wp.cloudflareCDNSSLHint') : t('wp.autoSSLHint') }}</p>
        </el-form-item>
        <el-form-item v-if="settingsForm.auto_ssl" :label="t('wp.sslEmail')">
          <el-input v-model="settingsForm.ssl_email" placeholder="admin@example.com" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="settingsSaving" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="deployLogVisible"
      :title="`${t('wp.deployLog')} — ${deployDomain}`"
      width="640px"
      :close-on-click-modal="false"
      :show-close="deployStatus !== 'running'"
      @close="closeDeployLog"
    >
      <div class="deploy-log-header">
        <el-tag v-if="deployStatus === 'running'" type="warning">{{ t('wp.deployRunning') }}</el-tag>
        <el-tag v-else-if="deployStatus === 'success'" type="success">{{ t('wp.deploySuccess') }}</el-tag>
        <el-tag v-else type="danger">{{ t('wp.deployFailed') }}</el-tag>
      </div>
      <div ref="logBoxRef" class="deploy-log-box">
        <div v-for="(line, i) in deployLogs" :key="i" class="deploy-log-line">{{ line }}</div>
        <div v-if="deployStatus === 'running'" class="deploy-log-line deploy-log-cursor">▌</div>
      </div>
      <template #footer>
        <el-button v-if="deployStatus !== 'running'" type="primary" @click="deployLogVisible = false">
          {{ t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="credDialog" :title="t('websites.credentials')" width="480px">
      <el-descriptions v-if="deployCredentials" :column="1" border>
        <el-descriptions-item v-if="deployCredentials.ftp_user" :label="t('websites.ftpUser')">
          {{ deployCredentials.ftp_user }}
        </el-descriptions-item>
        <el-descriptions-item v-if="deployCredentials.ftp_password" :label="t('websites.ftpPassword')">
          {{ deployCredentials.ftp_password }}
        </el-descriptions-item>
        <el-descriptions-item v-if="deployCredentials.db_name" :label="t('websites.dbName')">
          {{ deployCredentials.db_name }}
        </el-descriptions-item>
        <el-descriptions-item v-if="deployCredentials.db_user" :label="t('websites.dbUser')">
          {{ deployCredentials.db_user }}
        </el-descriptions-item>
        <el-descriptions-item v-if="deployCredentials.db_password" :label="t('websites.dbPassword')">
          {{ deployCredentials.db_password }}
        </el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button type="primary" @click="credDialog = false">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="domainDrawer" :title="`${t('wp.domainManage')} — ${currentSite?.domain || ''}`" size="480px">
      <div class="domain-panel">
        <el-input v-model="newDomain" :placeholder="t('wp.domainPlaceholder')" style="margin-bottom: 8px">
          <template #append>
            <el-button type="primary" @click="addDomain">{{ t('wp.addDomain') }}</el-button>
          </template>
        </el-input>
        <el-table :data="domainList" stripe size="small">
          <el-table-column prop="domain" :label="t('wp.domain')" />
          <el-table-column prop="type" :label="t('common.type')" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.type === 'primary' ? 'primary' : 'info'">
                {{ row.type === 'primary' ? t('wp.primaryDomain') : t('wp.aliasDomain') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="80">
            <template #default="{ row }">
              <el-button v-if="row.type !== 'primary'" text type="danger" size="small" @click="removeDomain(row)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-button type="primary" style="margin-top: 16px" @click="applyDomains">{{ t('wp.applyDomains') }}</el-button>
      </div>
    </el-drawer>

    <el-drawer v-model="fileDrawer" :title="`${t('wp.editFiles')} — ${currentSite?.domain || ''}`" size="60%">
      <div v-if="currentSite" class="file-drawer">
        <el-alert v-if="currentSite.nginx_conf" :title="`${t('wp.nginxConf')}: ${currentSite.nginx_conf}`" type="info" :closable="false" style="margin-bottom: 12px" />
        <el-table :data="fileEntries" stripe max-height="280" @row-dblclick="(row: any) => row.is_dir ? loadSiteFiles(row.path) : openSiteFile(row.path)">
          <el-table-column prop="name" :label="t('wp.fileName')" />
          <el-table-column prop="size" :label="t('wp.fileSize')" width="90" />
          <el-table-column :label="t('common.actions')" width="100">
            <template #default="{ row }">
              <el-button v-if="!row.is_dir" text type="primary" @click="openSiteFile(row.path)">{{ t('common.edit') }}</el-button>
              <el-button v-else text @click="loadSiteFiles(row.path)">{{ t('wp.openDir') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div v-if="editingFile" class="file-editor">
          <div class="file-editor-header">
            <span>{{ editingFile }}</span>
            <el-button type="primary" size="small" @click="saveSiteFile">{{ t('common.save') }}</el-button>
          </div>
          <el-input v-model="fileContent" type="textarea" :rows="16" />
        </div>
      </div>
    </el-drawer>

    <WordPressBackupDialog
      v-model:visible="backupDialogVisible"
      :site-id="backupSiteId"
      :domain="backupDomain"
      @updated="load"
    />
    <WordPressSEOPushDialog
      v-model="seoDialogVisible"
      :site-id="seoSiteId"
      :domain="seoDomain"
      @updated="load"
    />
  </div>
</template>

<style scoped>
.file-drawer { display: flex; flex-direction: column; gap: 16px; }
.domain-panel { display: flex; flex-direction: column; }
.file-editor-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; font-size: 13px; color: #606266; word-break: break-all; }
.deploy-log-header { margin-bottom: 10px; }
.deploy-log-box {
  background: #1e1e1e;
  color: #d4d4d4;
  font-family: Consolas, Monaco, 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.55;
  padding: 12px 14px;
  border-radius: 6px;
  max-height: 360px;
  overflow-y: auto;
  min-height: 200px;
}
.deploy-log-line { white-space: pre-wrap; word-break: break-all; }
.deploy-log-cursor { color: var(--el-color-success); animation: blink 1s step-end infinite; }
@keyframes blink { 50% { opacity: 0; } }
.backup-tag-clickable { cursor: pointer; transition: opacity 0.15s; }
.backup-tag-clickable:hover { opacity: 0.85; }
.ssl-tags { display: flex; flex-direction: column; align-items: center; gap: 4px; }
.form-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
.site-info-link {
  color: var(--el-color-primary);
  cursor: pointer;
  font-weight: 600;
}
.site-info-link:hover {
  text-decoration: underline;
}
.root-path-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  max-width: 100%;
}
.root-path-cell:hover .root-path-edit {
  opacity: 1;
}
.root-path-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}
.root-path-edit {
  flex-shrink: 0;
  opacity: 0.35;
  color: var(--el-color-primary);
  font-size: 14px;
}
.wp-actions {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  flex-wrap: nowrap;
}
.wp-actions :deep(.el-button) {
  padding: 4px 6px;
  margin: 0;
}
</style>
