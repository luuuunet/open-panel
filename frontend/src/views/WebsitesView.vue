<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import api, { AI_REQUEST_TIMEOUT, resolveApiError } from '@/api'
import { formatBytes } from '@/utils/formatBytes'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Folder, VideoPlay, VideoPause, Delete, Search, EditPen, FolderOpened, RefreshRight, Download, Setting, Odometer, Lock, Check, ArrowDown, MagicStick } from '@element-plus/icons-vue'
import SiteModifyDialog from '@/components/SiteModifyDialog.vue'
import SiteBackupDialog from '@/components/SiteBackupDialog.vue'

const { t } = useI18n()
const router = useRouter()

const projects = ref<any[]>([])
const options = ref<any>({})
const webserver = ref<any>({ active: 'nginx', servers: [] })
const projectTab = ref('php')
const searchText = ref('')
const selectedRows = ref<any[]>([])
const switchingWebServer = ref(false)
const cacheGlobalEnabled = ref(false)
const cacheAccelLoadingId = ref<number | null>(null)
const crossSiteLoadingId = ref<number | null>(null)
const phpAccelLoadingId = ref<number | null>(null)
const phpVersionLoadingId = ref<number | null>(null)

const ACTIONS_COL_MIN = 280
const ACTIONS_COL_MAX = 420
const actionsColWidth = ref(320)
const sitesTableRef = ref<{ doLayout?: () => void }>()
let actionsLayoutObserver: ResizeObserver | undefined
let actionsWidthRaf = 0

function scheduleActionsColWidthSync() {
  cancelAnimationFrame(actionsWidthRaf)
  actionsWidthRaf = requestAnimationFrame(() => {
    nextTick(() => syncActionsColWidth())
  })
}

function syncActionsColWidth() {
  const bars = document.querySelectorAll<HTMLElement>('.websites-page .site-actions')
  if (!bars.length) {
    actionsColWidth.value = ACTIONS_COL_MIN
    return
  }
  let max = ACTIONS_COL_MIN
  bars.forEach((el) => {
    const wrap = el.style.flexWrap
    const width = el.style.width
    el.style.flexWrap = 'nowrap'
    el.style.width = 'max-content'
    max = Math.max(max, el.scrollWidth + 24)
    el.style.flexWrap = wrap
    el.style.width = width
  })
  actionsColWidth.value = Math.min(ACTIONS_COL_MAX, max)
  nextTick(() => sitesTableRef.value?.doLayout?.())
}

function bindActionsColLayoutObserver() {
  actionsLayoutObserver?.disconnect()
  const tableEl = document.querySelector('.websites-page .sites-table')
  if (!tableEl) return
  actionsLayoutObserver = new ResizeObserver(() => scheduleActionsColWidthSync())
  actionsLayoutObserver.observe(tableEl)
}

const remarkDialogVisible = ref(false)
const remarkSaving = ref(false)
const remarkEditRow = ref<any>(null)
const remarkEditValue = ref('')

const expiresDialogVisible = ref(false)
const expiresSaving = ref(false)
const expiresEditRow = ref<any>(null)
const expiresPermanent = ref(true)
const expiresDate = ref('')

const dialogVisible = ref(false)
const createTab = ref('create')
const submitting = ref(false)
const credDialog = ref(false)
const credentials = ref<any>(null)
const domainError = ref('')

const pathPickerVisible = ref(false)
const pathEntries = ref<any[]>([])
const browsePath = ref('/')

const form = ref({
  domains_text: '',
  description: '',
  root_path: '',
  ftp: 'create',
  database: 'none',
  php_version: '8.3',
  category: '默认类别',
  dns_mode: 'manual',
  ssl: false,
  expires_at: '',
  expires_permanent: true,
})

const batchForm = ref({
  batch_text: '',
  dns_mode: 'manual',
  category: '默认类别',
})

const batchCredentials = ref<any[]>([])
const batchCredDialog = ref(false)

const siteModifyVisible = ref(false)
const siteModifyId = ref<number | null>(null)
const siteModifyMenu = ref('domain')

const siteBackupVisible = ref(false)
const siteBackupId = ref<number | null>(null)
const siteBackupDomain = ref('')

const fileDrawer = ref(false)
const fileEntries = ref<any[]>([])
const fileContent = ref('')
const editingFile = ref('')
const currentSite = ref<any>(null)

const javaDialogVisible = ref(false)
const javaForm = ref({
  name: '',
  domain: '',
  path: '',
  port: 8080,
  java_ver: '17',
  tomcat_key: 'tomcat9',
  context_path: '/',
  remark: '',
})

const aiForm = ref({
  repo_url: '',
  branch: 'main',
  github_token: '',
  domain: '',
  notes: '',
})
const aiPlan = ref<any>(null)
const aiAnalyzing = ref(false)
const aiDeploying = ref(false)
const aiAutoRunning = ref(false)
const aiWizardStep = ref(0)
const aiEnvDialogVisible = ref(false)
const aiDomainDialogVisible = ref(false)
const aiEnvOptions = ref<Array<{
  key: string
  label: string
  description: string
  required: boolean
  installed: boolean
  selected: boolean
}>>([])
const aiSelectedApps = ref<string[]>([])
const aiDomainDraft = ref({ domain: '', dns_mode: 'manual' })
const aiAutoPhase = ref<'idle' | 'analyze' | 'plan' | 'execute' | 'deploy' | 'retry'>('idle')
const aiPipelineActive = ref('analyze')
const aiPipelinePhases = ['analyze', 'plan', 'execute', 'deploy'] as const
type AiPipelinePhase = typeof aiPipelinePhases[number]

function parseAiPipelineSteps(job: any): Array<{ phase: string; status: string; log: string; error?: string }> {
  if (!job?.steps_json) {
    return aiPipelinePhases.map((phase) => ({ phase, status: 'pending', log: '' }))
  }
  try {
    const steps = JSON.parse(job.steps_json)
    if (Array.isArray(steps) && steps.length) return steps
  } catch { /* ignore */ }
  return aiPipelinePhases.map((phase) => ({ phase, status: 'pending', log: '' }))
}

const aiPipelineSteps = computed(() => parseAiPipelineSteps(aiJob.value))

function syncAiPipelineFromJob(job: any) {
  const phase = String(job?.current_phase || '').trim() as AiPipelinePhase
  if (aiPipelinePhases.includes(phase)) {
    aiPipelineActive.value = phase
    aiAutoPhase.value = phase
  } else if (job?.status === 'running') {
    const running = aiPipelineSteps.value.find((s) => s.status === 'running')
    if (running) {
      aiPipelineActive.value = running.phase
      aiAutoPhase.value = running.phase as typeof aiAutoPhase.value
    }
  }
}

function aiPipelinePhaseLabel(phase: string) {
  const key = `websites.aiPipeline${phase.charAt(0).toUpperCase()}${phase.slice(1)}`
  return t(key)
}

function aiPipelineStatusLabel(status: string) {
  if (status === 'running') return t('websites.aiPipelineRunning')
  if (status === 'done') return t('websites.aiPipelineDone')
  if (status === 'failed') return t('websites.aiPipelineFailed')
  return t('websites.aiPipelinePending')
}

function aiPipelineTagType(status: string): 'info' | 'success' | 'danger' | 'warning' {
  if (status === 'done') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'running') return 'warning'
  return 'info'
}

const aiActivePhaseLog = computed(() => {
  const step = aiPipelineSteps.value.find((s) => s.phase === aiPipelineActive.value)
  return step?.log || ''
})

const aiOptionalOpen = ref<string[]>([])
const aiAdvancedPlanOpen = ref<string[]>([])
const aiJob = ref<any>(null)
const aiPlanEdit = ref({
  domain: '',
  php_version: '8.3',
  need_database: false,
  deploy_script: '',
  summary: '',
  post_notes: '',
  document_root: '',
  use_docker: false,
  use_pm2: false,
  node_port: 31080,
  enable_webhook: true,
  rollback_on_fail: true,
})
let aiJobTimer: ReturnType<typeof setInterval> | null = null
const aiLogPreRef = ref<HTMLElement | null>(null)

