<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import MailBulkPanel from '@/components/MailBulkPanel.vue'
import SoftwareInstallLogDialog from '@/components/SoftwareInstallLogDialog.vue'

const { t } = useI18n()

interface ServiceStatus {
  key: string
  name: string
  installed: boolean
  running: boolean
  enabled: boolean
}

interface PortStatus {
  port: number
  label: string
  open: boolean
  proto: string
}

interface StackStatus {
  installed: boolean
  ready: boolean
  platform_note?: string
  services: ServiceStatus[]
  ports: PortStatus[]
  domain_count: number
  mailbox_count: number
  vmail_base: string
  hostname: string
  server_ip: string
  config_synced: boolean
  last_sync_error?: string
}

interface DNSHint {
  type: string
  name: string
  value: string
  priority?: number
  purpose: string
  required: boolean
}

const status = ref<StackStatus | null>(null)
const domains = ref<any[]>([])
const mailboxes = ref<any[]>([])
const tab = ref('overview')
const loading = ref(false)
const actionLoading = ref('')

const domainDialog = ref(false)
const mailboxDialog = ref(false)
const testDialog = ref(false)
const sslDialog = ref(false)
const dnsDialog = ref(false)
const passwordDialog = ref(false)

const domainForm = ref({ domain: '' })
const mailboxForm = ref({ domain: '', address: '', password: '', quota: 1024 })
const testForm = ref({ from: '', to: '', subject: '', body: '' })
const sslForm = ref({ domain: '' })
const dnsHints = ref<DNSHint[]>([])
const dnsDomain = ref('')
const passwordForm = ref({ id: 0, password: '' })

const batchDialog = ref(false)
const batchForm = ref({
  domain: '',
  lines: '',
  default_password: '',
  generate_password: true,
  quota: 1024,
})
const batchResult = ref<any>(null)
const batchResultDialog = ref(false)

const exportDialog = ref(false)
const exportForm = ref({ domain: '', format: 'csv' as 'csv' | 'json' })

const importDialog = ref(false)
const importForm = ref({ skip_existing: false, update_password: true })
const importFileRef = ref<HTMLInputElement | null>(null)

const backups = ref<any[]>([])
const backupDialog = ref(false)
const backupForm = ref({ domain: '', include_maildir: true })
const importBackupFileRef = ref<HTMLInputElement | null>(null)

interface WebmailStatus {
  installed: boolean
  running: boolean
  url: string
  admin_url: string
  host: string
  port: number
  mail_domain: string
  install_path: string
  admin_password_file: string
  hint: string
  setup_error?: string
}

const webmail = ref<WebmailStatus | null>(null)
const webmailForm = ref({
  mail_domain: '',
  host_prefix: 'webmail',
  port: 889,
  use_port_mode: false,
})
const webmailInstallLogOpen = ref(false)
const webmailInstallTrigger = ref(false)

async function loadWebmail() {
  const res: any = await api.get('/mail/webmail')
  webmail.value = res.data
  if (res.data?.mail_domain) {
    webmailForm.value.mail_domain = res.data.mail_domain
  } else if (domainOptions.value.length) {
    webmailForm.value.mail_domain = domainOptions.value[0]
  }
}

function openWebmailInstallLog(triggerInstall = false) {
  webmailInstallTrigger.value = triggerInstall
  webmailInstallLogOpen.value = true
}

async function onWebmailInstallDone(payload: { success: boolean }) {
  if (payload.success) {
    await loadWebmail()
  }
}

async function installWebmail() {
  openWebmailInstallLog(true)
}

async function uninstallWebmail() {
  await ElMessageBox.confirm(t('mail.webmailUninstallConfirm'), t('common.warning'), { type: 'warning' })
  await withAction('webmail-uninstall', async () => {
    await api.post('/mail/webmail/uninstall')
    ElMessage.success(t('mail.webmailUninstalled'))
    await loadWebmail()
  })
}

async function repairWebmail() {
  await withAction('webmail-repair', async () => {
    const res: any = await api.post('/mail/webmail/repair')
    webmail.value = res.data
    ElMessage.success(t('mail.webmailRepaired'))
  })
}

function openWebmail() {
  if (webmail.value?.url) window.open(webmail.value.url, '_blank')
}

