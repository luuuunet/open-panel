<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api, { downloadAuthenticated, resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { Plus, Refresh, Download, VideoPlay, Connection, Document, Edit, Delete } from '@element-plus/icons-vue'

const emit = defineEmits<{
  connect: [query: Record<string, string>]
  'assets-changed': []
}>()

const { t } = useI18n()
const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')

const tab = ref('assets')
const loading = ref(false)

const complianceScore = ref<any>(null)
const accessRequests = ref<any[]>([])
const accessDialog = ref(false)
const accessForm = ref({ asset_id: 0, account_id: null as number | null, reason: '', duration_hours: 4 })
const knownHosts = ref<any[]>([])
const exportLoading = ref(false)

const assets = ref<any[]>([])
const groups = ref<any[]>([])
const permissions = ref<any[]>([])
const users = ref<any[]>([])
const sessions = ref<any[]>([])
const activeSessions = ref<any[]>([])
const commandAudits = ref<any[]>([])
const sessionSearch = ref('')

const assetDialog = ref(false)
const editingAsset = ref<any>(null)
const assetForm = ref({
  name: '', host: '', port: 22, protocol: 'ssh', username: 'root',
  auth_method: 'password', password: '', key_id: null as number | null,
  group_id: null as number | null, tags: '', remark: '',
})

const groupDialog = ref(false)
const groupForm = ref({ name: '', remark: '' })

const permDialog = ref(false)
const permForm = ref({ user_id: 0, asset_id: 0, permission: 'connect' })

const policyDialog = ref(false)
const policyForm = ref({ mode: 'block', blocklist: [] as string[], custom_rules: [] as string[] })
const policyBlocklistText = ref('')

const replayDialog = ref(false)
const replayLog = ref('')
const replayCommands = ref<string[]>([])
const replayTitle = ref('')

const sshKeys = ref<any[]>([])
const clusterNodes = ref<any[]>([])

// Ops center
const opsSubTab = ref('adhoc')
const templates = ref<any[]>([])
const jobs = ref<any[]>([])
const jobRuns = ref<any[]>([])
const adhocHistory = ref<any[]>([])
const selectedAssetIds = ref<number[]>([])
const adhocForm = ref({ command: 'hostname', language: 'shell', timeout_sec: 30, cwd: '' })
const adhocRunning = ref(false)
const adhocResults = ref<any[]>([])
const adhocRunId = ref<number | null>(null)
const expandedResult = ref<number[]>([])

const templateDialog = ref(false)
const editingTemplate = ref<any>(null)
const templateForm = ref({ name: '', type: 'command', language: 'shell', content: '', remark: '' })

const jobDialog = ref(false)
const editingJob = ref<any>(null)
const jobForm = ref({
  name: '', template_id: 0, asset_ids: [] as number[], schedule: '',
  timeout_sec: 30, cwd: '', enabled: true,
})

const runDetailDialog = ref(false)
const runDetail = ref<any>(null)
const runDetailLoading = ref(false)
const jobHistoryDialog = ref(false)
const jobHistoryJob = ref<any>(null)

// Account management
const accounts = ref<any[]>([])
const rotationLogs = ref<any[]>([])
const accountFilterAssetId = ref<number | null>(null)
const selectedAccountIds = ref<number[]>([])
const accountSubTab = ref('list')
const accountDialog = ref(false)
const editingAccount = ref<any>(null)
const accountForm = ref({
  asset_id: 0, username: '', auth_method: 'password', password: '',
  key_id: null as number | null, is_privileged: false, status: 'active',
  auto_rotate: false, rotate_after_session: false, rotate_days: 90, remark: '',
})
const vaultImportInput = ref<HTMLInputElement | null>(null)

function accountExpiring(row: any) {
  if (!row.expires_at) return false
  const exp = new Date(row.expires_at).getTime()
  const warn = Date.now() + 7 * 86400000
  return exp <= warn
}

function sourceTag(s: string) {
  if (s === 'discovered') return 'info'
  if (s === 'pushed') return 'success'
  return ''
}

async function loadComplianceScore() {
  if (!isAdmin.value) return
  try {
    complianceScore.value = ((await api.get('/bastion/compliance/score')) as any).data
  } catch { /* ignore */ }
}

async function exportCompliance() {
  exportLoading.value = true
  try {
    const to = new Date()
    const from = new Date(to.getTime() - 30 * 86400000)
    const res: any = await api.post('/bastion/compliance/export', {
      from: from.toISOString(), to: to.toISOString(),
    })
    const fn = res.data?.filename
    if (fn) {
      await downloadAuthenticated(`/bastion/compliance/download/${fn}`, fn)
    }
    ElMessage.success(t('bastionPage.complianceExportOk'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    exportLoading.value = false
  }
}

async function loadAccessRequests() {
  try {
    accessRequests.value = ((await api.get('/bastion/access-requests')) as any).data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function submitAccessRequest() {
  try {
    await api.post('/bastion/access-requests', accessForm.value)
    ElMessage.success(t('common.success'))
    accessDialog.value = false
    accessForm.value = { asset_id: 0, account_id: null, reason: '', duration_hours: 4 }
    loadAccessRequests()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function approveAccess(row: any) {
  try {
    await api.post(`/bastion/access-requests/${row.id}/approve`)
    ElMessage.success(t('common.success'))
    loadAccessRequests()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function rejectAccess(row: any) {
  try {
    await api.post(`/bastion/access-requests/${row.id}/reject`)
    loadAccessRequests()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function loadKnownHosts() {
  if (!isAdmin.value) return
  try {
    knownHosts.value = ((await api.get('/bastion/known-hosts')) as any).data || []
  } catch { /* ignore */ }
}

async function captureHostKey(assetId: number) {
  try {
    await api.post(`/bastion/known-hosts/capture/${assetId}`)
    ElMessage.success(t('bastionPage.knownHostCaptured'))
    loadKnownHosts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function acceptHostKey(assetId: number) {
  try {
    await api.post(`/bastion/known-hosts/${assetId}/accept`)
    ElMessage.success(t('common.success'))
    loadKnownHosts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function accessStatusTag(s: string) {
  if (s === 'approved') return 'success'
  if (s === 'pending') return 'warning'
  if (s === 'rejected') return 'danger'
  return 'info'
}

async function loadAccounts() {
  loading.value = true
  try {
    const params = accountFilterAssetId.value ? { asset_id: accountFilterAssetId.value } : {}
    accounts.value = ((await api.get('/bastion/accounts', { params })) as any).data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function loadRotationLogs() {
  try {
    rotationLogs.value = ((await api.get('/bastion/accounts/rotation-logs')) as any).data || []
  } catch { /* ignore */ }
}

function openAccount(row?: any, assetId?: number) {
  editingAccount.value = row || null
  if (row) {
    accountForm.value = {
      asset_id: row.asset_id, username: row.username,
      auth_method: row.auth_method || 'password', password: '',
      key_id: row.key_id || null, is_privileged: !!row.is_privileged,
      status: row.status || 'active', auto_rotate: !!row.auto_rotate,
      rotate_after_session: !!row.rotate_after_session,
      rotate_days: row.rotate_days || 90, remark: row.remark || '',
    }
  } else {
    accountForm.value = {
      asset_id: assetId || assets.value[0]?.id || 0, username: '',
      auth_method: 'password', password: '', key_id: null,
      is_privileged: false, status: 'active', auto_rotate: false,
      rotate_after_session: false, rotate_days: 90, remark: '',
    }
  }
  accountDialog.value = true
}

async function saveAccount() {
  try {
    if (editingAccount.value) {
      await api.put(`/bastion/accounts/${editingAccount.value.id}`, accountForm.value)
    } else {
      await api.post('/bastion/accounts', accountForm.value)
    }
    ElMessage.success(t('common.success'))
    accountDialog.value = false
    loadAccounts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deleteAccount(row: any) {
  await ElMessageBox.confirm(t('common.confirmDelete'), { type: 'warning' })
  try {
    await api.delete(`/bastion/accounts/${row.id}`)
    loadAccounts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function discoverAccounts(assetId: number) {
  try {
    const res: any = await api.post(`/bastion/accounts/discover/${assetId}`)
    ElMessage.success(t('bastionPage.accountsDiscoverOk', { n: (res.data || []).length }))
    if (tab.value === 'accounts') loadAccounts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function rotateAccount(row: any) {
  await ElMessageBox.confirm(t('bastionPage.accountsRotateConfirm', { user: row.username }), { type: 'warning' })
  try {
    await api.post(`/bastion/accounts/${row.id}/rotate`)
    ElMessage.success(t('common.success'))
    loadAccounts()
    loadRotationLogs()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function rotateBatch() {
  const ids = selectedAccountIds.value.length ? selectedAccountIds.value : undefined
  await ElMessageBox.confirm(t('bastionPage.accountsBatchRotateConfirm'), { type: 'warning' })
  try {
    const res: any = await api.post('/bastion/accounts/rotate-batch', { account_ids: ids || [] })
    ElMessage.success(t('bastionPage.accountsBatchRotateOk', { ok: res.data?.success || 0, fail: res.data?.failed || 0 }))
    loadAccounts()
    loadRotationLogs()
    selectedAccountIds.value = []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function pushAccount(row: any, createUser = false) {
  try {
    await api.post(`/bastion/accounts/${row.id}/push`, { create_user: createUser })
    ElMessage.success(t('bastionPage.accountsPushOk'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function testAccount(row: any) {
  try {
    const res: any = await api.post(`/bastion/accounts/${row.id}/test`)
    if (res.data?.success) ElMessage.success(res.data.message || t('common.success'))
    else ElMessage.warning(res.data?.message || t('common.failed'))
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function connectAccount(row: any) {
  emit('connect', { asset_id: String(row.asset_id), account_id: String(row.id) })
}

async function exportVault() {
  try {
    await downloadAuthenticated('/bastion/accounts/vault/export', 'bastion-vault.json')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function triggerVaultImport() {
  vaultImportInput.value?.click()
}

async function onVaultImport(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const res: any = await api.post('/bastion/accounts/vault/import', JSON.parse(text))
    ElMessage.success(t('bastionPage.accountsImportOk', { n: res.data?.imported || 0, skip: res.data?.skipped || 0 }))
    loadAccounts()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    input.value = ''
  }
}


async function loadAccountData() {
  await Promise.all([loadAssets(), loadAccounts(), loadRotationLogs()])
}

const languages = ['shell', 'python', 'mysql', 'pgsql']
const timeoutOptions = [10, 30, 60]
const templateTypes = ['command', 'playbook']
const accountStatuses = ['active', 'disabled', 'locked']

const assetTree = computed(() => {
  const byGroup: Record<string, any[]> = {}
  for (const a of assets.value) {
    if (a.protocol && a.protocol !== 'ssh') continue
    const g = a.group_name || t('bastionPage.ungrouped')
    if (!byGroup[g]) byGroup[g] = []
    byGroup[g].push(a)
  }
  return byGroup
})

function toggleAsset(id: number, checked: boolean) {
  if (checked) {
    if (!selectedAssetIds.value.includes(id)) selectedAssetIds.value.push(id)
  } else {
    selectedAssetIds.value = selectedAssetIds.value.filter((x) => x !== id)
  }
}

function opsStatusTag(s: string) {
  if (s === 'success') return 'success'
  if (s === 'partial') return 'warning'
  if (s === 'running') return 'info'
  return 'danger'
}

async function loadTemplates() {
  try {
    templates.value = ((await api.get('/bastion/ops/templates')) as any).data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function loadJobs() {
  try {
    jobs.value = ((await api.get('/bastion/ops/jobs')) as any).data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function loadAdhocHistory() {
  try {
    adhocHistory.value = ((await api.get('/bastion/ops/adhoc/history')) as any).data || []
  } catch { /* ignore */ }
}

async function runAdhoc() {
  if (!selectedAssetIds.value.length) {
    ElMessage.warning(t('bastionPage.opsSelectAssets'))
    return
  }
  if (!adhocForm.value.command.trim()) {
    ElMessage.warning(t('bastionPage.opsEnterCommand'))
    return
  }
  adhocRunning.value = true
  adhocResults.value = []
  try {
    const res: any = await api.post('/bastion/ops/adhoc', {
      asset_ids: selectedAssetIds.value,
      command: adhocForm.value.command,
      language: adhocForm.value.language,
      timeout_sec: adhocForm.value.timeout_sec,
      cwd: adhocForm.value.cwd,
    })
    adhocRunId.value = res.data?.id
    ElMessage.success(t('bastionPage.opsRunStarted'))
    await pollRun(adhocRunId.value!)
    loadAdhocHistory()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    adhocRunning.value = false
  }
}

async function pollRun(runId: number) {
  for (let i = 0; i < 60; i++) {
    await new Promise((r) => setTimeout(r, 1000))
    const detail: any = await api.get(`/bastion/ops/runs/${runId}`)
    if (detail.data?.status !== 'running') {
      adhocResults.value = detail.data?.results || []
      return detail.data
    }
  }
}

async function openRunDetail(runId: number) {
  runDetailLoading.value = true
  runDetailDialog.value = true
  try {
    runDetail.value = ((await api.get(`/bastion/ops/runs/${runId}`)) as any).data
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    runDetailLoading.value = false
  }
}

function applyTemplate(tpl: any) {
  adhocForm.value.command = tpl.content
  adhocForm.value.language = tpl.language || 'shell'
  opsSubTab.value = 'adhoc'
}

function openTemplate(row?: any) {
  editingTemplate.value = row || null
  if (row) {
    templateForm.value = {
      name: row.name, type: row.type || 'command', language: row.language || 'shell',
      content: row.content, remark: row.remark || '',
    }
  } else {
    templateForm.value = { name: '', type: 'command', language: 'shell', content: adhocForm.value.command, remark: '' }
  }
  templateDialog.value = true
}

async function saveTemplate() {
  try {
    if (editingTemplate.value) {
      await api.put(`/bastion/ops/templates/${editingTemplate.value.id}`, templateForm.value)
    } else {
      await api.post('/bastion/ops/templates', templateForm.value)
    }
    ElMessage.success(t('common.success'))
    templateDialog.value = false
    loadTemplates()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deleteTemplate(row: any) {
  if (row.builtin) return
  await ElMessageBox.confirm(t('common.confirmDelete'), { type: 'warning' })
  try {
    await api.delete(`/bastion/ops/templates/${row.id}`)
    loadTemplates()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function openJob(row?: any) {
  editingJob.value = row || null
  if (row) {
    let ids: number[] = []
    try { ids = JSON.parse(row.asset_ids || '[]') } catch { /* ignore */ }
    jobForm.value = {
      name: row.name, template_id: row.template_id, asset_ids: ids,
      schedule: row.schedule || '', timeout_sec: row.timeout_sec || 30,
      cwd: row.cwd || '', enabled: row.enabled !== false,
    }
  } else {
    jobForm.value = {
      name: '', template_id: templates.value[0]?.id || 0, asset_ids: [...selectedAssetIds.value],
      schedule: '', timeout_sec: 30, cwd: '', enabled: true,
    }
  }
  jobDialog.value = true
}

async function saveJob() {
  try {
    if (editingJob.value) {
      await api.put(`/bastion/ops/jobs/${editingJob.value.id}`, jobForm.value)
    } else {
      await api.post('/bastion/ops/jobs', jobForm.value)
    }
    ElMessage.success(t('common.success'))
    jobDialog.value = false
    loadJobs()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deleteJob(row: any) {
  await ElMessageBox.confirm(t('common.confirmDelete'), { type: 'warning' })
  try {
    await api.delete(`/bastion/ops/jobs/${row.id}`)
    loadJobs()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function runJob(row: any) {
  try {
    const res: any = await api.post(`/bastion/ops/jobs/${row.id}/run`)
    ElMessage.success(t('bastionPage.opsRunStarted'))
    await openRunDetail(res.data?.id)
    loadJobs()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function showJobHistory(row: any) {
  jobHistoryJob.value = row
  await loadJobRuns(row.id)
  jobHistoryDialog.value = true
}

async function loadJobRuns(jobId: number) {
  jobRuns.value = ((await api.get(`/bastion/ops/jobs/${jobId}/runs`)) as any).data || []
}

async function loadOpsData() {
  await Promise.all([loadAssets(), loadTemplates(), loadJobs(), loadAdhocHistory()])
}

const protocols = ['ssh', 'mysql', 'pgsql', 'redis']
const permTypes = ['connect', 'sftp', 'readonly']

async function loadAssets() {
  loading.value = true
  try {
    const res: any = await api.get('/bastion/assets')
    assets.value = res.data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  if (!isAdmin.value) return
  try {
    groups.value = ((await api.get('/bastion/groups')) as any).data || []
  } catch { /* ignore */ }
}

async function loadPermissions() {
  if (!isAdmin.value) return
  try {
    permissions.value = ((await api.get('/bastion/permissions')) as any).data || []
  } catch { /* ignore */ }
}

async function loadUsers() {
  if (!isAdmin.value) return
  try {
    users.value = ((await api.get('/users')) as any).data || []
  } catch { /* ignore */ }
}

async function loadSessions() {
  loading.value = true
  try {
    const q = sessionSearch.value.trim()
    sessions.value = ((await api.get('/bastion/sessions', { params: q ? { q } : {} })) as any).data || []
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    loading.value = false
  }
}

async function loadActive() {
  if (!isAdmin.value) return
  try {
    activeSessions.value = ((await api.get('/bastion/active-sessions')) as any).data || []
  } catch { /* ignore */ }
}

async function loadAudits() {
  if (!isAdmin.value) return
  try {
    commandAudits.value = ((await api.get('/bastion/command-audits')) as any).data || []
  } catch { /* ignore */ }
}

async function loadAux() {
  if (!isAdmin.value) return
  try {
    sshKeys.value = ((await api.get('/terminal/keys')) as any).data || []
    clusterNodes.value = ((await api.get('/cluster/nodes')) as any).data || []
  } catch { /* ignore */ }
}

function openAsset(row?: any) {
  editingAsset.value = row || null
  if (row) {
    assetForm.value = {
      name: row.name, host: row.host, port: row.port || 22,
      protocol: row.protocol || 'ssh', username: row.username || 'root',
      auth_method: row.auth_method || 'password', password: '',
      key_id: row.key_id || null, group_id: row.group_id || null,
      tags: row.tags || '', remark: row.remark || '',
    }
  } else {
    assetForm.value = {
      name: '', host: '', port: 22, protocol: 'ssh', username: 'root',
      auth_method: 'password', password: '', key_id: null,
      group_id: null, tags: '', remark: '',
    }
  }
  assetDialog.value = true
}

async function saveAsset() {
  try {
    const payload = { ...assetForm.value }
    if (editingAsset.value) {
      await api.put(`/bastion/assets/${editingAsset.value.id}`, payload)
    } else {
      await api.post('/bastion/assets', payload)
    }
    ElMessage.success(t('common.success'))
    assetDialog.value = false
    loadAssets()
    emit('assets-changed')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deleteAsset(row: any) {
  await ElMessageBox.confirm(t('bastionPage.deleteAssetConfirm', { name: row.name }), { type: 'warning' })
  try {
    await api.delete(`/bastion/assets/${row.id}`)
    ElMessage.success(t('common.success'))
    loadAssets()
    emit('assets-changed')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function importFromCluster(node: any) {
  try {
    await api.post(`/bastion/assets/import/cluster/${node.id}`)
    ElMessage.success(t('bastionPage.importOk'))
    loadAssets()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function saveGroup() {
  try {
    await api.post('/bastion/groups', groupForm.value)
    ElMessage.success(t('common.success'))
    groupDialog.value = false
    loadGroups()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function savePermission() {
  try {
    await api.post('/bastion/permissions', permForm.value)
    ElMessage.success(t('common.success'))
    permDialog.value = false
    loadPermissions()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function deletePermission(row: any) {
  await ElMessageBox.confirm(t('common.confirmDelete'), { type: 'warning' })
  try {
    await api.delete(`/bastion/permissions/${row.id}`)
    loadPermissions()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function openPolicy() {
  try {
    const p = (await api.get('/bastion/command-policy')) as any
    policyForm.value = { mode: p.mode || 'block', blocklist: p.blocklist || [], custom_rules: p.custom_rules || [] }
    policyBlocklistText.value = [...policyForm.value.blocklist, ...policyForm.value.custom_rules].join('\n')
    policyDialog.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function savePolicy() {
  const lines = policyBlocklistText.value.split('\n').map((s) => s.trim()).filter(Boolean)
  try {
    await api.put('/bastion/command-policy', {
      mode: policyForm.value.mode,
      blocklist: lines,
      custom_rules: [],
    })
    ElMessage.success(t('common.success'))
    policyDialog.value = false
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function openReplay(row: any) {
  replayTitle.value = `${row.username} @ ${row.host} · ${row.start_time}`
  try {
    const res = (await api.get(`/bastion/sessions/${row.id}/log`)) as any
    replayLog.value = res.log || ''
    replayCommands.value = ((await api.get(`/bastion/sessions/${row.id}/commands`)) as any).data || []
    replayDialog.value = true
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function downloadSession(row: any) {
  try {
    await downloadAuthenticated(`/bastion/sessions/${row.id}/download`, `session-${row.id}.log`)
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

async function killSession(row: any) {
  await ElMessageBox.confirm(t('bastionPage.killConfirm'), { type: 'warning' })
  try {
    await api.post(`/bastion/active-sessions/${row.session_key}/kill`)
    ElMessage.success(t('common.success'))
    loadActive()
    loadSessions()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  }
}

function connectAsset(row: any) {
  emit('connect', { asset_id: String(row.id) })
}

function statusTag(s: string) {
  if (s === 'active') return 'success'
  if (s === 'killed') return 'danger'
  return 'info'
}

function formatSize(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

watch(tab, (v) => {
  if (v === 'assets') loadAssets()
  if (v === 'permissions') { loadPermissions(); loadUsers(); loadAssets(); loadAudits() }
  if (v === 'audit') loadSessions()
  if (v === 'active') loadActive()
  if (v === 'ops') loadOpsData()
  if (v === 'accounts') loadAccountData()
  if (v === 'access') loadAccessRequests()
})

onMounted(async () => {
  if (!isAdmin.value) tab.value = 'access'
  await loadAssets()
  if (isAdmin.value) {
    loadGroups()
    loadAux()
    loadComplianceScore()
    loadKnownHosts()
  }
})
</script>

<template>
  <div class="terminal-pam-panel">
    <div class="pam-toolbar">
      <div>
        <p class="page-desc">{{ t('bastionPage.desc') }}</p>
        <div v-if="isAdmin && complianceScore" class="compliance-bar">
          <el-tag type="success" size="large">{{ t('bastionPage.complianceScore') }}: {{ Math.round(complianceScore.score) }} ({{ complianceScore.grade }})</el-tag>
          <el-button size="small" :loading="exportLoading" @click="exportCompliance">{{ t('bastionPage.complianceExport') }}</el-button>
        </div>
      </div>
      <el-button v-if="tab === 'assets' && isAdmin" type="primary" :icon="Plus" @click="openAsset()">
        {{ t('bastionPage.addAsset') }}
      </el-button>
    </div>

    <el-tabs v-model="tab" class="bastion-tabs">
      <el-tab-pane v-if="isAdmin" :label="t('bastionPage.tabAssets')" name="assets">
        <div v-if="isAdmin" class="toolbar-row">
          <el-button size="small" @click="groupDialog = true">{{ t('bastionPage.addGroup') }}</el-button>
          <el-dropdown v-if="clusterNodes.length" trigger="click">
            <el-button size="small">{{ t('bastionPage.importCluster') }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="n in clusterNodes.filter(x => !x.is_local)" :key="n.id" @click="importFromCluster(n)">
                  {{ n.name }} ({{ n.host }})
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button size="small" :icon="Refresh" @click="loadAssets">{{ t('common.refresh') }}</el-button>
          <el-button size="small" @click="loadKnownHosts">{{ t('bastionPage.knownHosts') }}</el-button>
        </div>
        <el-table v-if="knownHosts.length" :data="knownHosts" size="small" stripe style="margin-bottom:16px">
          <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" />
          <el-table-column prop="fingerprint" :label="t('bastionPage.knownHostFp')" min-width="200" show-overflow-tooltip />
          <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="100" />
          <el-table-column :label="t('common.actions')" width="160">
            <template #default="{ row }">
              <el-button v-if="row.status === 'pending'" link type="primary" @click="acceptHostKey(row.asset_id)">{{ t('bastionPage.knownHostAccept') }}</el-button>
              <el-button link @click="captureHostKey(row.asset_id)">{{ t('common.refresh') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-table v-loading="loading" :data="assets" stripe>
          <el-table-column prop="name" :label="t('bastionPage.colName')" min-width="120" />
          <el-table-column prop="host" :label="t('bastionPage.colHost')" min-width="140" />
          <el-table-column prop="port" :label="t('bastionPage.colPort')" width="80" />
          <el-table-column prop="protocol" :label="t('bastionPage.colProtocol')" width="90" />
          <el-table-column prop="username" :label="t('bastionPage.colUser')" width="100" />
          <el-table-column prop="group_name" :label="t('bastionPage.colGroup')" width="100" />
          <el-table-column prop="tags" :label="t('bastionPage.colTags')" min-width="100" />
          <el-table-column :label="t('common.actions')" width="280" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Connection" @click="connectAsset(row)">{{ t('bastionPage.connect') }}</el-button>
              <template v-if="isAdmin">
                <el-button v-if="!row.protocol || row.protocol === 'ssh'" link type="primary" @click="discoverAccounts(row.id)">{{ t('bastionPage.accountsDiscover') }}</el-button>
                <el-button link type="primary" @click="openAsset(row)">{{ t('common.edit') }}</el-button>
                <el-button link type="danger" @click="deleteAsset(row)">{{ t('common.delete') }}</el-button>
              </template>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('bastionPage.tabAccess')" name="access">
        <div class="toolbar-row">
          <el-button type="primary" size="small" :icon="Plus" @click="accessDialog = true">{{ t('bastionPage.accessSubmit') }}</el-button>
          <el-button size="small" :icon="Refresh" @click="loadAccessRequests">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table :data="accessRequests" stripe>
          <el-table-column prop="username" :label="t('bastionPage.colUser')" width="100" />
          <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" min-width="120" />
          <el-table-column prop="reason" :label="t('bastionPage.accessReason')" min-width="160" show-overflow-tooltip />
          <el-table-column prop="duration_hours" :label="t('bastionPage.accessDuration')" width="90" />
          <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="100">
            <template #default="{ row }"><el-tag :type="accessStatusTag(row.status)" size="small">{{ row.status }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="expires_at" :label="t('bastionPage.accountsExpires')" width="170" />
          <el-table-column v-if="isAdmin" :label="t('common.actions')" width="160">
            <template #default="{ row }">
              <template v-if="row.status === 'pending'">
                <el-button link type="primary" @click="approveAccess(row)">{{ t('bastionPage.accessApprove') }}</el-button>
                <el-button link type="danger" @click="rejectAccess(row)">{{ t('bastionPage.accessReject') }}</el-button>
              </template>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane v-if="isAdmin" :label="t('bastionPage.tabPermissions')" name="permissions">
        <div class="toolbar-row">
          <el-button type="primary" size="small" :icon="Plus" @click="permDialog = true">{{ t('bastionPage.grantPerm') }}</el-button>
          <el-button size="small" @click="openPolicy">{{ t('bastionPage.commandPolicy') }}</el-button>
        </div>
        <el-table :data="permissions" stripe>
          <el-table-column prop="username" :label="t('bastionPage.colUser')" />
          <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" />
          <el-table-column prop="permission" :label="t('bastionPage.colPermType')" />
          <el-table-column :label="t('common.actions')" width="100">
            <template #default="{ row }">
              <el-button link type="danger" @click="deletePermission(row)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <h3 class="sub-title">{{ t('bastionPage.blockedCommands') }}</h3>
        <el-table :data="commandAudits" size="small" stripe @row-click="() => {}">
          <el-table-column prop="created_at" :label="t('bastionPage.colTime')" width="180" />
          <el-table-column prop="username" :label="t('bastionPage.colUser')" width="100" />
          <el-table-column prop="command" :label="t('bastionPage.colCommand')" min-width="200" show-overflow-tooltip />
          <el-table-column prop="action" :label="t('bastionPage.colAction')" width="90" />
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('bastionPage.tabAudit')" name="audit">
        <div class="toolbar-row">
          <el-input v-model="sessionSearch" :placeholder="t('bastionPage.searchSessions')" style="max-width:280px" clearable @keyup.enter="loadSessions" />
          <el-button :icon="Refresh" @click="loadSessions">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table v-loading="loading" :data="sessions" stripe>
          <el-table-column prop="username" :label="t('bastionPage.colUser')" width="100" />
          <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" min-width="120" />
          <el-table-column prop="host" :label="t('bastionPage.colHost')" min-width="120" />
          <el-table-column prop="start_time" :label="t('bastionPage.colStart')" width="170" />
          <el-table-column prop="end_time" :label="t('bastionPage.colEnd')" width="170" />
          <el-table-column :label="t('bastionPage.colSize')" width="90">
            <template #default="{ row }">{{ formatSize(row.log_size) }}</template>
          </el-table-column>
          <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="90">
            <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ row.status }}</el-tag></template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="160" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="VideoPlay" @click="openReplay(row)">{{ t('bastionPage.replay') }}</el-button>
              <el-button link :icon="Download" @click="downloadSession(row)">{{ t('common.download') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane v-if="isAdmin" :label="t('bastionPage.tabActive')" name="active">
        <div class="toolbar-row">
          <el-button :icon="Refresh" @click="loadActive">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table :data="activeSessions" stripe>
          <el-table-column prop="username" :label="t('bastionPage.colUser')" />
          <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" />
          <el-table-column prop="host" :label="t('bastionPage.colHost')" />
          <el-table-column prop="start_time" :label="t('bastionPage.colStart')" />
          <el-table-column :label="t('common.actions')" width="120">
            <template #default="{ row }">
              <el-button link type="danger" @click="killSession(row)">{{ t('bastionPage.disconnect') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane v-if="isAdmin" :label="t('bastionPage.tabAccounts')" name="accounts">
        <el-tabs v-model="accountSubTab" type="card" class="ops-sub-tabs">
          <el-tab-pane :label="t('bastionPage.accountsList')" name="list">
            <div class="toolbar-row">
              <el-select v-model="accountFilterAssetId" clearable :placeholder="t('bastionPage.accountsFilterAsset')" style="width:200px" @change="loadAccounts">
                <el-option v-for="a in assets.filter(x => !x.protocol || x.protocol === 'ssh')" :key="a.id" :label="a.name" :value="a.id" />
              </el-select>
              <el-button type="primary" size="small" :icon="Plus" @click="openAccount()">{{ t('bastionPage.accountsAdd') }}</el-button>
              <el-button size="small" @click="rotateBatch">{{ t('bastionPage.accountsBatchRotate') }}</el-button>
              <el-button size="small" :icon="Download" @click="exportVault">{{ t('bastionPage.accountsExportVault') }}</el-button>
              <el-button size="small" @click="triggerVaultImport">{{ t('bastionPage.accountsImportVault') }}</el-button>
              <input ref="vaultImportInput" type="file" accept=".json,application/json" style="display:none" @change="onVaultImport" />
              <el-button size="small" :icon="Refresh" @click="loadAccounts">{{ t('common.refresh') }}</el-button>
            </div>
            <el-table v-loading="loading" :data="accounts" stripe @selection-change="(rows: any[]) => { selectedAccountIds = rows.map(r => r.id) }">
              <el-table-column type="selection" width="45" />
              <el-table-column prop="asset_name" :label="t('bastionPage.colAsset')" min-width="120" />
              <el-table-column prop="username" :label="t('bastionPage.colUser')" width="110" />
              <el-table-column :label="t('bastionPage.accountsPrivileged')" width="80">
                <template #default="{ row }">
                  <el-tag v-if="row.is_privileged" type="warning" size="small">{{ t('common.yes') }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="source" :label="t('bastionPage.accountsSource')" width="100">
                <template #default="{ row }"><el-tag :type="sourceTag(row.source)" size="small">{{ row.source }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="last_rotated_at" :label="t('bastionPage.accountsLastRotated')" width="170" />
              <el-table-column :label="t('bastionPage.accountsExpires')" width="170">
                <template #default="{ row }">
                  <span :class="{ 'expiring-soon': accountExpiring(row) }">{{ row.expires_at || '—' }}</span>
                </template>
              </el-table-column>
              <el-table-column :label="t('bastionPage.accountsAutoRotate')" width="100">
                <template #default="{ row }">{{ row.auto_rotate ? `${row.rotate_days}d` : '—' }}</template>
              </el-table-column>
              <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="90" />
              <el-table-column :label="t('common.actions')" width="320" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="connectAccount(row)">{{ t('bastionPage.connect') }}</el-button>
                  <el-button link @click="testAccount(row)">{{ t('bastionPage.accountsTest') }}</el-button>
                  <el-button link @click="rotateAccount(row)">{{ t('bastionPage.accountsRotate') }}</el-button>
                  <el-button link @click="pushAccount(row)">{{ t('bastionPage.accountsPush') }}</el-button>
                  <el-button link @click="openAccount(row)">{{ t('common.edit') }}</el-button>
                  <el-button link type="danger" @click="deleteAccount(row)">{{ t('common.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
          <el-tab-pane :label="t('bastionPage.accountsRotationLogs')" name="logs">
            <div class="toolbar-row">
              <el-button :icon="Refresh" @click="loadRotationLogs">{{ t('common.refresh') }}</el-button>
            </div>
            <el-table :data="rotationLogs" stripe>
              <el-table-column prop="rotated_at" :label="t('bastionPage.colTime')" width="180" />
              <el-table-column prop="username" :label="t('bastionPage.colUser')" width="120" />
              <el-table-column prop="asset_id" :label="t('bastionPage.colAsset')" width="90" />
              <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="90">
                <template #default="{ row }"><el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="message" :label="t('common.remark')" min-width="200" show-overflow-tooltip />
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <el-tab-pane :label="t('bastionPage.tabOps')" name="ops">
        <el-tabs v-model="opsSubTab" type="card" class="ops-sub-tabs">
          <el-tab-pane :label="t('bastionPage.opsAdhoc')" name="adhoc">
            <div class="ops-layout">
              <div class="ops-assets">
                <div class="ops-panel-title">{{ t('bastionPage.opsSelectAssets') }}</div>
                <div v-for="(list, group) in assetTree" :key="group" class="asset-group">
                  <div class="group-label">{{ group }}</div>
                  <el-checkbox
                    v-for="a in list"
                    :key="a.id"
                    :model-value="selectedAssetIds.includes(a.id)"
                    @change="(v: boolean) => toggleAsset(a.id, v)"
                  >
                    {{ a.name }} ({{ a.host }})
                  </el-checkbox>
                </div>
              </div>
              <div class="ops-editor">
                <el-form label-width="80px" size="small">
                  <el-form-item :label="t('bastionPage.opsLanguage')">
                    <el-select v-model="adhocForm.language" style="width:120px">
                      <el-option v-for="l in languages" :key="l" :label="l" :value="l" />
                    </el-select>
                  </el-form-item>
                  <el-form-item :label="t('bastionPage.opsTimeout')">
                    <el-select v-model="adhocForm.timeout_sec" style="width:120px">
                      <el-option v-for="n in timeoutOptions" :key="n" :label="`${n}s`" :value="n" />
                    </el-select>
                  </el-form-item>
                  <el-form-item :label="t('bastionPage.opsCwd')">
                    <el-input v-model="adhocForm.cwd" :placeholder="t('bastionPage.opsCwdHint')" />
                  </el-form-item>
                  <el-form-item :label="t('bastionPage.opsCommand')">
                    <el-input v-model="adhocForm.command" type="textarea" :rows="10" class="cmd-editor" />
                  </el-form-item>
                  <el-form-item>
                    <el-button type="primary" :loading="adhocRunning" :icon="VideoPlay" @click="runAdhoc">
                      {{ t('bastionPage.opsRun') }}
                    </el-button>
                    <el-button :icon="Document" @click="openTemplate()">{{ t('bastionPage.opsSaveTemplate') }}</el-button>
                    <el-dropdown trigger="click" style="margin-left:8px">
                      <el-button>{{ t('bastionPage.opsUseTemplate') }}</el-button>
                      <template #dropdown>
                        <el-dropdown-menu>
                          <el-dropdown-item v-for="tpl in templates" :key="tpl.id" @click="applyTemplate(tpl)">
                            {{ tpl.name }} <el-tag v-if="tpl.builtin" size="small" type="info">built-in</el-tag>
                          </el-dropdown-item>
                        </el-dropdown-menu>
                      </template>
                    </el-dropdown>
                  </el-form-item>
                </el-form>
              </div>
            </div>
            <div v-if="adhocResults.length" class="ops-results">
              <div class="ops-panel-title">{{ t('bastionPage.opsResults') }}</div>
              <el-collapse v-model="expandedResult">
                <el-collapse-item v-for="r in adhocResults" :key="r.id" :name="r.id" :title="`${r.asset_name} — ${r.status}`">
                  <el-tag :type="opsStatusTag(r.status)" size="small">{{ r.status }}</el-tag>
                  <span class="result-meta">exit={{ r.exit_code }} · {{ r.duration_ms }}ms</span>
                  <pre class="result-output">{{ r.output }}</pre>
                </el-collapse-item>
              </el-collapse>
            </div>
            <div v-if="adhocHistory.length" class="ops-history">
              <div class="ops-panel-title">{{ t('bastionPage.opsRecentAdhoc') }}</div>
              <el-table :data="adhocHistory" size="small" stripe>
                <el-table-column prop="started_at" :label="t('bastionPage.colStart')" width="170" />
                <el-table-column prop="username" :label="t('bastionPage.colUser')" width="100" />
                <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="90">
                  <template #default="{ row }"><el-tag :type="opsStatusTag(row.status)" size="small">{{ row.status }}</el-tag></template>
                </el-table-column>
                <el-table-column prop="command" :label="t('bastionPage.colCommand')" min-width="200" show-overflow-tooltip />
                <el-table-column :label="t('common.actions')" width="100">
                  <template #default="{ row }">
                    <el-button link type="primary" @click="openRunDetail(row.id)">{{ t('bastionPage.opsDetail') }}</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </el-tab-pane>

          <el-tab-pane v-if="isAdmin" :label="t('bastionPage.opsJobs')" name="jobs">
            <div class="toolbar-row">
              <el-button type="primary" size="small" :icon="Plus" @click="openJob()">{{ t('bastionPage.opsAddJob') }}</el-button>
              <el-button size="small" :icon="Refresh" @click="loadJobs">{{ t('common.refresh') }}</el-button>
            </div>
            <el-table :data="jobs" stripe>
              <el-table-column prop="name" :label="t('bastionPage.colName')" min-width="140" />
              <el-table-column prop="template_name" :label="t('bastionPage.opsTemplate')" min-width="120" />
              <el-table-column prop="schedule" :label="t('bastionPage.opsSchedule')" width="140" />
              <el-table-column prop="last_status" :label="t('bastionPage.colStatus')" width="100">
                <template #default="{ row }">
                  <el-tag v-if="row.last_status" :type="opsStatusTag(row.last_status)" size="small">{{ row.last_status }}</el-tag>
                  <span v-else>—</span>
                </template>
              </el-table-column>
              <el-table-column prop="last_run_at" :label="t('bastionPage.opsLastRun')" width="170" />
              <el-table-column :label="t('common.actions')" width="220" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" :icon="VideoPlay" @click="runJob(row)">{{ t('bastionPage.opsRun') }}</el-button>
                  <el-button link :icon="Edit" @click="openJob(row)">{{ t('common.edit') }}</el-button>
                  <el-button link @click="showJobHistory(row)">{{ t('bastionPage.opsHistory') }}</el-button>
                  <el-button link type="danger" :icon="Delete" @click="deleteJob(row)">{{ t('common.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane v-if="isAdmin" :label="t('bastionPage.opsTemplates')" name="templates">
            <div class="toolbar-row">
              <el-button type="primary" size="small" :icon="Plus" @click="openTemplate()">{{ t('bastionPage.opsAddTemplate') }}</el-button>
              <el-button size="small" :icon="Refresh" @click="loadTemplates">{{ t('common.refresh') }}</el-button>
            </div>
            <el-table :data="templates" stripe>
              <el-table-column prop="name" :label="t('bastionPage.colName')" min-width="140" />
              <el-table-column prop="type" :label="t('bastionPage.opsType')" width="100" />
              <el-table-column prop="language" :label="t('bastionPage.opsLanguage')" width="90" />
              <el-table-column prop="remark" :label="t('common.remark')" min-width="160" show-overflow-tooltip />
              <el-table-column :label="t('bastionPage.opsBuiltin')" width="90">
                <template #default="{ row }">{{ row.builtin ? t('common.yes') : t('common.no') }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" width="200" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="applyTemplate(row)">{{ t('bastionPage.opsUse') }}</el-button>
                  <el-button v-if="!row.builtin" link :icon="Edit" @click="openTemplate(row)">{{ t('common.edit') }}</el-button>
                  <el-button v-if="!row.builtin" link type="danger" :icon="Delete" @click="deleteTemplate(row)">{{ t('common.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="accessDialog" :title="t('bastionPage.accessSubmit')" width="480px">
      <el-form label-width="100px">
        <el-form-item :label="t('bastionPage.colAsset')">
          <el-select v-model="accessForm.asset_id" filterable>
            <el-option v-for="a in assets" :key="a.id" :label="`${a.name} (${a.host})`" :value="a.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.accessReason')"><el-input v-model="accessForm.reason" type="textarea" /></el-form-item>
        <el-form-item :label="t('bastionPage.accessDuration')">
          <el-input-number v-model="accessForm.duration_hours" :min="1" :max="72" /> h
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="accessDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="submitAccessRequest">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="assetDialog" :title="editingAsset ? t('bastionPage.editAsset') : t('bastionPage.addAsset')" width="520px">
      <el-form label-width="100px">
        <el-form-item :label="t('bastionPage.colName')"><el-input v-model="assetForm.name" /></el-form-item>
        <el-form-item :label="t('bastionPage.colHost')"><el-input v-model="assetForm.host" /></el-form-item>
        <el-form-item :label="t('bastionPage.colPort')"><el-input-number v-model="assetForm.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('bastionPage.colProtocol')">
          <el-select v-model="assetForm.protocol"><el-option v-for="p in protocols" :key="p" :label="p" :value="p" /></el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colUser')"><el-input v-model="assetForm.username" /></el-form-item>
        <el-alert :title="t('bastionPage.assetAdminHint')" type="info" :closable="false" show-icon style="margin-bottom:12px" />
        <el-form-item :label="t('bastionPage.authMethod')">
          <el-radio-group v-model="assetForm.auth_method">
            <el-radio value="password">{{ t('bastionPage.authPassword') }}</el-radio>
            <el-radio value="key">{{ t('bastionPage.authKey') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="assetForm.auth_method === 'password'" :label="t('bastionPage.password')">
          <el-input v-model="assetForm.password" type="password" show-password :placeholder="editingAsset?.has_password ? t('bastionPage.passwordKeep') : ''" />
        </el-form-item>
        <el-form-item v-if="assetForm.auth_method === 'key'" :label="t('bastionPage.sshKey')">
          <el-select v-model="assetForm.key_id" clearable>
            <el-option v-for="k in sshKeys.filter(x => x.has_private)" :key="k.id" :label="k.name" :value="k.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colGroup')">
          <el-select v-model="assetForm.group_id" clearable>
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colTags')"><el-input v-model="assetForm.tags" /></el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="assetForm.remark" type="textarea" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="assetDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveAsset">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="accountDialog" :title="editingAccount ? t('bastionPage.accountsEdit') : t('bastionPage.accountsAdd')" width="520px">
      <el-form label-width="110px">
        <el-form-item :label="t('bastionPage.colAsset')">
          <el-select v-model="accountForm.asset_id" filterable :disabled="!!editingAccount">
            <el-option v-for="a in assets.filter(x => !x.protocol || x.protocol === 'ssh')" :key="a.id" :label="a.name" :value="a.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colUser')"><el-input v-model="accountForm.username" /></el-form-item>
        <el-form-item :label="t('bastionPage.authMethod')">
          <el-radio-group v-model="accountForm.auth_method">
            <el-radio value="password">{{ t('bastionPage.authPassword') }}</el-radio>
            <el-radio value="key">{{ t('bastionPage.authKey') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="accountForm.auth_method === 'password'" :label="t('bastionPage.password')">
          <el-input v-model="accountForm.password" type="password" show-password :placeholder="editingAccount?.has_password ? t('bastionPage.passwordKeep') : ''" />
        </el-form-item>
        <el-form-item v-if="accountForm.auth_method === 'key'" :label="t('bastionPage.sshKey')">
          <el-select v-model="accountForm.key_id" clearable>
            <el-option v-for="k in sshKeys.filter(x => x.has_private)" :key="k.id" :label="k.name" :value="k.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.accountsPrivileged')">
          <el-switch v-model="accountForm.is_privileged" @change="(v: boolean) => { if (v) accountForm.auto_rotate = true }" />
        </el-form-item>
        <el-form-item :label="t('bastionPage.accountsAutoRotate')">
          <el-switch v-model="accountForm.auto_rotate" />
          <el-input-number v-if="accountForm.auto_rotate" v-model="accountForm.rotate_days" :min="1" :max="365" style="margin-left:12px" />
        </el-form-item>
        <el-form-item :label="t('bastionPage.rotateAfterSession')">
          <el-switch v-model="accountForm.rotate_after_session" />
        </el-form-item>
        <el-form-item :label="t('bastionPage.colStatus')">
          <el-select v-model="accountForm.status"><el-option v-for="s in accountStatuses" :key="s" :label="s" :value="s" /></el-select>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="accountForm.remark" type="textarea" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="accountDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveAccount">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="groupDialog" :title="t('bastionPage.addGroup')" width="400px">
      <el-form label-width="80px">
        <el-form-item :label="t('bastionPage.colName')"><el-input v-model="groupForm.name" /></el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="groupForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveGroup">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permDialog" :title="t('bastionPage.grantPerm')" width="420px">
      <el-form label-width="90px">
        <el-form-item :label="t('bastionPage.colUser')">
          <el-select v-model="permForm.user_id" filterable>
            <el-option v-for="u in users" :key="u.id" :label="u.username" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colAsset')">
          <el-select v-model="permForm.asset_id" filterable>
            <el-option v-for="a in assets" :key="a.id" :label="a.name" :value="a.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colPermType')">
          <el-select v-model="permForm.permission"><el-option v-for="p in permTypes" :key="p" :label="p" :value="p" /></el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="permDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="savePermission">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="policyDialog" :title="t('bastionPage.commandPolicy')" width="560px">
      <el-form label-width="100px">
        <el-form-item :label="t('bastionPage.policyMode')">
          <el-radio-group v-model="policyForm.mode">
            <el-radio value="block">{{ t('bastionPage.modeBlock') }}</el-radio>
            <el-radio value="warn">{{ t('bastionPage.modeWarn') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('bastionPage.blocklist')">
          <el-input v-model="policyBlocklistText" type="textarea" :rows="10" :placeholder="t('bastionPage.blocklistHint')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="policyDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="savePolicy">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="replayDialog" :title="t('bastionPage.replay')" width="80%" top="5vh">
      <p class="replay-meta">{{ replayTitle }}</p>
      <div v-if="replayCommands.length" class="cmd-list">
        <h4>{{ t('bastionPage.extractedCommands') }}</h4>
        <pre>{{ replayCommands.join('\n') }}</pre>
      </div>
      <div class="replay-log"><pre>{{ replayLog }}</pre></div>
    </el-dialog>

    <el-dialog v-model="templateDialog" :title="editingTemplate ? t('bastionPage.opsEditTemplate') : t('bastionPage.opsAddTemplate')" width="560px">
      <el-form label-width="90px">
        <el-form-item :label="t('bastionPage.colName')"><el-input v-model="templateForm.name" /></el-form-item>
        <el-form-item :label="t('bastionPage.opsType')">
          <el-select v-model="templateForm.type"><el-option v-for="x in templateTypes" :key="x" :label="x" :value="x" /></el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.opsLanguage')">
          <el-select v-model="templateForm.language"><el-option v-for="l in languages" :key="l" :label="l" :value="l" /></el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.opsCommand')"><el-input v-model="templateForm.content" type="textarea" :rows="8" /></el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="templateForm.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="templateDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveTemplate">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="jobDialog" :title="editingJob ? t('bastionPage.opsEditJob') : t('bastionPage.opsAddJob')" width="560px">
      <el-form label-width="100px">
        <el-form-item :label="t('bastionPage.colName')"><el-input v-model="jobForm.name" /></el-form-item>
        <el-form-item :label="t('bastionPage.opsTemplate')">
          <el-select v-model="jobForm.template_id" filterable>
            <el-option v-for="tpl in templates" :key="tpl.id" :label="tpl.name" :value="tpl.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.colAsset')">
          <el-select v-model="jobForm.asset_ids" multiple filterable style="width:100%">
            <el-option v-for="a in assets.filter(x => !x.protocol || x.protocol === 'ssh')" :key="a.id" :label="a.name" :value="a.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.opsSchedule')">
          <el-input v-model="jobForm.schedule" :placeholder="t('bastionPage.opsScheduleHint')" />
        </el-form-item>
        <el-form-item :label="t('bastionPage.opsTimeout')">
          <el-select v-model="jobForm.timeout_sec"><el-option v-for="n in timeoutOptions" :key="n" :label="`${n}s`" :value="n" /></el-select>
        </el-form-item>
        <el-form-item :label="t('bastionPage.opsCwd')"><el-input v-model="jobForm.cwd" /></el-form-item>
        <el-form-item :label="t('bastionPage.opsEnabled')"><el-switch v-model="jobForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="jobDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveJob">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="jobHistoryDialog" :title="t('bastionPage.opsHistory')" width="640px">
      <p v-if="jobHistoryJob" class="replay-meta">{{ jobHistoryJob.name }}</p>
      <el-table :data="jobRuns" size="small" stripe>
        <el-table-column prop="started_at" :label="t('bastionPage.colStart')" width="170" />
        <el-table-column prop="triggered_by" :label="t('bastionPage.opsTrigger')" width="90" />
        <el-table-column prop="status" :label="t('bastionPage.colStatus')" width="90">
          <template #default="{ row }"><el-tag :type="opsStatusTag(row.status)" size="small">{{ row.status }}</el-tag></template>
        </el-table-column>
        <el-table-column :label="t('common.actions')" width="100">
          <template #default="{ row }">
            <el-button link type="primary" @click="openRunDetail(row.id)">{{ t('bastionPage.opsDetail') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="runDetailDialog" :title="t('bastionPage.opsRunDetail')" width="720px">
      <div v-loading="runDetailLoading">
        <template v-if="runDetail">
          <p class="replay-meta">{{ runDetail.job_name || t('bastionPage.opsAdhoc') }} · {{ runDetail.status }} · {{ runDetail.started_at }}</p>
          <el-collapse>
            <el-collapse-item v-for="r in runDetail.results || []" :key="r.id" :title="`${r.asset_name} — ${r.status}`">
              <el-tag :type="opsStatusTag(r.status)" size="small">{{ r.status }}</el-tag>
              <span class="result-meta">exit={{ r.exit_code }} · {{ r.duration_ms }}ms</span>
              <pre class="result-output">{{ r.output }}</pre>
            </el-collapse-item>
          </el-collapse>
        </template>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.bastion-page .page-header, .terminal-pam-panel .pam-toolbar { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.compliance-bar { display: flex; align-items: center; gap: 12px; margin-top: 12px; }
.page-desc { color: var(--el-text-color-secondary); margin-top: 4px; }
.toolbar-row { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.sub-title { margin: 20px 0 8px; font-size: 14px; }
.replay-log pre, .cmd-list pre {
  background: #0d1117; color: #c9d1d9; padding: 12px; border-radius: 8px;
  max-height: 60vh; overflow: auto; font-size: 12px; white-space: pre-wrap;
}
.replay-meta { margin-bottom: 8px; color: var(--el-text-color-secondary); }
.asset-group { margin-bottom: 10px; }
.group-label { font-size: 12px; font-weight: 600; color: var(--el-text-color-secondary); margin-bottom: 4px; }
.asset-group :deep(.el-checkbox) { display: flex; margin: 4px 0; }
.ops-layout { display: grid; grid-template-columns: 260px 1fr; gap: 16px; min-height: 320px; }
.ops-assets { border: 1px solid var(--el-border-color-lighter); border-radius: 8px; padding: 10px; overflow: auto; max-height: 420px; }
.ops-editor { border: 1px solid var(--el-border-color-lighter); border-radius: 8px; padding: 12px; }
.ops-panel-title { font-weight: 600; font-size: 13px; margin-bottom: 8px; }
.cmd-editor :deep(textarea) { font-family: monospace; font-size: 13px; }
.ops-results, .ops-history { margin-top: 16px; }
.result-output { background: #0d1117; color: #c9d1d9; padding: 10px; border-radius: 6px; font-size: 12px; white-space: pre-wrap; max-height: 240px; overflow: auto; margin-top: 8px; }
.result-meta { margin-left: 8px; font-size: 12px; color: var(--el-text-color-secondary); }
@media (max-width: 900px) { .ops-layout { grid-template-columns: 1fr; } }
.expiring-soon { color: var(--el-color-warning); font-weight: 500; }
</style>