function scrollAiLogToBottom() {
  nextTick(() => {
    const el = aiLogPreRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

watch(() => aiJob.value?.log, () => scrollAiLogToBottom())

const aiRepairDialogVisible = ref(false)
const aiRepairLoading = ref(false)
const aiRepairLogs = ref<string[]>([])
const aiRepairSummary = ref('')
const aiRepairDiagnosis = ref('')
const aiRepairFixed = ref<boolean | null>(null)
const aiRepairSiteDomain = ref('')

const categoryOptions = computed(() =>
  (options.value.categories || []).map((c: any) => ({ value: c.name, label: c.name }))
)

const activeServer = computed(() => {
  const active = webserver.value?.active || 'nginx'
  return (webserver.value?.servers || []).find((s: any) => s.key === active)
    || (webserver.value?.servers || []).find((s: any) => s.status === 'running')
    || webserver.value?.servers?.[0]
})

const otherServer = computed(() => {
  const active = webserver.value?.active || 'nginx'
  return (webserver.value?.servers || []).find((s: any) => s.key !== active)
})

async function loadProjects() {
  const [projRes, optRes, wsRes, cacheRes]: any[] = await Promise.all([
    api.get('/websites/projects', { params: { type: projectTab.value, search: searchText.value || undefined } }),
    api.get('/websites/options'),
    api.get('/websites/webserver'),
    api.get('/cache/config').catch(() => ({ data: null })),
  ])
  projects.value = projRes.data || []
  options.value = optRes.data || {}
  webserver.value = wsRes.data || { active: 'nginx', servers: [] }
  cacheGlobalEnabled.value = !!cacheRes?.data?.enabled
  const root = options.value.default_root || ''
  if (!form.value.root_path) form.value.root_path = root
  scheduleActionsColWidthSync()
}

function siteCacheActive(row: any) {
  return row.source === 'website' && row.cache_enabled && cacheGlobalEnabled.value
}

function openRemarkEdit(row: any) {
  remarkEditRow.value = row
  remarkEditValue.value = row.remark || ''
  remarkDialogVisible.value = true
}

async function saveSiteRemarkEdit() {
  const row = remarkEditRow.value
  if (!row) return
  remarkSaving.value = true
  try {
    if (row.source === 'website') {
      await api.patch(`/websites/${row.id}`, { remark: remarkEditValue.value })
    } else if (row.source === 'java') {
      await api.patch(`/java/${row.id}`, { remark: remarkEditValue.value })
    } else if (row.source === 'node') {
      await api.patch(`/nodejs/${row.id}`, { remark: remarkEditValue.value })
    } else {
      return
    }
    row.remark = remarkEditValue.value.trim()
    ElMessage.success(t('common.saved'))
    remarkDialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    remarkSaving.value = false
  }
}

function isSiteExpired(row: any) {
  if (!row?.expires_at) return false
  return new Date(row.expires_at).getTime() < Date.now()
}

function openExpiresEdit(row: any) {
  if (row.source !== 'website') return
  expiresEditRow.value = row
  if (row.expires_at) {
    expiresPermanent.value = false
    expiresDate.value = String(row.expires_at).slice(0, 10)
  } else {
    expiresPermanent.value = true
    expiresDate.value = ''
  }
  expiresDialogVisible.value = true
}

async function saveSiteExpiresEdit() {
  const row = expiresEditRow.value
  if (!row || row.source !== 'website') return
  if (!expiresPermanent.value && !expiresDate.value) {
    ElMessage.warning(t('websites.expiresDateRequired'))
    return
  }
  expiresSaving.value = true
  try {
    const payload = { expires_at: expiresPermanent.value ? '' : expiresDate.value }
    await api.patch(`/websites/${row.id}`, payload)
    row.expires_at = expiresPermanent.value ? null : expiresDate.value
    row.expires_label = expiresPermanent.value ? t('websites.permanent') : expiresDate.value
    ElMessage.success(t('common.saved'))
    expiresDialogVisible.value = false
    await loadProjects()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    expiresSaving.value = false
  }
}

async function toggleCrossSiteProtect(row: any) {
  if (row.source !== 'website') return
  const enable = !row.cross_site_protect_enabled
  crossSiteLoadingId.value = row.id
  try {
    const res: any = await api.post(`/websites/${row.id}/cross-site-protect/toggle`, { enabled: enable })
    row.cross_site_protect_enabled = !!res.data?.cross_site_protect_enabled
    ElMessage.success(enable ? t('websites.crossSiteOn') : t('websites.crossSiteOff'))
    await loadProjects()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.crossSiteFailed')))
  } finally {
    crossSiteLoadingId.value = null
  }
}

async function changePhpVersion(row: any, version: string, opt?: { installed?: boolean; key?: string }) {
  if (row.source !== 'website' || row.php_version_value === version) return
  if (version !== 'static' && opt && opt.installed === false && opt.key) {
    try {
      await ElMessageBox.confirm(
        t('websites.phpVersionInstallHint', { version }),
        t('websites.phpVersion'),
        { type: 'info', confirmButtonText: t('websites.goInstallPhp'), cancelButtonText: t('common.cancel') }
      )
      await router.push({ path: '/software', query: { key: opt.key } })
    } catch {
      /* cancelled */
    }
    return
  }
  phpVersionLoadingId.value = row.id
  try {
    await api.patch(`/websites/${row.id}`, { php_version: version })
    ElMessage.success(t('websites.phpVersionUpdated'))
    await loadProjects()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.phpVersionFailed')))
  } finally {
    phpVersionLoadingId.value = null
  }
}

function onPhpMenuCommand(row: any, cmd: { action: string; value?: string; installed?: boolean; key?: string }) {
  if (cmd.action === 'version' && cmd.value) {
    void changePhpVersion(row, cmd.value, cmd)
    return
  }
  if (cmd.action === 'accel') {
    void togglePhpAccel(row)
  }
}

async function togglePhpAccel(row: any) {
  if (row.source !== 'website' || !row.php_enabled) return
  const enable = !row.php_accel_enabled
  phpAccelLoadingId.value = row.id
  try {
    const res: any = await api.post(`/websites/${row.id}/php-accel/toggle`, { enabled: enable }, { timeout: 1200000 })
    row.php_accel_enabled = !!res.data?.php_accel_enabled
    row.cache_enabled = !!res.data?.cache_enabled
    if (res.data?.cache_enabled) cacheGlobalEnabled.value = true
    ElMessage.success(res.data?.message || (enable ? t('websites.phpAccelOn') : t('websites.phpAccelOff')))
    await loadProjects()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.phpAccelFailed')))
  } finally {
    phpAccelLoadingId.value = null
  }
}

async function autoEnableSiteCache(row: any) {
  if (row.source !== 'website') return
  if (siteCacheActive(row)) {
    ElMessage.info(t('websites.accelEnabled'))
    return
  }
  cacheAccelLoadingId.value = row.id
  try {
    const res: any = await api.post(`/cache/sites/${row.id}/auto-enable`, {}, { timeout: 1200000 })
    row.cache_enabled = true
    cacheGlobalEnabled.value = !!res.data?.global_enabled
    ElMessage.success(res.data?.message || t('websites.accelSuccess'))
    await loadProjects()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.accelFailed')))
  } finally {
    cacheAccelLoadingId.value = null
  }
}

async function load() {
  await loadProjects()
}

watch(projectTab, () => {
  selectedRows.value = []
  loadProjects()
})

function openDialog() {
  if (projectTab.value === 'java') {
    javaDialogVisible.value = true
    return
  }
  if (projectTab.value === 'node') {
    router.push({ path: '/runtimes', query: { tab: 'nodejs' } })
    return
  }
  if (projectTab.value !== 'php') {
    ElMessage.info(t('websites.phpTabOnlyCreate'))
    return
  }
  domainError.value = ''
  resetForms()
  createTab.value = 'aiGithub'
  dialogVisible.value = true
}

function resetForms() {
  const root = options.value.default_root || ''
  form.value = {
    domains_text: '',
    description: '',
    root_path: root,
    ftp: 'create',
    database: 'none',
    php_version: options.value.php_versions?.find((p: any) => p.value !== 'static')?.value || '8.3',
    category: categoryOptions.value[0]?.value || '默认类别',
    dns_mode: 'manual',
    ssl: false,
    expires_at: '',
    expires_permanent: true,
  }
  batchForm.value = {
    batch_text: '',
    dns_mode: 'manual',
    category: categoryOptions.value[0]?.value || '默认类别',
  }
  resetAIForm()
}

function resetAIForm() {
  aiForm.value = { repo_url: '', branch: 'main', github_token: '', domain: '', notes: '' }
  aiPlan.value = null
  aiJob.value = null
  aiAutoPhase.value = 'idle'
  aiPipelineActive.value = 'analyze'
  aiWizardStep.value = 0
  aiEnvDialogVisible.value = false
  aiDomainDialogVisible.value = false
  aiEnvOptions.value = []
  aiSelectedApps.value = []
  aiDomainDraft.value = { domain: '', dns_mode: 'manual' }
  aiPlanEdit.value = {
    domain: '',
    php_version: '8.3',
    need_database: false,
    deploy_script: '',
    summary: '',
    post_notes: '',
    document_root: '',
    use_docker: false,
    use_pm2: false,
    node_port: 31080,
    enable_webhook: true,
    rollback_on_fail: true,
  }
  stopAIJobPoll()
}

function stopAIJobPoll() {
  if (aiJobTimer) {
    clearInterval(aiJobTimer)
    aiJobTimer = null
  }
}

function applyAIPlanResult(data: any) {
  aiPlan.value = data
  const p = data?.plan || {}
  const repoBranch = String(data?.repo?.branch || '').trim()
  if (repoBranch) {
    aiForm.value.branch = repoBranch
  }
  aiPlanEdit.value = {
    domain: p.domain || suggestDomainFromRepo(aiForm.value.repo_url),
    php_version: p.php_version || '8.3',
    need_database: !!p.need_database,
    deploy_script: p.deploy_script || '',
    summary: p.summary || '',
    post_notes: p.post_notes || '',
    document_root: p.document_root || '',
    use_docker: !!p.use_docker,
    use_pm2: !!p.use_pm2,
    node_port: p.node_port || 31080,
    enable_webhook: p.enable_webhook !== false,
    rollback_on_fail: p.rollback_on_fail !== false,
  }
  aiEnvOptions.value = Array.isArray(data?.required_env) ? data.required_env : []
  aiSelectedApps.value = aiEnvOptions.value.filter((e) => e.selected).map((e) => e.key)
  aiDomainDraft.value.domain = aiPlanEdit.value.domain || suggestDomainFromRepo(aiForm.value.repo_url)
  aiDomainDraft.value.dns_mode = 'manual'
}

function onEnvSelectionChange(keys: string[]) {
  aiSelectedApps.value = keys
  aiEnvOptions.value.forEach((e) => {
    e.selected = keys.includes(e.key)
  })
}

async function handleAIWizardAnalyze() {
  if (!aiForm.value.repo_url.trim()) {
    ElMessage.warning(t('websites.aiRepoRequired'))
    return
  }
  aiAnalyzing.value = true
  aiPlan.value = null
  aiWizardStep.value = 0
  try {
    const res: any = await api.post('/websites/ai/analyze', {
      repo_url: aiForm.value.repo_url,
      branch: aiForm.value.branch || 'main',
      github_token: aiForm.value.github_token,
      notes: aiForm.value.notes || '',
    }, { timeout: AI_REQUEST_TIMEOUT })
    applyAIPlanResult(res.data)
    aiWizardStep.value = 1
    aiEnvDialogVisible.value = true
    ElMessage.success(t('websites.aiAnalyzeDone'))
  } catch (e: any) {
    ElMessage.error(e?.error || t('websites.aiAnalyzeFailed'))
  } finally {
    aiAnalyzing.value = false
  }
}

function confirmEnvSelection() {
  const missingRequired = aiEnvOptions.value.filter(
    (e) => e.required && !e.installed && !aiSelectedApps.value.includes(e.key),
  )
  if (missingRequired.length) {
    ElMessage.warning(t('websites.aiEnvRequiredMissing', { names: missingRequired.map((e) => e.label).join('、') }))
    return
  }
  if (!aiSelectedApps.value.length && aiEnvOptions.value.some((e) => !e.installed)) {
    ElMessage.warning(t('websites.aiEnvSelectOne'))
    return
  }
  if (!aiEnvOptions.value.length) {
    aiSelectedApps.value = []
  }
  aiEnvDialogVisible.value = false
  aiWizardStep.value = 2
  aiDomainDialogVisible.value = true
}

async function startAIDeployFromWizard() {
  const domain = aiDomainDraft.value.domain.trim()
  if (!domain) {
    ElMessage.warning(t('websites.domainRequired'))
    return
  }
  domainError.value = ''
  try {
    const checkRes: any = await api.post('/domains/check', { domains_text: domain })
    if (!checkRes.data?.available) {
      const c = checkRes.data?.conflicts?.[0]
      domainError.value = c ? `${c.domain}: ${c.owner}` : t('websites.domainTaken')
      ElMessage.error(domainError.value)
      return
    }
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.createFailed')))
    return
  }

  aiDomainDialogVisible.value = false
  aiWizardStep.value = 3
  aiPlanEdit.value.domain = domain
  aiAutoRunning.value = true
  aiAutoPhase.value = 'plan'
  aiJob.value = null
  aiAdvancedPlanOpen.value = []
  aiDeploying.value = true

  try {
    const plan = {
      ...aiPlan.value?.plan,
      domain,
      php_version: aiPlanEdit.value.php_version,
      need_database: aiPlanEdit.value.need_database,
      deploy_script: aiPlanEdit.value.deploy_script,
      summary: aiPlanEdit.value.summary,
      post_notes: aiPlanEdit.value.post_notes,
      document_root: aiPlanEdit.value.document_root,
      use_docker: aiPlanEdit.value.use_docker,
      use_pm2: aiPlanEdit.value.use_pm2,
      node_port: aiPlanEdit.value.node_port,
      enable_webhook: aiPlanEdit.value.enable_webhook,
      rollback_on_fail: aiPlanEdit.value.rollback_on_fail,
    }
    const res: any = await api.post('/websites/ai/deploy', {
      repo_url: aiForm.value.repo_url,
      branch: String(aiPlan.value?.repo?.branch || aiForm.value.branch || 'main').trim(),
      github_token: aiForm.value.github_token,
      notes: aiForm.value.notes || '',
      selected_apps: aiSelectedApps.value,
      plan,
    }, { timeout: 30000 })
    aiJob.value = res.data
    syncAiPipelineFromJob(res.data)
    scrollAiLogToBottom()
    ElMessage.success(t('websites.aiDeployStarted'))
    startAIJobPoll(res.data?.id)
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.createFailed'), t('common.requestTimeout')))
    aiAutoRunning.value = false
    aiAutoPhase.value = 'idle'
    aiWizardStep.value = 2
    aiDomainDialogVisible.value = true
  } finally {
    aiDeploying.value = false
  }
}

function suggestDomainFromRepo(url: string) {
  const u = url.trim().replace(/\/+$/, '').replace(/\.git$/, '')
  const parts = u.split('/')
  const name = parts[parts.length - 1]?.toLowerCase()
  return name ? `${name}.local` : 'github-site.local'
}

const aiJobRetrying = computed(() => {
  const log = String(aiJob.value?.log || '')
  return /自动安装并重试|自动安装缺失组件|根据部署日志安装缺失组件/.test(log)
})

const aiProgressLabel = computed(() => {
  if (aiAutoPhase.value === 'analyze') return t('websites.aiPhaseAnalyze')
  if (aiAutoPhase.value === 'plan') return t('websites.aiPhasePlan')
  if (aiAutoPhase.value === 'execute') return t('websites.aiPhaseExecute')
  if (aiAutoPhase.value === 'deploy') return t('websites.aiPhaseDeploy')
  if (aiAutoPhase.value === 'retry' || aiJobRetrying.value) return t('websites.aiPhaseRetry')
  if (aiAnalyzing.value) return t('websites.aiAnalyzing')
  if (aiDeploying.value) return t('websites.aiDeployRunning')
  return t('websites.aiAutoDeploying')
})

const aiProgressDetail = computed(() => {
  if (aiJob.value?.log) {
    const lines = String(aiJob.value.log).split('\n').filter(Boolean)
    return lines[lines.length - 1] || ''
  }
  return aiPlan.value?.ai_reply || ''
})

const aiDeployLogText = computed(() => {
  const phaseLog = aiActivePhaseLog.value.trim()
  if (phaseLog) return phaseLog
  const log = String(aiJob.value?.log || '').trim()
  if (log) return log
  if (!aiAutoRunning.value) return ''
  const hint = aiProgressLabel.value
  const detail = aiProgressDetail.value
  return detail && detail !== hint ? `${hint}\n${detail}` : hint
})

const aiShowDeployLog = computed(() => aiAutoRunning.value || !!aiJob.value)

function startAIJobPoll(jobId: number) {
  stopAIJobPoll()
  if (!jobId) return
  aiAutoRunning.value = true

  const poll = async () => {
    try {
      const res: any = await api.get(`/websites/ai/jobs/${jobId}`)
      aiJob.value = res.data
      syncAiPipelineFromJob(res.data)
      if (aiJobRetrying.value) {
        aiAutoPhase.value = 'retry'
        aiPipelineActive.value = 'execute'
      }
      scrollAiLogToBottom()
      if (res.data?.status === 'success') {
        stopAIJobPoll()
        aiAutoRunning.value = false
        aiAutoPhase.value = 'idle'
        ElMessage.success(t('websites.aiDeploySuccess'))
        dialogVisible.value = false
        resetAIForm()
        load()
      } else if (res.data?.status === 'failed') {
        stopAIJobPoll()
        aiAutoRunning.value = false
        aiAutoPhase.value = 'idle'
        const failedStep = parseAiPipelineSteps(res.data).find((s) => s.status === 'failed')
        if (failedStep) aiPipelineActive.value = failedStep.phase
        ElMessage.error(res.data?.error || t('websites.aiDeployFailed'))
      }
    } catch {
      stopAIJobPoll()
      aiAutoRunning.value = false
      aiAutoPhase.value = 'idle'
    }
  }

  poll()
  aiJobTimer = setInterval(poll, 1000)
}

async function handleCreate() {
  if (!form.value.domains_text.trim()) {
    domainError.value = t('websites.domainRequired')
    return
  }
  domainError.value = ''
  submitting.value = true
  try {
    const checkRes: any = await api.post('/domains/check', {
      domains_text: form.value.domains_text,
    })
    if (!checkRes.data?.available) {
      const c = checkRes.data?.conflicts?.[0]
      domainError.value = c ? `${c.domain}: ${c.owner}` : t('websites.domainTaken')
      ElMessage.error(domainError.value)
      return
    }
    const payload = {
      ...form.value,
      expires_at: form.value.expires_permanent ? '' : form.value.expires_at,
    }
    delete (payload as any).expires_permanent
    const res: any = await api.post('/websites', payload)
    ElMessage.success(t('websites.created'))
    dialogVisible.value = false
    if (res.data?.ftp_password || res.data?.db_password) {
      credentials.value = res.data
      credDialog.value = true
    }
    resetForms()
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.createFailed')))
  } finally {
    submitting.value = false
  }
}

async function handleBatchCreate() {
  if (!batchForm.value.batch_text.trim()) {
    domainError.value = t('websites.domainRequired')
    return
  }
  domainError.value = ''
  submitting.value = true
  try {
    const res: any = await api.post('/websites/batch', batchForm.value)
    const ok = res.data?.created?.length || 0
    const fail = res.data?.failed?.length || 0
    ElMessage.success(t('websites.batchCreated', { ok, fail }))
    dialogVisible.value = false
    const withCreds = (res.data?.created || []).filter(
      (r: any) => r.ftp_password || r.db_password
    )
    if (withCreds.length) {
      batchCredentials.value = withCreds
      batchCredDialog.value = true
    }
    resetForms()
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.createFailed')))
  } finally {
    submitting.value = false
  }
}

async function confirmSiteRemoval(message: string): Promise<boolean> {
  const phrase = t('websites.deleteConfirmPhrase')
  try {
    const { value } = await ElMessageBox.prompt(message, t('websites.deleteConfirmTitle'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      inputPlaceholder: phrase,
      inputValidator: (val) => val?.trim() === phrase || t('websites.deleteConfirmInputError'),
      type: 'warning',
    })
    return value?.trim() === phrase
  } catch {
    return false
  }
}

async function handleDelete(row: any) {
  const ok = await confirmSiteRemoval(t('websites.deleteConfirmPrompt', { domain: row.domain }))
  if (!ok) return
  if (row.source === 'website') {
    await api.delete(`/websites/${row.id}`)
  } else if (row.source === 'node') {
    await api.delete(`/nodejs/${row.id}`)
  } else if (row.source === 'java') {
    await api.delete(`/java/${row.id}`)
  }
  ElMessage.success(t('websites.deleted'))
  load()
}

async function batchDelete() {
  const websiteIds = selectedRows.value.filter((r) => r.source === 'website').map((r) => r.id)
  if (!websiteIds.length) {
    ElMessage.warning(t('websites.batchDeleteEmpty'))
    return
  }
  const ok = await confirmSiteRemoval(t('websites.batchDeleteConfirmPrompt', { n: websiteIds.length }))
  if (!ok) return
  await api.post('/websites/batch-delete', { ids: websiteIds })
  ElMessage.success(t('websites.deleted'))
  selectedRows.value = []
  load()
}

async function toggleSite(row: any) {
  const next = row.status === 'running' ? 'stopped' : 'running'
  if (row.source === 'node') {
    await api.patch(`/nodejs/${row.id}/toggle`, { status: next })
  } else if (row.source === 'java') {
    await api.patch(`/java/${row.id}/toggle`, { status: next })
  } else if (row.source === 'website') {
    await api.post(`/websites/${row.id}/toggle`, { status: next })
  }
  ElMessage.success(t('websites.statusUpdated'))
  load()
}

async function switchWebServer(key: string) {
  if (webserver.value?.active === key) return
  const target = (webserver.value?.servers || []).find((s: any) => s.key === key)
  if (!target?.installed) {
    ElMessage.warning(t('websites.webServerNotInstalled', { name: target?.name || key }))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('websites.webServerSwitchConfirm', { name: target.name }),
      t('common.confirm'),
      { type: 'warning' }
    )
  } catch {
    return
  }
  switchingWebServer.value = true
  try {
    const res: any = await api.post(`/websites/webserver/${key}/start`)
    webserver.value = res.data
    ElMessage.success(t('websites.webServerSwitched', { name: target.name }))
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.webServerSwitchFailed')))
  } finally {
    switchingWebServer.value = false
  }
}

async function openPathPicker() {
  pathPickerVisible.value = true
  browsePath.value = form.value.root_path
  await loadPathDir(browsePath.value)
}

async function loadPathDir(path: string) {
  try {
    const res: any = await api.get('/files', { params: { path } })
    pathEntries.value = res.data || []
    browsePath.value = path
  } catch {
    pathEntries.value = []
  }
}

function selectPath(path: string) {
  form.value.root_path = path
  pathPickerVisible.value = false
}

function openSiteModify(row: any, menu = 'domain') {
  if (row.source !== 'website') {
    router.push('/nodejs')
    return
  }
  siteModifyId.value = row.id
  siteModifyMenu.value = menu
  siteModifyVisible.value = true
}

function openSiteSSL(row: any) {
  if (row.source !== 'website') {
    ElMessage.info(t('websites.sslWebsiteOnly'))
    return
  }
  openSiteModify(row, 'ssl')
}

function openSiteBackup(row: any) {
  if (row.source !== 'website') {
    ElMessage.info(t('websites.backupWebsiteOnly'))
    return
  }
  siteBackupId.value = row.id
  siteBackupDomain.value = row.domain
  siteBackupVisible.value = true
}

function openSiteFiles(row: any) {
  const p = row.root_path || row.path
  if (!p) return
  router.push({ path: '/files', query: { path: p } })
}

async function openFileManager(row: any) {
  const dir = row.root_path || row.path
  if (!dir) {
    ElMessage.warning(t('websites.noRootPath'))
    return
  }
  currentSite.value = row
  fileDrawer.value = true
  editingFile.value = ''
  fileContent.value = ''
  await loadSiteFiles(dir)
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

async function repairSite(row: any) {
  if (row.source !== 'website') {
    ElMessage.info(t('websites.repairWebsiteOnly'))
    return
  }
  try {
    await api.post(`/websites/${row.id}/apply`)
    ElMessage.success(t('wp.repaired'))
    load()
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  }
}

async function startAIDiagnoseRepair(row: any) {
  if (row.source !== 'website') {
    ElMessage.info(t('websites.repairWebsiteOnly'))
    return
  }
  try {
    const stRes: any = await api.get('/ai/assistant/status')
    const st = stRes.data
    if (!st?.configured) {
      try {
        await ElMessageBox.confirm(
          st?.message || t('websites.aiRepairNotConfigured'),
          t('websites.aiRepairNeedConfig'),
          {
            confirmButtonText: t('aiHub.openSettings'),
            cancelButtonText: t('common.cancel'),
            type: 'warning',
          },
        )
        router.push('/settings')
      } catch {
        /* user cancelled */
      }
      return
    }
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
    return
  }

  aiRepairSiteDomain.value = row.domain || ''
  aiRepairLogs.value = []
  aiRepairSummary.value = ''
  aiRepairDiagnosis.value = ''
  aiRepairFixed.value = null
  aiRepairDialogVisible.value = true
  aiRepairLoading.value = true

  try {
    const res: any = await api.post(`/websites/${row.id}/ai/diagnose-repair`, {}, { timeout: AI_REQUEST_TIMEOUT })
    const data = res.data || {}
    aiRepairLogs.value = data.logs || []
    aiRepairSummary.value = data.summary || ''
    aiRepairDiagnosis.value = data.diagnosis || ''
    aiRepairFixed.value = !!data.fixed
    if (data.fixed) {
      ElMessage.success(t('websites.aiRepairSuccess'))
      load()
    } else if ((data.failed_actions?.length || 0) > 0) {
      ElMessage.warning(t('websites.aiRepairPartial'))
      load()
    } else {
      ElMessage.warning(t('websites.aiRepairPartial'))
    }
  } catch (e: any) {
    aiRepairLogs.value.push(resolveApiError(e, t('websites.aiRepairFailed'), t('common.requestTimeout')))
    aiRepairFixed.value = false
    ElMessage.error(resolveApiError(e, t('websites.aiRepairFailed'), t('common.requestTimeout')))
  } finally {
    aiRepairLoading.value = false
  }
}

function canUseSiteFiles(row: any) {
  return !!(row.root_path || row.path)
}

function onSelectionChange(rows: any[]) {
  selectedRows.value = rows
}

function sslLabel(row: any) {
  if (row.ssl_status === 'active' || row.ssl) return t('websites.sslOn')
  return t('websites.sslOff')
}

function backupLabel(row: any) {
  if (row.backup_status && row.backup_status !== 'none') return row.backup_status
  return t('websites.backupNone')
}

function backupTagType(row: any) {
  if (row.backup_status && row.backup_status !== 'none') return 'success'
  return 'warning'
}

onMounted(() => {
  load()
  bindActionsColLayoutObserver()
  scheduleActionsColWidthSync()
  window.addEventListener('resize', scheduleActionsColWidthSync)
})
onUnmounted(() => {
  stopAIJobPoll()
  actionsLayoutObserver?.disconnect()
  window.removeEventListener('resize', scheduleActionsColWidthSync)
  cancelAnimationFrame(actionsWidthRaf)
})

async function handleCreateJava() {
  if (!javaForm.value.domain.trim()) {
    ElMessage.warning(t('websites.domainRequired'))
    return
  }
  submitting.value = true
  try {
    await api.post('/java', javaForm.value)
    ElMessage.success(t('websites.javaCreated'))
    javaDialogVisible.value = false
    javaForm.value = { name: '', domain: '', path: '', port: 8080, java_ver: '17', tomcat_key: 'tomcat9', context_path: '/', remark: '' }
    load()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('websites.createFailed')))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="websites-page">
    <el-tabs v-model="projectTab" class="project-tabs">
      <el-tab-pane :label="t('websites.tabPhp')" name="php" />
      <el-tab-pane :label="t('websites.tabNode')" name="node" />
      <el-tab-pane :label="t('websites.tabJava')" name="java" />
    </el-tabs>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-button type="success" @click="openDialog">{{ t('websites.addSite') }}</el-button>
        <el-dropdown trigger="click">
          <el-button>{{ t('websites.batchOps') }}</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item :disabled="!selectedRows.length" @click="batchDelete">
                {{ t('websites.batchDelete') }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <div v-if="activeServer" class="webserver-badge" :class="{ running: activeServer.status === 'running' }">
          <span class="dot" />
          <span>{{ activeServer.name }} {{ activeServer.version }}</span>
          <el-dropdown v-if="otherServer" trigger="click">
            <el-button text size="small" :loading="switchingWebServer">{{ t('websites.switchWebServer') }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="switchWebServer(otherServer.key)">
                  {{ t('websites.switchTo', { name: otherServer.name }) }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
      <el-input
        v-model="searchText"
        class="search-input"
        clearable
        :placeholder="t('websites.searchPlaceholder')"
        @keyup.enter="loadProjects"
        @clear="loadProjects"
      >
        <template #append>
          <el-button :icon="Search" @click="loadProjects" />
        </template>
      </el-input>
    </div>

    <el-table ref="sitesTableRef" :data="projects" stripe class="sites-table" @selection-change="onSelectionChange">
      <el-table-column type="selection" width="46" />
      <el-table-column :label="t('websites.domain')" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">
          <el-tooltip :content="row.domain" placement="top">
            <el-button type="primary" link class="domain-link" @click="openSiteModify(row)">{{ row.domain }}</el-button>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.status')" width="90" align="center">
        <template #default="{ row }">
          <el-button
            circle
            size="small"
            :type="row.status === 'running' ? 'success' : 'info'"
            :icon="row.status === 'running' ? VideoPlay : VideoPause"
            @click="toggleSite(row)"
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.backup')" width="90" align="center">
        <template #default="{ row }">
          <el-tag
            v-if="row.source === 'website'"
            size="small"
            class="backup-tag-clickable"
            :type="backupTagType(row)"
            effect="plain"
            :title="t('websites.backupClickHint')"
            @click="openSiteBackup(row)"
          >
            {{ backupLabel(row) }}
          </el-tag>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.rootPath')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <span
            v-if="row.root_path"
            class="site-path-link"
            :title="row.root_path"
            @click="openSiteFiles(row)"
          >
            <el-icon class="site-path-icon"><Folder /></el-icon>
            <span class="site-path-text">{{ row.root_path }}</span>
          </span>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.security')" width="70" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.security" size="small" type="success">WAF</el-tag>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.expires')" width="110">
        <template #default="{ row }">
          <span
            class="expires-link"
            :class="{ expired: isSiteExpired(row), disabled: row.source !== 'website' }"
            :title="row.source === 'website' ? t('websites.expiresClickHint') : ''"
            @click="openExpiresEdit(row)"
          >
            {{ row.expires_label || t('websites.permanent') }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.remark')" min-width="80" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="remark-link" :title="row.remark || t('common.remarkClickHint')" @click="openRemarkEdit(row)">
            {{ row.remark || '—' }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="projectTab === 'node' ? 'Node' : t('websites.phpVersion')" width="128" align="center">
        <template #default="{ row }">
          <template v-if="projectTab === 'node'">
            {{ row.node_version || '—' }}
          </template>
          <template v-else-if="row.source === 'website'">
            <el-dropdown
              trigger="click"
              :disabled="phpVersionLoadingId === row.id || phpAccelLoadingId === row.id"
              @command="(cmd: any) => onPhpMenuCommand(row, cmd)"
            >
              <el-tag
                class="php-version-tag"
                :type="row.php_accel_enabled ? 'success' : 'info'"
                effect="plain"
                :title="t('websites.phpVersionClickHint')"
              >
                <span v-if="phpVersionLoadingId === row.id || phpAccelLoadingId === row.id">…</span>
                <span v-else class="php-version-tag-inner">
                  <span>{{ row.php_version || '—' }}</span>
                  <el-icon class="php-version-caret"><ArrowDown /></el-icon>
                </span>
              </el-tag>
              <template #dropdown>
                <el-dropdown-menu class="php-version-menu">
                  <el-dropdown-item
                    v-for="p in options.php_versions || []"
                    :key="p.value"
                    :command="{ action: 'version', value: p.value, installed: p.installed, key: p.key }"
                    :class="{ 'is-active': row.php_version_value === p.value, 'is-uninstalled': p.value !== 'static' && p.installed === false }"
                  >
                    <span>{{ p.label }}</span>
                    <el-tag v-if="p.value !== 'static' && p.installed === false" size="small" type="info">{{ t('websites.phpNotInstalled') }}</el-tag>
                    <el-icon v-else-if="row.php_version_value === p.value" style="margin-left: 6px"><Check /></el-icon>
                  </el-dropdown-item>
                  <el-dropdown-item
                    divided
                    :command="{ action: 'accel' }"
                    :disabled="!row.php_enabled && row.php_version_value === 'static'"
                  >
                    <span class="php-accel-menu-item">
                      <span>{{ row.php_accel_enabled ? t('websites.phpAccelEnabled') : t('websites.phpAccelMenu') }}</span>
                      <el-tag v-if="row.php_accel_enabled" size="small" type="success">{{ t('common.enabled') }}</el-tag>
                    </span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
          <span v-else class="muted">{{ row.php_version || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.ssl')" width="90" align="center">
        <template #default="{ row }">
          <el-tag
            v-if="row.source === 'website'"
            size="small"
            class="ssl-tag-clickable"
            :type="row.ssl_status === 'active' || row.ssl ? 'success' : 'danger'"
            effect="plain"
            :title="t('websites.sslClickHint')"
            @click="openSiteSSL(row)"
          >
            {{ sslLabel(row) }}
          </el-tag>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('websites.traffic')" width="96" align="center">
        <template #default="{ row }">
          <div class="traffic-cell">
            <div class="traffic-line" :title="t('websites.trafficToday')">{{ formatBytes(row.traffic_today || 0) }}</div>
            <div class="traffic-line traffic-total" :title="t('websites.trafficTotal')">{{ formatBytes(row.traffic || 0) }}</div>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" :width="actionsColWidth" :min-width="ACTIONS_COL_MIN" fixed="right" align="center" class-name="site-actions-cell" label-class-name="site-actions-cell">
        <template #default="{ row }">
          <div class="site-actions">
            <div class="site-actions-group site-actions-group--danger">
              <el-tooltip :content="t('common.delete')" placement="top">
                <span class="site-op-btn site-op-btn--danger">
                  <el-button circle size="small" :icon="Delete" @click="handleDelete(row)" />
                </span>
              </el-tooltip>
            </div>
            <span class="site-actions-sep" aria-hidden="true" />
            <div class="site-actions-group">
              <el-tooltip :content="row.cross_site_protect_enabled ? t('websites.crossSiteEnabled') : t('websites.crossSiteHint')" placement="top">
                <span
                  class="site-op-btn"
                  :class="row.cross_site_protect_enabled ? 'site-op-btn--success' : 'site-op-btn--warn'"
                >
                  <el-button
                    circle
                    size="small"
                    :icon="Lock"
                    :disabled="row.source !== 'website'"
                    :loading="crossSiteLoadingId === row.id"
                    @click="toggleCrossSiteProtect(row)"
                  />
                </span>
              </el-tooltip>
              <el-tooltip :content="siteCacheActive(row) ? t('websites.accelEnabled') : t('websites.accelHint')" placement="top">
                <span class="site-op-btn" :class="siteCacheActive(row) ? 'site-op-btn--success' : 'site-op-btn--primary'">
                  <el-button
                    circle
                    size="small"
                    :icon="Odometer"
                    :disabled="row.source !== 'website'"
                    :loading="cacheAccelLoadingId === row.id"
                    @click="autoEnableSiteCache(row)"
                  />
                </span>
              </el-tooltip>
            </div>
            <span class="site-actions-sep" aria-hidden="true" />
            <div class="site-actions-group">
              <el-tooltip :content="t('siteModify.title')" placement="top">
                <span class="site-op-btn site-op-btn--primary">
                  <el-button circle size="small" :icon="Setting" :disabled="row.source !== 'website'" @click="openSiteModify(row)" />
                </span>
              </el-tooltip>
              <el-tooltip :content="t('wp.editFiles')" placement="top">
                <span class="site-op-btn site-op-btn--primary">
                  <el-button circle size="small" :icon="EditPen" :disabled="!canUseSiteFiles(row)" @click="openFileManager(row)" />
                </span>
              </el-tooltip>
              <el-tooltip :content="t('wp.openInFiles')" placement="top">
                <span class="site-op-btn site-op-btn--neutral">
                  <el-button circle size="small" :icon="FolderOpened" :disabled="!canUseSiteFiles(row)" @click="openSiteFiles(row)" />
                </span>
              </el-tooltip>
            </div>
            <span class="site-actions-sep" aria-hidden="true" />
            <div class="site-actions-group">
              <el-tooltip :content="t('wp.repair')" placement="top">
                <span class="site-op-btn site-op-btn--warn">
                  <el-button circle size="small" :icon="RefreshRight" :disabled="row.source !== 'website'" @click="repairSite(row)" />
                </span>
              </el-tooltip>
              <el-tooltip :content="t('websites.aiRepair')" placement="top">
                <span class="site-op-btn site-op-btn--ai">
                  <el-button
                    circle
                    size="small"
                    :icon="MagicStick"
                    :disabled="row.source !== 'website'"
                    :loading="aiRepairLoading && aiRepairSiteDomain === row.domain"
                    @click="startAIDiagnoseRepair(row)"
                  />
                </span>
              </el-tooltip>
              <el-tooltip :content="t('wpBackup.runNow')" placement="top">
                <span class="site-op-btn site-op-btn--success">
                  <el-button circle size="small" :icon="Download" :disabled="row.source !== 'website'" @click="openSiteBackup(row)" />
                </span>
              </el-tooltip>
            </div>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="projectTab === 'java' && !projects.length" :description="t('websites.javaEmpty')" />

    <el-dialog v-model="javaDialogVisible" :title="t('websites.addJavaSite')" width="520px">
      <el-form label-width="100px">
        <el-form-item :label="t('websites.domain')" required>
          <el-input v-model="javaForm.domain" placeholder="app.example.com" />
        </el-form-item>
        <el-form-item :label="t('websites.rootPath')">
          <el-input v-model="javaForm.path" placeholder="/opt/owpanel/data/wwwroot/app.example.com" />
        </el-form-item>
        <el-form-item label="Tomcat">
          <el-select v-model="javaForm.tomcat_key" style="width: 100%">
            <el-option label="Tomcat 9" value="tomcat9" />
            <el-option label="Tomcat 10" value="tomcat10" />
          </el-select>
        </el-form-item>
        <el-form-item label="JDK">
          <el-select v-model="javaForm.java_ver" style="width: 100%">
            <el-option label="JDK 21" value="21" />
            <el-option label="JDK 17" value="17" />
            <el-option label="JDK 11" value="11" />
            <el-option label="JDK 8" value="8" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('websites.port')">
          <el-input-number v-model="javaForm.port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="Context">
          <el-input v-model="javaForm.context_path" placeholder="/" />
        </el-form-item>
        <el-form-item :label="t('websites.remark')">
          <el-input v-model="javaForm.remark" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="javaDialogVisible = false">{{ t('websites.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="handleCreateJava">{{ t('websites.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dialogVisible" :title="t('websites.addSiteTitle')" :width="createTab === 'aiGithub' ? '820px' : '640px'" destroy-on-close class="add-site-dialog">
      <el-tabs v-model="createTab" class="add-site-tabs">
        <el-tab-pane name="aiGithub">
          <template #label>
            <span class="ai-tab-label" :class="{ 'is-active': createTab === 'aiGithub' }">
              <el-icon class="ai-tab-icon"><MagicStick /></el-icon>
              <span>{{ t('websites.tabAIGithub') }}</span>
              <el-tag size="small" effect="dark" class="ai-tab-badge">{{ t('websites.tabAIGithubBadge') }}</el-tag>
            </span>
          </template>

          <div class="ai-github-hero">
            <div class="ai-github-hero-glow" aria-hidden="true" />
            <div class="ai-github-hero-icon">
              <el-icon><MagicStick /></el-icon>
            </div>
            <div class="ai-github-hero-text">
              <h3>{{ t('websites.aiGithubHeroTitle') }}</h3>
              <p>{{ t('websites.aiGithubHeroDescWizard') }}</p>
              <p class="ai-supported-fw">{{ t('websites.aiSupportedFrameworks') }}</p>
            </div>
          </div>

          <el-steps :active="aiWizardStep" finish-status="success" align-center class="ai-wizard-steps">
            <el-step :title="t('websites.aiWizardStepAnalyze')" />
            <el-step :title="t('websites.aiWizardStepEnv')" />
            <el-step :title="t('websites.aiWizardStepDomain')" />
            <el-step :title="t('websites.aiWizardStepDeploy')" />
          </el-steps>

          <el-form label-width="110px" class="site-form ai-github-form">
            <el-form-item :label="t('websites.aiRepoUrl')" required>
              <el-input
                v-model="aiForm.repo_url"
                placeholder="https://github.com/user/repo"
                size="large"
                class="ai-repo-input"
                @keyup.enter="handleAIWizardAnalyze"
              >
                <template #prefix>
                  <span class="github-prefix">GitHub</span>
                </template>
              </el-input>
            </el-form-item>

            <el-collapse v-model="aiOptionalOpen" class="ai-optional-collapse">
              <el-collapse-item :title="t('websites.aiOptionalFields')" name="optional">
                <el-form-item :label="t('websites.aiBranch')">
                  <el-input v-model="aiForm.branch" placeholder="main" />
                </el-form-item>
                <el-form-item :label="t('websites.aiGithubToken')">
                  <el-input v-model="aiForm.github_token" type="password" show-password :placeholder="t('websites.aiGithubTokenHint')" />
                </el-form-item>
                <el-form-item :label="t('websites.aiNotes')">
                  <el-input v-model="aiForm.notes" type="textarea" :rows="2" :placeholder="t('websites.aiNotesHint')" />
                </el-form-item>
              </el-collapse-item>
            </el-collapse>
          </el-form>

          <div v-if="aiAutoRunning || aiAnalyzing || aiDeploying" class="ai-progress-banner">
            <el-icon class="is-loading"><RefreshRight /></el-icon>
            <div class="ai-progress-body">
              <span>{{ aiProgressLabel }}</span>
              <p v-if="aiProgressDetail && !aiJob?.log" class="ai-progress-detail">{{ aiProgressDetail }}</p>
            </div>
          </div>

          <div v-if="aiShowDeployLog" class="ai-pipeline">
            <div class="ai-pipeline-steps">
              <button
                v-for="step in aiPipelineSteps"
                :key="step.phase"
                type="button"
                class="ai-pipeline-step"
                :class="{
                  'is-active': aiPipelineActive === step.phase,
                  [`is-${step.status}`]: true,
                }"
                @click="aiPipelineActive = step.phase"
              >
                <span class="ai-pipeline-step-name">{{ aiPipelinePhaseLabel(step.phase) }}</span>
                <el-tag size="small" :type="aiPipelineTagType(step.status)">
                  {{ aiPipelineStatusLabel(step.status) }}
                </el-tag>
              </button>
            </div>
          </div>

          <div v-if="aiShowDeployLog" class="ai-job-log ai-job-log--live">
            <div class="ai-job-log-head">
              <span class="ai-job-log-title">
                {{ t('websites.aiDeployLog') }} — {{ aiPipelinePhaseLabel(aiPipelineActive) }}
              </span>
              <el-tag
                v-if="aiJob?.status"
                :type="aiJob.status === 'success' ? 'success' : aiJob.status === 'failed' ? 'danger' : 'warning'"
                size="small"
              >
                {{ aiJob.status }}
              </el-tag>
              <el-tag v-else size="small" type="warning">running</el-tag>
            </div>
            <pre ref="aiLogPreRef" class="log-pre log-pre--live">{{ aiDeployLogText || t('websites.aiDeployWaiting') }}</pre>
          </div>

          <template v-if="aiPlan">
            <el-collapse v-model="aiAdvancedPlanOpen" class="ai-plan-collapse">
              <el-collapse-item :title="t('websites.aiAdvancedPlan')" name="plan">
                <p class="ai-summary">{{ aiPlanEdit.summary }}</p>
                <el-descriptions :column="2" border size="small" class="ai-meta">
                  <el-descriptions-item :label="t('websites.domain')">{{ aiPlanEdit.domain }}</el-descriptions-item>
                  <el-descriptions-item :label="t('websites.aiFramework')">{{ aiPlan.plan?.framework || aiPlan.repo?.framework_hint }}</el-descriptions-item>
                  <el-descriptions-item :label="t('websites.phpVersion')">{{ aiPlanEdit.php_version }}</el-descriptions-item>
                  <el-descriptions-item :label="t('websites.aiConfidence')">{{ aiPlan.plan?.confidence }}</el-descriptions-item>
                  <el-descriptions-item v-if="aiPlan.repo?.has_docker_compose || aiPlan.repo?.has_dockerfile" :label="t('websites.aiDocker')">
                    {{ aiPlan.repo?.has_docker_compose ? 'docker-compose' : 'Dockerfile' }}
                  </el-descriptions-item>
                  <el-descriptions-item v-if="aiPlan.repo?.has_node_server" :label="t('websites.aiNodeServer')">{{ t('websites.aiNodeServerYes') }}</el-descriptions-item>
                </el-descriptions>
                <el-form label-width="110px" class="site-form mt-12">
                  <el-form-item :label="t('websites.domain')" required :error="domainError">
                    <el-input v-model="aiPlanEdit.domain" />
                  </el-form-item>
                  <el-form-item :label="t('websites.phpVersion')">
                    <el-select v-model="aiPlanEdit.php_version" style="width: 100%">
                      <el-option v-for="p in options.php_versions || []" :key="p.value" :label="p.label" :value="p.value" />
                    </el-select>
                  </el-form-item>
                  <el-form-item :label="t('websites.aiDocumentRoot')">
                    <el-input v-model="aiPlanEdit.document_root" :placeholder="t('websites.aiDocumentRootHint')" />
                  </el-form-item>
                  <el-form-item :label="t('websites.database')">
                    <el-switch v-model="aiPlanEdit.need_database" :active-text="t('websites.aiNeedDb')" />
                  </el-form-item>
                  <el-form-item :label="t('websites.aiUseDocker')">
                    <el-switch v-model="aiPlanEdit.use_docker" />
                  </el-form-item>
                  <el-form-item :label="t('websites.aiUsePM2')">
                    <el-switch v-model="aiPlanEdit.use_pm2" />
                  </el-form-item>
                  <el-form-item v-if="aiPlanEdit.use_pm2" :label="t('websites.aiNodePort')">
                    <el-input-number v-model="aiPlanEdit.node_port" :min="1024" :max="65535" />
                  </el-form-item>
                  <el-form-item :label="t('websites.aiEnableWebhook')">
                    <el-switch v-model="aiPlanEdit.enable_webhook" :active-text="t('websites.aiEnableWebhookHint')" />
                  </el-form-item>
                  <el-form-item :label="t('websites.aiRollbackOnFail')">
                    <el-switch v-model="aiPlanEdit.rollback_on_fail" :active-text="t('websites.aiRollbackOnFailHint')" />
                  </el-form-item>
                  <el-form-item :label="t('websites.aiDeployScript')">
                    <el-input v-model="aiPlanEdit.deploy_script" type="textarea" :rows="6" />
                  </el-form-item>
                  <el-form-item v-if="aiPlanEdit.post_notes" :label="t('websites.aiPostNotes')">
                    <span class="post-notes">{{ aiPlanEdit.post_notes }}</span>
                    <p class="muted post-notes-hint">{{ t('websites.aiPostNotesAutoHint') }}</p>
                  </el-form-item>
                </el-form>
              </el-collapse-item>
            </el-collapse>
          </template>

        </el-tab-pane>

        <el-tab-pane :label="t('websites.tabCreate')" name="create">
          <el-form label-width="100px" class="site-form">
            <el-form-item :label="t('websites.dnsMode')">
              <el-radio-group v-model="form.dns_mode">
                <el-radio v-for="m in options.dns_modes || []" :key="m.value" :value="m.value">
                  {{ m.label }}
                </el-radio>
              </el-radio-group>
            </el-form-item>

            <el-form-item :label="t('websites.domain')" required :error="domainError">
              <el-input
                v-model="form.domains_text"
                type="textarea"
                :rows="5"
                :placeholder="`${t('websites.domainHint')}\n${t('websites.domainWildcard')}\n${t('websites.domainPort')}`"
              />
            </el-form-item>

            <el-form-item :label="t('websites.description')">
              <el-input v-model="form.description" />
            </el-form-item>

            <el-form-item :label="t('websites.rootPath')">
              <el-input v-model="form.root_path">
                <template #append>
                  <el-button :icon="Folder" @click="openPathPicker()">{{ t('websites.selectPath') }}</el-button>
                </template>
              </el-input>
            </el-form-item>

            <el-form-item :label="t('websites.ftp')">
              <el-select v-model="form.ftp" style="width: 100%">
                <el-option v-for="o in options.ftp_options || []" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('websites.database')">
              <el-select v-model="form.database" style="width: 100%">
                <el-option v-for="o in options.database_options || []" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('websites.phpVersion')">
              <el-select v-model="form.php_version" style="width: 100%">
                <el-option v-for="p in options.php_versions || []" :key="p.value" :label="p.label" :value="p.value" />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('websites.category')">
              <el-select v-model="form.category" style="width: 100%">
                <el-option v-for="c in categoryOptions" :key="c.value" :label="c.label" :value="c.value" />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('websites.expires')">
              <div class="expires-form">
                <el-checkbox v-model="form.expires_permanent">{{ t('websites.permanent') }}</el-checkbox>
                <el-date-picker
                  v-if="!form.expires_permanent"
                  v-model="form.expires_at"
                  type="date"
                  value-format="YYYY-MM-DD"
                  :placeholder="t('websites.expiresDatePlaceholder')"
                  style="width: 100%"
                />
              </div>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="t('websites.tabBatch')" name="batch">
          <div class="batch-panel">
            <el-input
              v-model="batchForm.batch_text"
              type="textarea"
              :rows="12"
              class="batch-textarea"
              :placeholder="t('websites.batchPlaceholder')"
            />
            <div class="batch-help">
              <p><strong>{{ t('websites.batchFormat') }}：</strong>{{ t('websites.batchFormatLine') }}</p>
              <p>{{ t('websites.batchDomainHint') }}</p>
              <p>{{ t('websites.batchFtpHint') }}</p>
              <p>{{ t('websites.batchDbHint') }}</p>
              <p>{{ t('websites.batchPhpHint') }}</p>
              <p class="batch-example">{{ t('websites.batchExample') }}</p>
            </div>
            <el-form label-width="100px" class="batch-extra">
              <el-form-item :label="t('websites.dnsMode')">
                <el-radio-group v-model="batchForm.dns_mode">
                  <el-radio v-for="m in options.dns_modes || []" :key="m.value" :value="m.value">
                    {{ m.label }}
                  </el-radio>
                </el-radio-group>
              </el-form-item>
              <el-form-item :label="t('websites.category')">
                <el-select v-model="batchForm.category" style="width: 240px">
                  <el-option v-for="c in categoryOptions" :key="c.value" :label="c.label" :value="c.value" />
                </el-select>
              </el-form-item>
            </el-form>
            <p v-if="domainError" class="batch-error">{{ domainError }}</p>
          </div>
        </el-tab-pane>
      </el-tabs>

      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('websites.cancel') }}</el-button>
        <template v-if="createTab === 'aiGithub'">
          <el-button v-if="aiPlan && !aiAutoRunning" text :loading="aiAnalyzing" @click="handleAIWizardAnalyze">
            {{ t('websites.aiReAnalyze') }}
          </el-button>
          <el-button
            v-if="!aiAutoRunning"
            type="primary"
            class="ai-auto-deploy-btn"
            :loading="aiAnalyzing"
            @click="handleAIWizardAnalyze"
          >
            <el-icon v-if="!aiAnalyzing" class="btn-icon"><MagicStick /></el-icon>
            {{ aiPlan ? t('websites.aiReAnalyze') : t('websites.aiWizardAnalyze') }}
          </el-button>
          <el-button
            v-if="aiPlan && !aiAutoRunning"
            type="primary"
            plain
            @click="aiEnvDialogVisible = true"
          >
            {{ t('websites.aiWizardContinueEnv') }}
          </el-button>
        </template>
        <el-button
          v-else
          type="primary"
          :loading="submitting"
          @click="createTab === 'create' ? handleCreate() : handleBatchCreate()"
        >
          {{ t('websites.create') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="aiEnvDialogVisible"
      :title="t('websites.aiEnvDialogTitle')"
      width="560px"
      :close-on-click-modal="false"
      append-to-body
    >
      <p v-if="aiPlanEdit.summary" class="ai-env-summary">{{ aiPlanEdit.summary }}</p>
      <p class="ai-env-hint">{{ t('websites.aiEnvDialogHint') }}</p>
      <div v-if="!aiEnvOptions.length" class="muted">{{ t('websites.aiEnvNoneNeeded') }}</div>
      <el-checkbox-group v-else v-model="aiSelectedApps" class="ai-env-list" @change="onEnvSelectionChange">
        <div v-for="item in aiEnvOptions" :key="item.key" class="ai-env-item">
          <el-checkbox
            :label="item.key"
            :disabled="item.required"
          >
            <span class="ai-env-label">{{ item.label }}</span>
            <el-tag v-if="item.installed" size="small" type="success">{{ t('websites.aiEnvInstalled') }}</el-tag>
            <el-tag v-else size="small" type="warning">{{ t('websites.aiEnvMissing') }}</el-tag>
            <span v-if="item.required" class="ai-env-required">{{ t('websites.aiEnvRequired') }}</span>
          </el-checkbox>
          <p v-if="item.description" class="ai-env-desc">{{ item.description }}</p>
        </div>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="aiEnvDialogVisible = false">{{ t('websites.cancel') }}</el-button>
        <el-button type="primary" @click="confirmEnvSelection">{{ t('websites.aiEnvConfirmNext') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="aiDomainDialogVisible"
      :title="t('websites.aiDomainDialogTitle')"
      width="520px"
      :close-on-click-modal="false"
      append-to-body
    >
      <p class="ai-env-hint">{{ t('websites.aiDomainDialogHint') }}</p>
      <el-form label-width="100px">
        <el-form-item :label="t('websites.domain')" required>
          <el-input v-model="aiDomainDraft.domain" :placeholder="t('websites.aiDomainHint')" />
        </el-form-item>
        <el-form-item :label="t('websites.dnsMode')">
          <el-radio-group v-model="aiDomainDraft.dns_mode">
            <el-radio v-for="m in options.dns_modes || []" :key="m.value" :value="m.value">
              {{ m.label }}
            </el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <p v-if="domainError" class="batch-error">{{ domainError }}</p>
      <template #footer>
        <el-button @click="aiDomainDialogVisible = false">{{ t('websites.cancel') }}</el-button>
        <el-button type="primary" :loading="aiDeploying" @click="startAIDeployFromWizard">
          {{ t('websites.aiWizardStartDeploy') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchCredDialog" :title="t('websites.batchCredentials')" width="560px">
      <div v-for="(item, idx) in batchCredentials" :key="idx" class="batch-cred-block">
        <h4>{{ item.site?.domain }}</h4>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item v-if="item.ftp_user" :label="t('websites.ftpUser')">
            {{ item.ftp_user }} / {{ item.ftp_password }}
          </el-descriptions-item>
          <el-descriptions-item v-if="item.db_name" :label="t('websites.dbName')">
            {{ item.db_name }} / {{ item.db_user }} / {{ item.db_password }}
          </el-descriptions-item>
        </el-descriptions>
      </div>
    </el-dialog>

    <el-dialog v-model="credDialog" :title="t('websites.credentials')" width="480px">
      <el-descriptions v-if="credentials" :column="1" border>
        <el-descriptions-item v-if="credentials.ftp_user" :label="t('websites.ftpUser')">
          {{ credentials.ftp_user }}
        </el-descriptions-item>
        <el-descriptions-item v-if="credentials.ftp_password" :label="t('websites.ftpPassword')">
          {{ credentials.ftp_password }}
        </el-descriptions-item>
        <el-descriptions-item v-if="credentials.db_name" :label="t('websites.dbName')">
          {{ credentials.db_name }}
        </el-descriptions-item>
        <el-descriptions-item v-if="credentials.db_user" :label="t('websites.dbUser')">
          {{ credentials.db_user }}
        </el-descriptions-item>
        <el-descriptions-item v-if="credentials.db_password" :label="t('websites.dbPassword')">
          {{ credentials.db_password }}
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <el-dialog v-model="pathPickerVisible" :title="t('websites.selectPath')" width="520px">
      <div class="path-bar">{{ browsePath }}</div>
      <el-table :data="pathEntries" stripe max-height="320" @row-dblclick="(row: any) => row.is_dir && loadPathDir(row.path)">
        <el-table-column prop="name" label="Name" />
        <el-table-column width="100">
          <template #default="{ row }">
            <el-button v-if="row.is_dir" text type="primary" @click="loadPathDir(row.path)">Open</el-button>
            <el-button v-else text type="success" @click="selectPath(row.path)">Select</el-button>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button type="primary" @click="selectPath(browsePath)">{{ t('websites.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remarkDialogVisible" :title="t('common.editRemark')" width="480px">
      <el-input v-model="remarkEditValue" type="textarea" :rows="3" :placeholder="t('common.remarkPlaceholder')" />
      <template #footer>
        <el-button @click="remarkDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="remarkSaving" @click="saveSiteRemarkEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="expiresDialogVisible" :title="t('websites.editExpires')" width="420px">
      <el-form label-width="96px">
        <el-form-item :label="t('websites.expires')">
          <el-checkbox v-model="expiresPermanent">{{ t('websites.permanent') }}</el-checkbox>
        </el-form-item>
        <el-form-item v-if="!expiresPermanent" :label="t('websites.expiresAt')">
          <el-date-picker
            v-model="expiresDate"
            type="date"
            value-format="YYYY-MM-DD"
            :placeholder="t('websites.expiresDatePlaceholder')"
            style="width: 100%"
          />
        </el-form-item>
        <p v-if="!expiresPermanent" class="expires-hint">{{ t('websites.expiresAutoStopHint') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="expiresDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="expiresSaving" @click="saveSiteExpiresEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="aiRepairDialogVisible"
      :title="t('websites.aiRepairTitle')"
      width="560px"
      :close-on-click-modal="!aiRepairLoading"
      :close-on-press-escape="!aiRepairLoading"
      :show-close="!aiRepairLoading"
    >
      <div v-loading="aiRepairLoading" :element-loading-text="t('websites.aiRepairRunning')">
        <p v-if="aiRepairSiteDomain" class="ai-repair-domain">{{ aiRepairSiteDomain }}</p>
        <template v-if="aiRepairSummary">
          <p class="ai-repair-label">{{ t('websites.aiRepairSummary') }}</p>
          <p class="ai-repair-text">{{ aiRepairSummary }}</p>
        </template>
        <template v-if="aiRepairDiagnosis">
          <p class="ai-repair-label">{{ t('websites.aiRepairDiagnosis') }}</p>
          <p class="ai-repair-text">{{ aiRepairDiagnosis }}</p>
        </template>
        <el-tag
          v-if="aiRepairFixed !== null"
          :type="aiRepairFixed ? 'success' : 'warning'"
          size="small"
          class="ai-repair-status"
        >
          {{ aiRepairFixed ? t('websites.aiRepairSuccess') : t('websites.aiRepairPartial') }}
        </el-tag>
        <pre v-if="aiRepairLogs.length" class="log-pre ai-repair-log">{{ aiRepairLogs.join('\n') }}</pre>
        <p v-else-if="!aiRepairLoading" class="ai-repair-empty">{{ t('websites.aiRepairRunning') }}</p>
      </div>
      <template #footer>
        <el-button :disabled="aiRepairLoading" @click="aiRepairDialogVisible = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>

    <SiteModifyDialog
      v-model:visible="siteModifyVisible"
      :site-id="siteModifyId"
      :initial-menu="siteModifyMenu"
      @updated="load"
    />
    <SiteBackupDialog
      v-model:visible="siteBackupVisible"
      :site-id="siteBackupId"
      :domain="siteBackupDomain"
      @updated="load"
    />

    <el-drawer v-model="fileDrawer" :title="`${t('wp.editFiles')} — ${currentSite?.domain || ''}`" size="60%">
      <div v-if="currentSite" class="file-drawer">
        <el-table
          :data="fileEntries"
          stripe
          max-height="280"
          @row-dblclick="(row: any) => row.is_dir ? loadSiteFiles(row.path) : openSiteFile(row.path)"
        >
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
  </div>
</template>

<style scoped>
.websites-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.project-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.webserver-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 13px;
}
.webserver-badge .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--el-color-info);
}
.webserver-badge.running .dot {
  background: var(--el-color-success);
}
.search-input {
  width: 260px;
}
.muted {
  color: var(--el-text-color-secondary);
}
.ssl-tag-clickable {
  cursor: pointer;
  transition: opacity 0.15s;
}
.ssl-tag-clickable:hover {
  opacity: 0.85;
}
.backup-tag-clickable {
  cursor: pointer;
  transition: opacity 0.15s;
}
.backup-tag-clickable:hover {
  opacity: 0.85;
}
.site-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 10px;
  width: 100%;
  box-sizing: border-box;
  border-radius: var(--apple-radius-sm, 10px);
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--apple-glass-border, rgba(0, 0, 0, 0.06));
  box-shadow: var(--apple-shadow-sm, 0 1px 2px rgba(0, 0, 0, 0.04));
}
.websites-page :deep(.site-actions-cell) {
  vertical-align: middle;
}
.websites-page :deep(.site-actions-cell .cell) {
  overflow: visible;
  padding: 6px 8px;
  line-height: normal;
  white-space: normal;
}
.websites-page :deep(.el-table__fixed-right .site-actions-cell .cell) {
  overflow: visible;
}
.site-actions-group {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}
.site-actions-sep {
  display: none;
}
.site-op-btn {
  display: inline-flex;
  line-height: 0;
}
.site-op-btn :deep(.el-button) {
  width: 28px;
  height: 28px;
  padding: 0;
  margin: 0;
  border: none;
  font-size: 15px;
  transition:
    transform 0.22s var(--apple-ease, ease),
    box-shadow 0.22s var(--apple-ease, ease),
    background 0.22s var(--apple-ease, ease),
    color 0.22s var(--apple-ease, ease);
}
.site-op-btn :deep(.el-button:not(.is-disabled):hover) {
  transform: translateY(-1px);
}
.site-op-btn :deep(.el-button.is-disabled) {
  opacity: 0.38;
  background: var(--el-fill-color-light) !important;
  color: var(--el-text-color-placeholder) !important;
}
.site-op-btn--neutral :deep(.el-button:not(.is-disabled)) {
  background: var(--el-bg-color);
  color: var(--el-text-color-regular);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}
.site-op-btn--neutral :deep(.el-button:not(.is-disabled):hover) {
  background: var(--el-fill-color);
  color: var(--el-color-primary);
  box-shadow: 0 3px 10px rgba(0, 0, 0, 0.08);
}
.site-op-btn--primary :deep(.el-button:not(.is-disabled)) {
  background: rgba(246, 130, 31, 0.1);
  color: var(--cf-orange, #f6821f);
  box-shadow: 0 1px 2px rgba(246, 130, 31, 0.12);
}
.site-op-btn--primary :deep(.el-button:not(.is-disabled):hover) {
  background: rgba(246, 130, 31, 0.18);
  color: var(--cf-orange, #f6821f);
  box-shadow: 0 4px 12px rgba(246, 130, 31, 0.2);
}
.site-op-btn--success :deep(.el-button:not(.is-disabled)) {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
  box-shadow: 0 1px 2px rgba(103, 194, 58, 0.12);
}
.site-op-btn--success :deep(.el-button:not(.is-disabled):hover) {
  background: var(--el-color-success-light-8);
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.18);
}
.site-op-btn--warn :deep(.el-button:not(.is-disabled)) {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
  box-shadow: 0 1px 2px rgba(230, 162, 60, 0.12);
}
.site-op-btn--warn :deep(.el-button:not(.is-disabled):hover) {
  background: var(--el-color-warning-light-8);
  box-shadow: 0 4px 12px rgba(230, 162, 60, 0.18);
}
.site-op-btn--ai :deep(.el-button:not(.is-disabled)) {
  background: linear-gradient(145deg, rgba(246, 130, 31, 0.22), rgba(246, 130, 31, 0.08));
  color: var(--cf-orange, #f6821f);
  box-shadow: 0 1px 3px rgba(246, 130, 31, 0.2);
}
.site-op-btn--ai :deep(.el-button:not(.is-disabled):hover) {
  background: linear-gradient(145deg, rgba(246, 130, 31, 0.32), rgba(246, 130, 31, 0.14));
  box-shadow: 0 4px 14px rgba(246, 130, 31, 0.28);
}
.site-op-btn--danger :deep(.el-button:not(.is-disabled)) {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger);
  box-shadow: 0 1px 2px rgba(245, 108, 108, 0.12);
}
.site-op-btn--danger :deep(.el-button:not(.is-disabled):hover) {
  background: var(--el-color-danger-light-8);
  box-shadow: 0 4px 12px rgba(245, 108, 108, 0.2);
}
.traffic-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  line-height: 1.25;
  font-size: 12px;
  white-space: nowrap;
}
.traffic-line.traffic-total {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}
.file-drawer {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.file-editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  font-size: 13px;
  color: #606266;
  word-break: break-all;
}
.site-path-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  min-width: 0;
  color: var(--el-color-primary);
  cursor: pointer;
}
.domain-link {
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}
.domain-link :deep(> span) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}
.remark-link {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
  color: var(--el-color-primary);
  cursor: pointer;
}
.site-path-link:hover .site-path-text {
  text-decoration: underline;
}
.remark-link:hover {
  text-decoration: underline;
}
.expires-link {
  cursor: pointer;
  color: var(--el-text-color-regular);
}
.expires-link:hover:not(.disabled) {
  color: var(--el-color-primary);
}
.expires-link.disabled {
  cursor: default;
}
.expires-link.expired {
  color: var(--el-color-danger);
}
.expires-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.expires-hint {
  margin: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
.site-path-icon {
  flex-shrink: 0;
}
.site-path-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.site-form {
  max-height: 52vh;
  overflow-y: auto;
  padding-right: 8px;
}
.path-bar {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  font-family: monospace;
  font-size: 13px;
  word-break: break-all;
}
.batch-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.batch-textarea :deep(textarea) {
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.6;
}
.batch-help {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.7;
  padding: 0 4px;
}
.batch-help p {
  margin: 0 0 4px;
}
.batch-example {
  color: var(--el-color-primary);
  margin-top: 8px !important;
}
.batch-extra {
  margin-top: 4px;
}
.batch-error {
  color: var(--el-color-danger);
  font-size: 13px;
  margin: 0;
}
.batch-cred-block {
  margin-bottom: 16px;
}
.batch-cred-block h4 {
  margin: 0 0 8px;
  font-size: 14px;
}
.ai-github-hint { margin-bottom: 16px; }
.ai-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.ai-tab-label.is-active {
  color: var(--el-color-primary);
}
.ai-tab-icon {
  font-size: 16px;
  color: #7c3aed;
}
.ai-tab-badge {
  border: none;
  background: linear-gradient(135deg, #7c3aed, #2563eb);
  color: #fff;
  font-size: 11px;
  height: 18px;
  padding: 0 6px;
}
.add-site-tabs :deep(.el-tabs__item.is-active .ai-tab-label) {
  color: var(--el-color-primary);
}
.ai-github-hero {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 18px 20px;
  margin-bottom: 20px;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(124, 58, 237, 0.12), rgba(37, 99, 235, 0.1));
  border: 1px solid rgba(124, 58, 237, 0.22);
  overflow: hidden;
}
.ai-github-hero-glow {
  position: absolute;
  top: -40px;
  right: -20px;
  width: 120px;
  height: 120px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(124, 58, 237, 0.25), transparent 70%);
  pointer-events: none;
}
.ai-github-hero-icon {
  flex-shrink: 0;
  width: 52px;
  height: 52px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 26px;
  color: #fff;
  background: linear-gradient(135deg, #7c3aed, #2563eb);
  box-shadow: 0 8px 20px rgba(124, 58, 237, 0.35);
}
.ai-github-hero-text h3 {
  margin: 0 0 6px;
  font-size: 17px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}
.ai-github-hero-text p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.ai-supported-fw {
  margin-top: 8px !important;
  font-size: 12px !important;
  opacity: 0.85;
}
.ai-github-form .ai-repo-input :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.25) inset;
}
.github-prefix {
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  padding-right: 4px;
}
.ai-optional-collapse,
.ai-plan-collapse {
  margin-bottom: 12px;
  border: none;
}
.ai-optional-collapse :deep(.el-collapse-item__header),
.ai-plan-collapse :deep(.el-collapse-item__header) {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  border: none;
  height: 40px;
}
.ai-progress-banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border-radius: 8px;
  background: rgba(124, 58, 237, 0.08);
  color: #7c3aed;
  font-size: 13px;
  font-weight: 500;
}
.ai-progress-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.ai-progress-detail {
  margin: 0;
  font-size: 12px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
  line-height: 1.45;
  word-break: break-word;
}
.ai-wizard-steps {
  margin: 16px 0 20px;
}
.ai-env-summary {
  margin: 0 0 8px;
  font-weight: 500;
}
.ai-env-hint {
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.ai-env-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
}
.ai-env-item {
  padding: 10px 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}
.ai-env-label {
  margin-right: 8px;
  font-weight: 500;
}
.ai-env-required {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-danger);
}
.ai-env-desc {
  margin: 6px 0 0 24px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.ai-pipeline {
  margin: 12px 0 0;
}
.ai-pipeline-steps {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}
.ai-pipeline-step {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-blank);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.ai-pipeline-step:hover,
.ai-pipeline-step.is-active {
  border-color: rgba(124, 58, 237, 0.45);
  box-shadow: 0 0 0 1px rgba(124, 58, 237, 0.12);
}
.ai-pipeline-step.is-running {
  background: rgba(124, 58, 237, 0.06);
}
.ai-pipeline-step.is-done {
  border-color: rgba(103, 194, 58, 0.35);
}
.ai-pipeline-step.is-failed {
  border-color: rgba(245, 108, 108, 0.45);
}
.ai-pipeline-step-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  line-height: 1.35;
}
.ai-job-log--inline {
  margin: 0 0 12px;
  max-height: 160px;
  overflow: auto;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  padding: 8px 10px;
}
.ai-job-log--live {
  margin: 12px 0 0;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-blank);
  overflow: hidden;
}
.ai-job-log-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-light);
}
.ai-job-log-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  flex: 1;
}
.log-pre--live {
  margin: 0;
  padding: 12px;
  max-height: 280px;
  min-height: 120px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: Consolas, 'Courier New', monospace;
  color: var(--el-text-color-regular);
  background: #1e1e1e;
  color: #d4d4d4;
}
.ai-job-log--inline .log-pre {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.ai-auto-deploy-btn {
  background: linear-gradient(135deg, #7c3aed, #2563eb) !important;
  border: none !important;
  font-weight: 600;
  padding: 10px 22px;
}
.ai-auto-deploy-btn .btn-icon {
  margin-right: 4px;
}
.ai-summary { margin: 0 0 12px; color: var(--el-text-color-regular); line-height: 1.6; }
.ai-meta { margin-bottom: 12px; }
.mt-12 { margin-top: 12px; }
.post-notes { color: var(--el-text-color-secondary); font-size: 13px; }
.ai-job-log { margin-top: 8px; }
.ai-repair-domain {
  margin: 0 0 12px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.ai-repair-label {
  margin: 0 0 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-regular);
}
.ai-repair-text {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.ai-repair-status { margin-bottom: 8px; }
.ai-repair-log { max-height: 280px; }
.ai-repair-empty {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.log-pre {
  margin-top: 8px;
  max-height: 200px;
  overflow: auto;
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 10px;
  border-radius: 6px;
  white-space: pre-wrap;
}
.php-version-tag {
  cursor: pointer;
  user-select: none;
  min-width: 82px;
  justify-content: center;
}
.php-version-tag-inner {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.php-version-caret {
  font-size: 12px;
  opacity: 0.75;
}
.php-version-tag:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}
.php-version-menu :deep(.el-dropdown-menu__item) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 148px;
}
.php-version-menu :deep(.el-dropdown-menu__item.is-uninstalled) {
  opacity: 0.85;
}
.php-accel-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 160px;
}
:deep(.el-dropdown-menu__item.is-active) {
  color: var(--el-color-primary);
  font-weight: 600;
}
</style>