function openWebmailAdmin() {
  if (webmail.value?.admin_url) window.open(webmail.value.admin_url, '_blank')
}

const domainOptions = computed(() => domains.value.map((d) => d.domain))

async function loadStatus() {
  const res: any = await api.get('/mail/status')
  status.value = res.data
}

async function loadDomains() {
  const res: any = await api.get('/mail/domains')
  domains.value = res.data || []
}

async function loadMailboxes() {
  const res: any = await api.get('/mail/mailboxes')
  mailboxes.value = res.data || []
}

async function loadBackups() {
  const res: any = await api.get('/mail/backups')
  backups.value = res.data || []
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadStatus(), loadDomains(), loadMailboxes(), loadWebmail(), loadBackups()])
  } finally {
    loading.value = false
  }
}

async function withAction(key: string, fn: () => Promise<void>) {
  actionLoading.value = key
  try {
    await fn()
    await loadAll()
  } finally {
    actionLoading.value = ''
  }
}

async function installStack() {
  await ElMessageBox.confirm(t('mail.installConfirm'), t('common.warning'), { type: 'warning' })
  await withAction('install', async () => {
    await api.post('/mail/install')
    ElMessage.success(t('mail.installed'))
  })
}

async function uninstallStack() {
  await ElMessageBox.confirm(t('mail.uninstallConfirm'), t('common.warning'), { type: 'warning' })
  await withAction('uninstall', async () => {
    await api.post('/mail/uninstall')
    ElMessage.success(t('mail.uninstalled'))
  })
}

async function restartServices() {
  await withAction('restart', async () => {
    await api.post('/mail/restart')
    ElMessage.success(t('mail.restarted'))
  })
}

async function syncMail() {
  await withAction('sync', async () => {
    await api.post('/mail/sync')
    ElMessage.success(t('mail.synced'))
  })
}

async function createDomain() {
  await api.post('/mail/domains', domainForm.value)
  ElMessage.success(t('mail.createdDomain'))
  domainDialog.value = false
  domainForm.value = { domain: '' }
  await loadAll()
}

async function createMailbox() {
  await api.post('/mail/mailboxes', mailboxForm.value)
  ElMessage.success(t('mail.createdMailbox'))
  mailboxDialog.value = false
  mailboxForm.value = { domain: '', address: '', password: '', quota: 1024 }
  await loadAll()
}

async function deleteDomain(id: number) {
  await ElMessageBox.confirm(t('mail.deleteDomainConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/mail/domains/${id}`)
  await loadAll()
}

async function deleteMailbox(id: number) {
  await ElMessageBox.confirm(t('mail.deleteMailboxConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/mail/mailboxes/${id}`)
  await loadAll()
}

function openMailboxDialog() {
  mailboxForm.value.domain = domainOptions.value[0] || ''
  mailboxDialog.value = true
}

function openPasswordDialog(row: any) {
  passwordForm.value = { id: row.id, password: '' }
  passwordDialog.value = true
}

async function updatePassword() {
  await api.patch(`/mail/mailboxes/${passwordForm.value.id}`, { password: passwordForm.value.password })
  ElMessage.success(t('mail.passwordUpdated'))
  passwordDialog.value = false
  await loadAll()
}

function openBatchDialog() {
  batchForm.value.domain = domainOptions.value[0] || ''
  batchDialog.value = true
}

async function submitBatchCreate() {
  try {
    const res: any = await api.post('/mail/mailboxes/batch', batchForm.value)
    batchResult.value = res.data
    batchDialog.value = false
    batchResultDialog.value = true
    ElMessage.success(t('mail.batchCreated', { created: res.data?.created || 0, failed: res.data?.failed || 0 }))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function downloadBatchResult() {
  if (!batchResult.value?.items?.length) return
  const rows = batchResult.value.items.filter((i: any) => i.password && !i.error)
  const csv = ['address,password', ...rows.map((i: any) => `${i.address},${i.password}`)].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'mailboxes-batch.csv'
  a.click()
  URL.revokeObjectURL(a.href)
}

async function exportMailboxes() {
  try {
    const token = localStorage.getItem('token') || ''
    const params = new URLSearchParams({ format: exportForm.value.format })
    if (exportForm.value.domain) params.set('domain', exportForm.value.domain)
    const w = window as Window & { __OWPANEL_BASE__?: string }
    const base = (w.__OWPANEL_BASE__ || '/').replace(/\/?$/, '/')
    const res = await fetch(`${base}api/v1/mail/mailboxes/export?${params}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok) throw new Error(await res.text())
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = exportForm.value.format === 'json' ? 'mailboxes.json' : 'mailboxes.csv'
    a.click()
    URL.revokeObjectURL(a.href)
    exportDialog.value = false
    ElMessage.success(t('mail.exportDone'))
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.failed'))
  }
}

async function importMailboxesFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  fd.append('skip_existing', String(importForm.value.skip_existing))
  fd.append('update_password', String(importForm.value.update_password))
  fd.append('format', file.name.endsWith('.json') ? 'json' : 'csv')
  try {
    const res: any = await api.post('/mail/mailboxes/import-file', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    importDialog.value = false
    ElMessage.success(t('mail.importDone', {
      created: res.data?.created || 0,
      updated: res.data?.updated || 0,
      skipped: res.data?.skipped || 0,
      failed: res.data?.failed || 0,
    }))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    input.value = ''
  }
}

async function createBackup() {
  try {
    await api.post('/mail/backups', backupForm.value)
    ElMessage.success(t('mail.backupCreated'))
    backupDialog.value = false
    await loadBackups()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function downloadBackup(row: any) {
  const token = localStorage.getItem('token') || ''
  const w = window as Window & { __OWPANEL_BASE__?: string }
  const base = (w.__OWPANEL_BASE__ || '/').replace(/\/?$/, '/')
  const res = await fetch(`${base}api/v1/mail/backups/${row.id}/download`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    ElMessage.error(t('common.failed'))
    return
  }
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `mail-backup-${row.id}.tar.gz`
  a.click()
  URL.revokeObjectURL(a.href)
}

async function restoreBackup(row: any) {
  await ElMessageBox.confirm(t('mail.restoreBackupConfirm'), t('common.warning'), { type: 'warning' })
  try {
    await api.post(`/mail/backups/${row.id}/restore`)
    ElMessage.success(t('mail.backupRestored'))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deleteBackup(row: any) {
  await ElMessageBox.confirm(t('mail.deleteBackupConfirm'), t('common.warning'), { type: 'warning' })
  await api.delete(`/mail/backups/${row.id}`)
  ElMessage.success(t('mail.backupDeleted'))
  await loadBackups()
}

async function importBackupFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  fd.append('restore_maildir', 'true')
  try {
    await api.post('/mail/backups/import', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    ElMessage.success(t('mail.backupImported'))
    await loadAll()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    input.value = ''
  }
}

function formatSize(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

async function showDNS(domain: string) {
  dnsDomain.value = domain
  const res: any = await api.get(`/mail/dns/${encodeURIComponent(domain)}`)
  dnsHints.value = res.data || []
  dnsDialog.value = true
}

async function sendTest() {
  try {
    await api.post('/mail/test', testForm.value)
    ElMessage.success(t('mail.testSent'))
    testDialog.value = false
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('mail.testFailed')))
  }
}

async function applySSL() {
  try {
    await api.post('/mail/ssl', sslForm.value)
    ElMessage.success(t('mail.sslApplied'))
    sslDialog.value = false
    await loadStatus()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('mail.sslFailed')))
  }
}

function serviceTagType(s: ServiceStatus) {
  if (!s.installed) return 'info'
  return s.running ? 'success' : 'warning'
}

function portTagType(p: PortStatus) {
  return p.open ? 'success' : 'info'
}

function dnsPurposeLabel(purpose: string) {
  const key = `mail.dnsPurpose.${purpose}` as const
  const text = t(key)
  return text === key ? purpose : text
}

onMounted(loadAll)
</script>

<template>
  <div v-loading="loading">
    <div class="page-header">
      <h2>{{ t('mail.title') }}</h2>
      <div class="header-actions">
        <el-button :loading="actionLoading === 'sync'" @click="syncMail">{{ t('mail.sync') }}</el-button>
        <el-button :loading="actionLoading === 'restart'" @click="restartServices">{{ t('mail.restart') }}</el-button>
        <el-button @click="testDialog = true">{{ t('mail.testSend') }}</el-button>
        <el-button v-if="status?.installed" type="danger" plain :loading="actionLoading === 'uninstall'" @click="uninstallStack">
          {{ t('mail.uninstall') }}
        </el-button>
        <el-button v-else type="primary" :loading="actionLoading === 'install'" @click="installStack">
          {{ t('mail.install') }}
        </el-button>
      </div>
    </div>

    <el-alert v-if="status?.platform_note" type="warning" :closable="false" show-icon class="hint">
      {{ status.platform_note }}
    </el-alert>
    <el-alert v-else type="info" :closable="false" show-icon class="hint">{{ t('mail.hint') }}</el-alert>

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('mail.overview')" name="overview">
        <el-card v-if="status" shadow="never" class="status-card">
          <div class="status-grid">
            <div>
              <span class="label">{{ t('mail.stackStatus') }}</span>
              <el-tag :type="status.ready ? 'success' : status.installed ? 'warning' : 'info'" size="small">
                {{ status.ready ? t('mail.ready') : status.installed ? t('mail.partial') : t('mail.notInstalled') }}
              </el-tag>
            </div>
            <div><span class="label">{{ t('mail.serverIp') }}</span>{{ status.server_ip || '—' }}</div>
            <div><span class="label">{{ t('mail.hostname') }}</span>{{ status.hostname || '—' }}</div>
            <div><span class="label">{{ t('mail.vmailBase') }}</span><code>{{ status.vmail_base }}</code></div>
            <div>
              <span class="label">{{ t('mail.configSync') }}</span>
              <el-tag :type="status.config_synced ? 'success' : 'warning'" size="small">
                {{ status.config_synced ? t('common.yes') : t('common.no') }}
              </el-tag>
            </div>
          </div>
          <p v-if="status.last_sync_error" class="sync-error">{{ status.last_sync_error }}</p>
        </el-card>

        <el-row :gutter="16" class="section-row">
          <el-col :xs="24" :md="12">
            <el-card shadow="never">
              <template #header>{{ t('mail.services') }}</template>
              <el-table :data="status?.services || []" size="small">
                <el-table-column prop="name" :label="t('mail.serviceName')" />
                <el-table-column :label="t('common.status')" width="120">
                  <template #default="{ row }">
                    <el-tag :type="serviceTagType(row)" size="small">
                      {{ row.running ? t('mail.running') : row.installed ? t('mail.stopped') : t('mail.missing') }}
                    </el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-card shadow="never">
              <template #header>{{ t('mail.ports') }}</template>
              <el-table :data="status?.ports || []" size="small">
                <el-table-column prop="port" label="Port" width="80" />
                <el-table-column prop="label" :label="t('mail.portLabel')" />
                <el-table-column :label="t('mail.listening')" width="100">
                  <template #default="{ row }">
                    <el-tag :type="portTagType(row)" size="small">{{ row.open ? t('common.yes') : t('common.no') }}</el-tag>
                  </template>
                </el-table-column>
              </el-table>
              <p class="port-hint">{{ t('mail.portHint') }}</p>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>

      <el-tab-pane :label="t('mail.domains')" name="domains">
        <div class="tab-toolbar">
          <el-button type="primary" @click="domainDialog = true">{{ t('mail.addDomain') }}</el-button>
          <el-button @click="sslDialog = true">{{ t('mail.applySsl') }}</el-button>
        </div>
        <el-table :data="domains" stripe>
          <el-table-column prop="domain" :label="t('mail.domain')" />
          <el-table-column prop="mailboxes" :label="t('mail.mailboxCount')" width="100" />
          <el-table-column prop="status" :label="t('common.status')" width="100">
            <template #default="{ row }"><el-tag type="success" size="small">{{ row.status }}</el-tag></template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="180">
            <template #default="{ row }">
              <el-button text type="primary" @click="showDNS(row.domain)">{{ t('mail.dnsRecords') }}</el-button>
              <el-button text type="danger" @click="deleteDomain(row.id)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('mail.mailboxes')" name="mailboxes">
        <div class="tab-toolbar mailbox-toolbar">
          <el-button type="primary" @click="openMailboxDialog">{{ t('mail.addMailbox') }}</el-button>
          <el-button @click="openBatchDialog">{{ t('mail.batchCreate') }}</el-button>
          <el-button @click="exportDialog = true; exportForm.domain = domainOptions[0] || ''">{{ t('mail.exportMailboxes') }}</el-button>
          <el-button @click="importDialog = true">{{ t('mail.importMailboxes') }}</el-button>
          <el-button @click="backupDialog = true; backupForm.domain = domainOptions[0] || ''">{{ t('mail.createBackup') }}</el-button>
          <el-button @click="importBackupFileRef?.click()">{{ t('mail.importBackup') }}</el-button>
          <input ref="importBackupFileRef" type="file" accept=".tar.gz,.tgz,.gz" hidden @change="importBackupFile" />
        </div>
        <el-table :data="mailboxes" stripe>
          <el-table-column prop="address" :label="t('mail.address')" />
          <el-table-column prop="domain" :label="t('mail.domain')" />
          <el-table-column prop="quota" :label="t('mail.quota')" width="100" />
          <el-table-column :label="t('mail.syncedCol')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.synced ? 'success' : 'warning'" size="small">{{ row.synced ? t('common.yes') : t('common.no') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="160">
            <template #default="{ row }">
              <el-button text type="primary" @click="openPasswordDialog(row)">{{ t('mail.changePassword') }}</el-button>
              <el-button text type="danger" @click="deleteMailbox(row.id)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-card shadow="never" class="backup-card">
          <template #header>{{ t('mail.backupList') }}</template>
          <el-table :data="backups" stripe size="small">
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="domain" :label="t('mail.domain')" />
            <el-table-column :label="t('mail.mailboxCount')" width="90">
              <template #default="{ row }">{{ row.mailbox_count }}</template>
            </el-table-column>
            <el-table-column :label="t('common.size')" width="100">
              <template #default="{ row }">{{ formatSize(row.size) }}</template>
            </el-table-column>
            <el-table-column prop="created_at" :label="t('common.createdAt')" width="170" />
            <el-table-column :label="t('common.actions')" width="220">
              <template #default="{ row }">
                <el-button text type="primary" @click="downloadBackup(row)">{{ t('common.download') }}</el-button>
                <el-button text type="warning" @click="restoreBackup(row)">{{ t('common.restore') }}</el-button>
                <el-button text type="danger" @click="deleteBackup(row)">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="t('mail.bulkSend')" name="bulk">
        <MailBulkPanel />
      </el-tab-pane>

      <el-tab-pane :label="t('mail.webmail')" name="webmail">
        <el-card v-if="webmail" shadow="never" class="status-card">
          <div class="status-grid">
            <div>
              <span class="label">{{ t('common.status') }}</span>
              <el-tag :type="webmail.installed ? (webmail.running ? 'success' : 'warning') : 'info'" size="small">
                {{ webmail.installed ? (webmail.running ? t('mail.running') : t('mail.stopped')) : t('mail.notInstalled') }}
              </el-tag>
            </div>
            <div v-if="webmail.host"><span class="label">{{ t('mail.webmailHost') }}</span>{{ webmail.host }}</div>
            <div v-else-if="webmail.port"><span class="label">{{ t('mail.webmailPort') }}</span>{{ webmail.port }}</div>
            <div v-if="webmail.url"><span class="label">URL</span><a :href="webmail.url" target="_blank" rel="noopener">{{ webmail.url }}</a></div>
          </div>
          <p class="hint-text">{{ webmail.hint }}</p>
          <p v-if="webmail.setup_error" class="sync-error">{{ webmail.setup_error }}</p>
          <div class="webmail-actions">
            <el-button v-if="webmail.installed && webmail.url" type="primary" @click="openWebmail">{{ t('mail.openWebmail') }}</el-button>
            <el-button v-if="webmail.installed && webmail.admin_url" @click="openWebmailAdmin">{{ t('mail.openWebmailAdmin') }}</el-button>
            <el-button v-if="webmail.installed" :loading="actionLoading === 'webmail-repair'" @click="repairWebmail">{{ t('mail.webmailRepair') }}</el-button>
            <el-button v-if="webmail.installed" type="danger" plain :loading="actionLoading === 'webmail-uninstall'" @click="uninstallWebmail">{{ t('mail.webmailUninstall') }}</el-button>
          </div>
        </el-card>

        <el-card v-if="!webmail?.installed" shadow="never" class="install-card">
          <template #header>{{ t('mail.webmailInstall') }}</template>
          <el-form label-width="120px">
            <el-form-item :label="t('mail.domain')">
              <el-select v-model="webmailForm.mail_domain" filterable allow-create style="width:100%" :placeholder="t('mail.webmailDomainHint')">
                <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('mail.webmailHostPrefix')">
              <el-input v-model="webmailForm.host_prefix" placeholder="webmail" :disabled="webmailForm.use_port_mode" />
              <p class="form-hint">{{ t('mail.webmailHostPrefixHint') }}</p>
            </el-form-item>
            <el-form-item :label="t('mail.webmailPortMode')">
              <el-switch v-model="webmailForm.use_port_mode" />
              <p class="form-hint">{{ t('mail.webmailPortModeHint') }}</p>
            </el-form-item>
            <el-form-item v-if="webmailForm.use_port_mode" :label="t('mail.webmailPort')">
              <el-input-number v-model="webmailForm.port" :min="1024" :max="65535" />
            </el-form-item>
          </el-form>
          <el-button type="primary" @click="installWebmail">{{ t('mail.webmailInstall') }}</el-button>
          <el-button plain @click="openWebmailInstallLog(false)">{{ t('mail.webmailViewInstallLog') }}</el-button>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="domainDialog" :title="t('mail.addDomain')" width="420px">
      <el-input v-model="domainForm.domain" :placeholder="t('mail.domainPlaceholder')" />
      <template #footer>
        <el-button @click="domainDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="createDomain">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="mailboxDialog" :title="t('mail.addMailbox')" width="480px">
      <el-form label-width="90px">
        <el-form-item :label="t('mail.domain')">
          <el-select v-model="mailboxForm.domain" filterable allow-create style="width:100%">
            <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('mail.address')">
          <el-input v-model="mailboxForm.address" :placeholder="t('mail.addressPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('common.password')">
          <el-input v-model="mailboxForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('mail.quota')">
          <el-input-number v-model="mailboxForm.quota" :min="64" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mailboxDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="createMailbox">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="passwordDialog" :title="t('mail.changePassword')" width="420px">
      <el-input v-model="passwordForm.password" type="password" show-password />
      <template #footer>
        <el-button @click="passwordDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="updatePassword">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchDialog" :title="t('mail.batchCreateTitle')" width="560px">
      <el-form label-width="100px">
        <el-form-item :label="t('mail.domain')">
          <el-select v-model="batchForm.domain" filterable allow-create style="width:100%">
            <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('mail.address')">
          <el-input v-model="batchForm.lines" type="textarea" :rows="8" :placeholder="t('mail.batchLinesHint')" />
        </el-form-item>
        <el-form-item :label="t('mail.batchGeneratePassword')">
          <el-switch v-model="batchForm.generate_password" />
        </el-form-item>
        <el-form-item v-if="!batchForm.generate_password" :label="t('mail.batchDefaultPassword')">
          <el-input v-model="batchForm.default_password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('mail.quota')">
          <el-input-number v-model="batchForm.quota" :min="64" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="submitBatchCreate">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchResultDialog" :title="t('mail.batchResultTitle')" width="640px">
      <el-table :data="batchResult?.items || []" stripe max-height="360" size="small">
        <el-table-column prop="address" :label="t('mail.address')" />
        <el-table-column prop="password" :label="t('common.password')" />
        <el-table-column prop="error" :label="t('common.error')" />
      </el-table>
      <template #footer>
        <el-button @click="downloadBatchResult">{{ t('mail.downloadResult') }}</el-button>
        <el-button type="primary" @click="batchResultDialog = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="exportDialog" :title="t('mail.exportMailboxes')" width="420px">
      <el-form label-width="90px">
        <el-form-item :label="t('mail.domain')">
          <el-select v-model="exportForm.domain" clearable filterable style="width:100%" :placeholder="t('common.all')">
            <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('mail.exportFormat')">
          <el-radio-group v-model="exportForm.format">
            <el-radio value="csv">CSV</el-radio>
            <el-radio value="json">JSON</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="exportDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="exportMailboxes">{{ t('mail.exportMailboxes') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialog" :title="t('mail.importMailboxes')" width="480px">
      <el-form label-width="120px">
        <el-form-item :label="t('mail.importSkipExisting')">
          <el-switch v-model="importForm.skip_existing" />
        </el-form-item>
        <el-form-item :label="t('mail.importUpdatePassword')">
          <el-switch v-model="importForm.update_password" />
        </el-form-item>
        <el-form-item :label="t('mail.importFile')">
          <input ref="importFileRef" type="file" accept=".csv,.json" @change="importMailboxesFile" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importDialog = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="backupDialog" :title="t('mail.createBackup')" width="420px">
      <el-form label-width="110px">
        <el-form-item :label="t('mail.domain')">
          <el-select v-model="backupForm.domain" clearable filterable style="width:100%" :placeholder="t('common.all')">
            <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('mail.includeMaildir')">
          <el-switch v-model="backupForm.include_maildir" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="backupDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="createBackup">{{ t('mail.createBackup') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="testDialog" :title="t('mail.testSend')" width="520px">
      <el-form label-width="80px">
        <el-form-item :label="t('mail.from')"><el-input v-model="testForm.from" /></el-form-item>
        <el-form-item :label="t('mail.to')"><el-input v-model="testForm.to" /></el-form-item>
        <el-form-item :label="t('mail.subject')"><el-input v-model="testForm.subject" /></el-form-item>
        <el-form-item :label="t('mail.body')"><el-input v-model="testForm.body" type="textarea" :rows="4" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="testDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="sendTest">{{ t('mail.send') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="sslDialog" :title="t('mail.applySsl')" width="420px">
      <p class="ssl-hint">{{ t('mail.sslHint') }}</p>
      <el-select v-model="sslForm.domain" filterable style="width:100%">
        <el-option v-for="d in domainOptions" :key="d" :label="d" :value="d" />
      </el-select>
      <template #footer>
        <el-button @click="sslDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="applySSL">{{ t('mail.applySsl') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dnsDialog" :title="t('mail.dnsRecords') + ' — ' + dnsDomain" width="720px">
      <p class="dns-hint">{{ t('mail.dnsHint') }}</p>
      <el-table :data="dnsHints" stripe size="small">
        <el-table-column prop="type" label="Type" width="70" />
        <el-table-column prop="name" :label="t('mail.dnsName')" />
        <el-table-column prop="value" :label="t('mail.dnsValue')" />
        <el-table-column prop="priority" :label="t('mail.priority')" width="80" />
        <el-table-column :label="t('mail.dnsPurposeCol')" width="120">
          <template #default="{ row }">{{ dnsPurposeLabel(row.purpose) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <SoftwareInstallLogDialog
      v-model="webmailInstallLogOpen"
      app-key="snappymail"
      :app-name="t('mail.webmailInstallLogTitle')"
      :trigger-install="webmailInstallTrigger"
      install-api-path="/mail/webmail/install"
      logs-api-path="/mail/webmail/install/logs"
      :install-payload="webmailForm"
      @done="onWebmailInstallDone"
    />
  </div>
</template>

<style scoped>
.hint { margin-bottom: 16px; }
.status-card { margin-bottom: 16px; }
.status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px 24px;
}
.label { color: var(--el-text-color-secondary); margin-right: 8px; }
.sync-error { margin-top: 12px; color: var(--el-color-danger); font-size: 13px; }
.section-row { margin-top: 0; }
.tab-toolbar { margin-bottom: 12px; }
.mailbox-toolbar { display: flex; flex-wrap: wrap; gap: 8px; }
.backup-card { margin-top: 16px; }
.port-hint, .ssl-hint, .dns-hint, .hint-text, .form-hint { margin-top: 12px; color: var(--el-text-color-secondary); font-size: 13px; }
.form-hint { margin-top: 4px; margin-bottom: 0; }
.webmail-actions { margin-top: 16px; display: flex; flex-wrap: wrap; gap: 8px; }
.install-card { margin-top: 16px; }
code { font-size: 12px; word-break: break-all; }
</style>
