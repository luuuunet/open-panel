<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { categoryLabel } from '@/locales'
import SoftwareIcon from '@/components/SoftwareIcon.vue'
import { ElMessage } from 'element-plus'
import { ArrowRight, Bell, Refresh, Timer, Promotion, Share, FolderOpened, Lock, Document, Box, Histogram, Cpu, Coin, Platform, DataAnalysis } from '@element-plus/icons-vue'
import { cfTheme } from '@/config/theme'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const loading = ref(false)
const tab = ref('overview')
const status = ref<any>(null)
const overview = ref<any>(null)
const events = ref<any[]>([])
const selectedKeys = ref<string[]>([])
const eventFilterType = ref('')
const eventFilterApp = ref('')
let timer: ReturnType<typeof setInterval>

const configForm = ref({
  enabled: true,
  interval_sec: 30,
  cooldown_sec: 300,
  max_restarts: 5,
  notify_webhook: '',
  notify_on_down: true,
  notify_on_fail: true,
  resource_enabled: false,
  cpu_threshold: 90,
  mem_threshold: 90,
  disk_threshold: 90,
  ssl_auto_renew: true,
  alert_days_ssl: 14,
  alert_days_site: 14,
  website_scan_enabled: true,
})

const websiteAudits = ref<any>(null)
const auditDetail = ref<any>(null)
const auditDrawer = ref(false)
const scanningSite = ref<number | null>(null)
const scanningAll = ref(false)

const watches = computed(() => status.value?.watches || [])
const websiteAuditItems = computed(() => websiteAudits.value?.items || [])
const installedCount = computed(() => watches.value.length)

const quickLinks = computed(() => [
  { path: '/product-analytics', icon: DataAnalysis, title: t('menu.abTesting'), desc: t('autoOps.linkAbTesting'), stat: 'A/B' },
  { path: '/uptime', icon: Bell, title: t('menu.uptime'), desc: t('autoOps.linkUptime'), stat: overview.value ? `${(overview.value.uptime_total || 0) - (overview.value.uptime_down || 0)}/${overview.value.uptime_total || 0}` : '—' },
  { path: '/cron', icon: Timer, title: t('menu.cron'), desc: t('autoOps.linkCron'), stat: overview.value ? `${overview.value.cron_enabled || 0}/${overview.value.cron_total || 0}` : '—' },
  { path: '/backup', icon: FolderOpened, title: t('menu.backup'), desc: t('autoOps.linkBackup'), stat: overview.value ? `${overview.value.backup_enabled || 0}/${overview.value.backup_total || 0}` : '—' },
  { path: '/devops', icon: Promotion, title: t('menu.devops'), desc: t('autoOps.linkDevops'), stat: 'CI/CD' },
  { path: '/cluster', icon: Share, title: t('menu.cluster'), desc: t('autoOps.linkCluster'), stat: t('autoOps.multiNode') },
  { path: '/k8s', icon: Platform, title: t('menu.k8s'), desc: t('autoOps.linkK8s'), stat: overview.value?.k8s_ready ? t('k8s.ready') : (overview.value?.k8s_installed ? t('k8s.notReady') : '—') },
  { path: '/ssl', icon: Lock, title: t('menu.ssl'), desc: t('autoOps.linkSSL'), stat: overview.value?.ssl_expiring_soon ? t('autoOps.expiringCount', { n: overview.value.ssl_expiring_soon }) : '—' },
  { path: '/websites', icon: Bell, title: t('menu.website'), desc: t('autoOps.linkSites'), stat: overview.value?.sites_expiring_soon ? t('autoOps.expiringCount', { n: overview.value.sites_expiring_soon }) : '—' },
  { path: '/logs', icon: Document, title: t('menu.logs'), desc: t('autoOps.linkLogs'), stat: overview.value?.log_auto_cleanup ? t('autoOps.logCleanupOn') : t('autoOps.logCleanupOff') },
  { path: '/extensions', icon: Box, title: t('menu.extensions'), desc: t('autoOps.linkExtensions'), stat: t('autoOps.hooks') },
  { path: '/protection', icon: Histogram, title: t('menu.protection'), desc: t('autoOps.linkProtection'), stat: 'WAF' },
])

function liveTagType(s: string) {
  if (s === 'running') return 'success'
  if (s === 'stopped') return 'info'
  return 'warning'
}

function liveStatusLabel(s: string) {
  if (s === 'running') return t('common.running')
  if (s === 'stopped') return t('common.stopped')
  return s || '—'
}

function eventTagType(type: string) {
  if (type === 'restart_ok') return 'success'
  if (type === 'down_detected') return 'warning'
  if (type?.startsWith('resource_')) return 'danger'
  if (type === 'cron_failed') return 'danger'
  if (type === 'ssl_expiring' || type === 'site_expiring') return 'warning'
  if (type === 'ssl_renew_fail') return 'danger'
  if (type === 'ssl_renew_ok') return 'success'
  if (type === 'restart_fail' || type === 'restart_skipped') return 'danger'
  return 'info'
}

function eventLabel(type: string) {
  const map: Record<string, string> = {
    down_detected: t('autoOps.eventDown'),
    restart_ok: t('autoOps.eventRestartOk'),
    restart_fail: t('autoOps.eventRestartFail'),
    restart_skipped: t('autoOps.eventSkipped'),
    resource_cpu: t('autoOps.eventResourceCPU'),
    resource_memory: t('autoOps.eventResourceMem'),
    resource_disk: t('autoOps.eventResourceDisk'),
    cron_failed: t('autoOps.eventCronFailed'),
    ssl_expiring: t('autoOps.eventSSLExpiring'),
    site_expiring: t('autoOps.eventSiteExpiring'),
    ssl_renew_fail: t('autoOps.eventSSLRenewFail'),
    ssl_renew_ok: t('autoOps.eventSSLRenewOk'),
    website_issue: t('autoOps.eventWebsiteIssue'),
  }
  return map[type] || type
}

function gradeTagType(grade: string) {
  if (grade === 'A' || grade === 'B') return 'success'
  if (grade === 'C') return 'warning'
  return 'danger'
}

function severityTagType(sev: string) {
  if (sev === 'critical') return 'danger'
  if (sev === 'warning') return 'warning'
  return 'info'
}

function formatTime(v: string) {
  if (!v) return '—'
  return new Date(v).toLocaleString()
}

function resourceColor(pct: number) {
  if (pct >= 90) return cfTheme.danger
  if (pct >= 75) return cfTheme.warning
  return cfTheme.success
}

function resourcePercent(v: number | undefined | null) {
  const n = Math.min(100, Math.max(0, Number(v) || 0))
  return Math.round(n * 10) / 10
}

function resourceFormat(pct: number) {
  return `${resourcePercent(pct)}%`
}

async function loadWebsiteAudits() {
  try {
    const res: any = await api.get('/auto-ops/website-audits')
    websiteAudits.value = res.data
  } catch {
    /* optional */
  }
}

async function scanAllWebsites() {
  scanningAll.value = true
  try {
    const res: any = await api.post('/auto-ops/website-scan')
    websiteAudits.value = res.data
    ElMessage.success(t('autoOps.websiteScanDone'))
    await loadOverview()
    await loadEvents()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
  } finally {
    scanningAll.value = false
  }
}

async function scanOneWebsite(row: { site_id: number }) {
  scanningSite.value = row.site_id
  try {
    const res: any = await api.post(`/auto-ops/website-audits/${row.site_id}/scan`)
    await loadWebsiteAudits()
    auditDetail.value = res.data
    auditDrawer.value = true
    ElMessage.success(t('autoOps.websiteScanDone'))
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
  } finally {
    scanningSite.value = null
  }
}

async function openAuditDetail(row: { site_id: number }) {
  try {
    const res: any = await api.get(`/auto-ops/website-audits/${row.site_id}`)
    auditDetail.value = res.data
    auditDrawer.value = true
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
  }
}

async function load() {
  loading.value = true
  try {
    const res: any = await api.get('/auto-ops/status')
    status.value = res.data
    if (res.data?.config) {
      configForm.value = { ...configForm.value, ...res.data.config }
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
  } finally {
    loading.value = false
  }
}

async function loadOverview() {
  try {
    const res: any = await api.get('/auto-ops/overview')
    overview.value = res.data
    if (res.data?.status) status.value = res.data.status
  } catch {
    /* optional */
  }
}

async function loadEvents() {
  const params: Record<string, string> = { limit: '100' }
  if (eventFilterType.value) params.event_type = eventFilterType.value
  if (eventFilterApp.value) params.app_key = eventFilterApp.value
  const res: any = await api.get('/auto-ops/events', { params })
  events.value = res.data || []
}

async function saveConfig() {
  await api.put('/auto-ops/config', configForm.value)
  ElMessage.success(t('autoOps.configSaved'))
  load()
}

async function scanNow() {
  loading.value = true
  try {
    const res: any = await api.post('/auto-ops/scan')
    status.value = res.data
    ElMessage.success(t('autoOps.scanDone'))
    await loadOverview()
    await loadEvents()
    await loadWebsiteAudits()
  } finally {
    loading.value = false
  }
}

async function patchWatch(row: any, field: 'watch_enabled' | 'auto_restart', val: boolean) {
  const payload: Record<string, boolean> = { [field]: val }
  if (field === 'auto_restart' && val) payload.watch_enabled = true
  if (field === 'watch_enabled' && !val) payload.auto_restart = false
  try {
    await api.patch(`/auto-ops/watch/${row.key}`, payload)
    ElMessage.success(t('autoOps.bulkUpdated'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
    await load()
  }
}

async function bulkEnable(autoRestart: boolean) {
  if (!selectedKeys.value.length) return
  try {
    await api.post('/auto-ops/watch/bulk', {
      keys: selectedKeys.value,
      watch_enabled: true,
      auto_restart: autoRestart,
    })
    ElMessage.success(t('autoOps.bulkUpdated'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
  }
}

onMounted(() => {
  load()
  loadOverview()
  loadEvents()
  if (route.query.tab === 'watch') tab.value = 'watch'
  if (route.query.tab === 'events') tab.value = 'events'
  if (route.query.tab === 'settings') tab.value = 'settings'
  if (route.query.tab === 'websites') tab.value = 'websites'
  loadWebsiteAudits()
  timer = setInterval(() => {
    load()
    if (tab.value === 'overview') loadOverview()
    if (tab.value === 'events') loadEvents()
    if (tab.value === 'websites') loadWebsiteAudits()
  }, 15000)
})

onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="auto-ops-page" v-loading="loading">
    <div class="page-header">
      <div>
        <h2>{{ t('autoOps.title') }}</h2>
        <p class="hint">{{ t('autoOps.subtitle') }}</p>
        <p v-if="status?.last_scan" class="last-scan">{{ t('autoOps.lastScan') }}: {{ formatTime(status.last_scan) }}</p>
      </div>
      <div class="header-stats">
        <el-tag :type="status?.config?.enabled ? 'success' : 'info'">
          {{ status?.config?.enabled ? t('autoOps.enabled') : t('autoOps.disabled') }}
        </el-tag>
        <el-tag type="info">{{ t('autoOps.installedCount', { n: installedCount }) }}</el-tag>
        <el-tag type="warning">{{ t('autoOps.watching', { n: status?.watch_count ?? 0 }) }}</el-tag>
        <el-tag v-if="(status?.down_count ?? 0) > 0" type="danger">
          {{ t('autoOps.downCount', { n: status?.down_count }) }}
        </el-tag>
        <el-button type="primary" @click="scanNow">{{ t('autoOps.scanNow') }}</el-button>
      </div>
    </div>

    <el-tabs v-model="tab" @tab-change="(name: string) => { if (name === 'websites') loadWebsiteAudits() }">
      <el-tab-pane :label="t('autoOps.overview')" name="overview">
        <div class="overview-grid">
          <div class="resource-row">
            <el-card shadow="never" class="stat-card stat-card-resource">
              <div class="resource-gauge">
                <el-progress
                  type="dashboard"
                  :percentage="resourcePercent(overview?.cpu_percent)"
                  :color="resourceColor(overview?.cpu_percent || 0)"
                  :width="108"
                  :stroke-width="10"
                >
                  <template #default>
                    <span class="gauge-val">{{ resourceFormat(overview?.cpu_percent) }}</span>
                  </template>
                </el-progress>
                <div class="resource-label">
                  <el-icon><Cpu /></el-icon>
                  <span>CPU</span>
                </div>
              </div>
            </el-card>
            <el-card shadow="never" class="stat-card stat-card-resource">
              <div class="resource-gauge">
                <el-progress
                  type="dashboard"
                  :percentage="resourcePercent(overview?.memory_percent)"
                  :color="resourceColor(overview?.memory_percent || 0)"
                  :width="108"
                  :stroke-width="10"
                >
                  <template #default>
                    <span class="gauge-val">{{ resourceFormat(overview?.memory_percent) }}</span>
                  </template>
                </el-progress>
                <div class="resource-label">
                  <el-icon><Coin /></el-icon>
                  <span>{{ t('autoOps.memory') }}</span>
                </div>
              </div>
            </el-card>
            <el-card shadow="never" class="stat-card stat-card-resource">
              <div class="resource-gauge">
                <el-progress
                  type="dashboard"
                  :percentage="resourcePercent(overview?.disk_percent)"
                  :color="resourceColor(overview?.disk_percent || 0)"
                  :width="108"
                  :stroke-width="10"
                >
                  <template #default>
                    <span class="gauge-val">{{ resourceFormat(overview?.disk_percent) }}</span>
                  </template>
                </el-progress>
                <div class="resource-label">
                  <el-icon><FolderOpened /></el-icon>
                  <span>{{ t('autoOps.disk') }}</span>
                </div>
              </div>
            </el-card>
          </div>
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('autoOps.uptimeMonitors') }}</div>
            <div class="stat-big">{{ overview?.uptime_total ?? 0 }}</div>
            <div v-if="(overview?.uptime_down ?? 0) > 0" class="stat-warn">{{ t('autoOps.uptimeDown', { n: overview.uptime_down }) }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('autoOps.cronJobs') }}</div>
            <div class="stat-big">{{ overview?.cron_enabled ?? 0 }} / {{ overview?.cron_total ?? 0 }}</div>
            <div v-if="(overview?.cron_failed ?? 0) > 0" class="stat-warn">{{ t('autoOps.cronFailed', { n: overview.cron_failed }) }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('autoOps.serviceWatch') }}</div>
            <div class="stat-big">{{ status?.watch_count ?? 0 }}</div>
            <div v-if="(status?.down_count ?? 0) > 0" class="stat-warn">{{ t('autoOps.downCount', { n: status.down_count }) }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card clickable" @click="router.push('/backup')">
            <div class="stat-label">{{ t('autoOps.backupTasks') }}</div>
            <div class="stat-big">{{ overview?.backup_enabled ?? 0 }} / {{ overview?.backup_total ?? 0 }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card clickable" @click="router.push('/ssl')">
            <div class="stat-label">{{ t('autoOps.sslExpiring') }}</div>
            <div class="stat-big">{{ overview?.ssl_expiring_soon ?? 0 }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card clickable" @click="router.push('/websites')">
            <div class="stat-label">{{ t('autoOps.sitesExpiring') }}</div>
            <div class="stat-big">{{ overview?.sites_expiring_soon ?? 0 }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card clickable" @click="router.push('/k8s')">
            <div class="stat-label">{{ t('autoOps.k8sCluster') }}</div>
            <div class="stat-big">{{ overview?.k8s_ready ? t('k8s.ready') : (overview?.k8s_installed ? t('k8s.notReady') : '—') }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card clickable" @click="tab = 'websites'; loadWebsiteAudits()">
            <div class="stat-label">{{ t('autoOps.websiteAudit') }}</div>
            <div class="stat-big">{{ overview?.website_avg_score ?? '—' }}</div>
            <div v-if="(overview?.website_issues ?? 0) > 0" class="stat-warn">{{ t('autoOps.websiteIssues', { n: overview.website_issues }) }}</div>
          </el-card>
        </div>

        <h3 class="section-title">{{ t('autoOps.quickLinks') }}</h3>
        <div class="link-grid">
          <button v-for="link in quickLinks" :key="link.path" type="button" class="link-card" @click="router.push(link.path)">
            <el-icon class="link-icon"><component :is="link.icon" /></el-icon>
            <div class="link-body">
              <div class="link-title">{{ link.title }}</div>
              <div class="link-desc">{{ link.desc }}</div>
            </div>
            <span class="link-stat">{{ link.stat }}</span>
            <el-icon><ArrowRight /></el-icon>
          </button>
        </div>

        <el-alert type="info" :closable="false" show-icon class="overview-hint">
          {{ t('autoOps.overviewHint') }}
        </el-alert>
      </el-tab-pane>

      <el-tab-pane :label="t('autoOps.watchList')" name="watch">
        <el-alert type="info" :closable="false" show-icon class="watch-hint">
          {{ t('autoOps.watchHint') }}
        </el-alert>
        <div class="toolbar">
          <el-button :disabled="!selectedKeys.length" @click="bulkEnable(false)">
            {{ t('autoOps.enableWatch') }}
          </el-button>
          <el-button type="success" :disabled="!selectedKeys.length" @click="bulkEnable(true)">
            {{ t('autoOps.enableWatchRestart') }}
          </el-button>
        </div>
        <el-table
          :data="watches"
          stripe
          :row-class-name="({ row }: any) => row.key === route.query.key ? 'highlight-row' : ''"
          @selection-change="(rows: any[]) => selectedKeys = rows.map(r => r.key)"
        >
          <el-table-column type="selection" width="48" />
          <el-table-column :label="t('autoOps.software')" min-width="180">
            <template #default="{ row }">
              <div class="table-app-name">
                <SoftwareIcon :app-key="row.key" :size="32" />
                <span>{{ row.name }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.category')" width="120">
            <template #default="{ row }">{{ categoryLabel(row.category, t) }}</template>
          </el-table-column>
          <el-table-column :label="t('autoOps.liveStatus')" width="100">
            <template #default="{ row }">
              <el-tag :type="liveTagType(row.live_status)" size="small">{{ liveStatusLabel(row.live_status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.watchEnabled')" width="100">
            <template #default="{ row }">
              <el-switch :model-value="row.watch_enabled" @change="(v: boolean) => patchWatch(row, 'watch_enabled', v)" />
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.autoRestart')" width="100">
            <template #default="{ row }">
              <el-switch :model-value="row.auto_restart" @change="(v: boolean) => patchWatch(row, 'auto_restart', v)" />
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.lastEvent')" min-width="160">
            <template #default="{ row }">
              <template v-if="row.last_event">
                <el-tag :type="eventTagType(row.last_event)" size="small">{{ eventLabel(row.last_event) }}</el-tag>
                <span class="event-time">{{ formatTime(row.last_event_at) }}</span>
              </template>
              <span v-else>—</span>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!watches.length && !loading" :description="t('autoOps.noInstalled')">
          <el-button type="primary" @click="router.push('/software?tab=installed')">{{ t('autoOps.goSoftware') }}</el-button>
        </el-empty>
      </el-tab-pane>

      <el-tab-pane :label="t('autoOps.websiteAuditTab')" name="websites">
        <el-alert type="info" :closable="false" show-icon class="watch-hint">
          {{ t('autoOps.websiteAuditHint') }}
        </el-alert>
        <div class="toolbar">
          <el-button type="primary" :loading="scanningAll" @click="scanAllWebsites">{{ t('autoOps.websiteScanAll') }}</el-button>
          <el-button :icon="Refresh" @click="loadWebsiteAudits">{{ t('common.refresh') }}</el-button>
          <span v-if="websiteAudits?.last_scan" class="last-scan-inline">{{ t('autoOps.lastScan') }}: {{ formatTime(websiteAudits.last_scan) }}</span>
        </div>
        <el-table :data="websiteAuditItems" stripe>
          <el-table-column prop="domain" :label="t('autoOps.websiteDomain')" min-width="160" />
          <el-table-column :label="t('autoOps.websiteScore')" width="100" sortable :sort-method="(a: any, b: any) => a.score - b.score">
            <template #default="{ row }">
              <el-tag :type="gradeTagType(row.grade)" size="small">{{ row.score }} · {{ row.grade }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.websiteHTTP')" width="80">
            <template #default="{ row }">{{ row.http_status || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('autoOps.websiteLatency')" width="90">
            <template #default="{ row }">{{ row.latency_ms }}ms</template>
          </el-table-column>
          <el-table-column :label="t('autoOps.websiteIssuesCol')" width="110">
            <template #default="{ row }">
              <span v-if="row.critical" class="issue-critical">{{ row.critical }} {{ t('autoOps.severityCritical') }}</span>
              <span v-if="row.warning" class="issue-warn">{{ row.warning }} {{ t('autoOps.severityWarning') }}</span>
              <span v-if="!row.critical && !row.warning">—</span>
            </template>
          </el-table-column>
          <el-table-column prop="top_issue" :label="t('autoOps.websiteTopIssue')" min-width="180" show-overflow-tooltip />
          <el-table-column :label="t('common.actions')" width="160" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openAuditDetail(row)">{{ t('autoOps.websiteViewReport') }}</el-button>
              <el-button link size="small" :loading="scanningSite === row.site_id" @click="scanOneWebsite(row)">{{ t('autoOps.websiteRescan') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!websiteAuditItems.length" :description="t('autoOps.websiteAuditEmpty')">
          <el-button type="primary" :loading="scanningAll" @click="scanAllWebsites">{{ t('autoOps.websiteScanAll') }}</el-button>
        </el-empty>
      </el-tab-pane>

      <el-tab-pane :label="t('autoOps.events')" name="events">
        <div class="toolbar">
          <el-select v-model="eventFilterType" clearable :placeholder="t('autoOps.filterEventType')" style="width: 160px" @change="loadEvents">
            <el-option value="down_detected" :label="t('autoOps.eventDown')" />
            <el-option value="restart_ok" :label="t('autoOps.eventRestartOk')" />
            <el-option value="restart_fail" :label="t('autoOps.eventRestartFail')" />
            <el-option value="resource_cpu" :label="t('autoOps.eventResourceCPU')" />
            <el-option value="resource_memory" :label="t('autoOps.eventResourceMem')" />
            <el-option value="resource_disk" :label="t('autoOps.eventResourceDisk')" />
            <el-option value="cron_failed" :label="t('autoOps.eventCronFailed')" />
            <el-option value="ssl_expiring" :label="t('autoOps.eventSSLExpiring')" />
            <el-option value="site_expiring" :label="t('autoOps.eventSiteExpiring')" />
            <el-option value="website_issue" :label="t('autoOps.eventWebsiteIssue')" />
            <el-option value="ssl_renew_fail" :label="t('autoOps.eventSSLRenewFail')" />
          </el-select>
          <el-input v-model="eventFilterApp" clearable :placeholder="t('autoOps.filterAppKey')" style="width: 160px" @keyup.enter="loadEvents" />
          <el-button :icon="Refresh" @click="loadEvents">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table :data="events" stripe max-height="520">
          <el-table-column :label="t('autoOps.time')" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="app_name" :label="t('autoOps.software')" width="140" />
          <el-table-column :label="t('autoOps.eventType')" width="130">
            <template #default="{ row }">
              <el-tag :type="eventTagType(row.event_type)" size="small">{{ eventLabel(row.event_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" :label="t('autoOps.message')" show-overflow-tooltip />
          <el-table-column prop="status" :label="t('autoOps.status')" width="90" />
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('autoOps.settings')" name="settings">
        <el-form label-width="180px" style="max-width: 640px">
          <el-divider content-position="left">{{ t('autoOps.sectionPolicy') }}</el-divider>
          <el-form-item :label="t('autoOps.enabled')">
            <el-switch v-model="configForm.enabled" />
          </el-form-item>
          <el-form-item :label="t('autoOps.intervalSec')">
            <el-input-number v-model="configForm.interval_sec" :min="10" :max="600" />
            <span class="form-hint">{{ t('autoOps.intervalHint') }}</span>
          </el-form-item>
          <el-form-item :label="t('autoOps.cooldownSec')">
            <el-input-number v-model="configForm.cooldown_sec" :min="60" :max="3600" />
            <span class="form-hint">{{ t('autoOps.cooldownHint') }}</span>
          </el-form-item>
          <el-form-item :label="t('autoOps.maxRestarts')">
            <el-input-number v-model="configForm.max_restarts" :min="1" :max="20" />
            <span class="form-hint">{{ t('autoOps.maxRestartsHint') }}</span>
          </el-form-item>

          <el-divider content-position="left">{{ t('autoOps.sectionAlert') }}</el-divider>
          <el-form-item :label="t('autoOps.notifyWebhook')">
            <el-input v-model="configForm.notify_webhook" :placeholder="t('autoOps.notifyWebhookHint')" />
          </el-form-item>
          <el-form-item :label="t('autoOps.notifyOnDown')">
            <el-switch v-model="configForm.notify_on_down" />
          </el-form-item>
          <el-form-item :label="t('autoOps.notifyOnFail')">
            <el-switch v-model="configForm.notify_on_fail" />
          </el-form-item>

          <el-divider content-position="left">{{ t('autoOps.sectionResource') }}</el-divider>
          <el-form-item :label="t('autoOps.resourceEnabled')">
            <el-switch v-model="configForm.resource_enabled" />
          </el-form-item>
          <el-form-item v-if="configForm.resource_enabled" :label="t('autoOps.cpuThreshold')">
            <el-input-number v-model="configForm.cpu_threshold" :min="50" :max="100" />
          </el-form-item>
          <el-form-item v-if="configForm.resource_enabled" :label="t('autoOps.memThreshold')">
            <el-input-number v-model="configForm.mem_threshold" :min="50" :max="100" />
          </el-form-item>
          <el-form-item v-if="configForm.resource_enabled" :label="t('autoOps.diskThreshold')">
            <el-input-number v-model="configForm.disk_threshold" :min="50" :max="100" />
          </el-form-item>

          <el-divider content-position="left">{{ t('autoOps.sectionExpiry') }}</el-divider>
          <el-form-item :label="t('autoOps.sslAutoRenew')">
            <el-switch v-model="configForm.ssl_auto_renew" />
            <span class="form-hint">{{ t('autoOps.sslAutoRenewHint') }}</span>
          </el-form-item>
          <el-form-item :label="t('autoOps.alertDaysSSL')">
            <el-input-number v-model="configForm.alert_days_ssl" :min="1" :max="90" />
          </el-form-item>
          <el-form-item :label="t('autoOps.alertDaysSite')">
            <el-input-number v-model="configForm.alert_days_site" :min="1" :max="365" />
          </el-form-item>

          <el-divider content-position="left">{{ t('autoOps.sectionWebsite') }}</el-divider>
          <el-form-item :label="t('autoOps.websiteScanEnabled')">
            <el-switch v-model="configForm.website_scan_enabled" />
            <span class="form-hint">{{ t('autoOps.websiteScanEnabledHint') }}</span>
          </el-form-item>

          <el-form-item>
            <el-button type="primary" @click="saveConfig">{{ t('common.save') }}</el-button>
          </el-form-item>
        </el-form>
        <el-alert type="info" :closable="false" show-icon :title="t('autoOps.guideTitle')">
          <template #default>
            <ol class="guide-list">
              <li>{{ t('autoOps.guide1') }}</li>
              <li>{{ t('autoOps.guide2') }}</li>
              <li>{{ t('autoOps.guide3') }}</li>
              <li>{{ t('autoOps.guide4') }}</li>
              <li>{{ t('autoOps.guide5') }}</li>
            </ol>
          </template>
        </el-alert>
      </el-tab-pane>
    </el-tabs>

    <el-drawer v-model="auditDrawer" :title="auditDetail?.domain || t('autoOps.websiteAuditTab')" size="520px">
      <div v-if="auditDetail" class="audit-drawer">
        <div class="audit-head">
          <el-tag :type="gradeTagType(auditDetail.grade)" size="large">{{ auditDetail.score }} / 100 · {{ auditDetail.grade }}</el-tag>
          <span class="audit-meta">{{ auditDetail.url }}</span>
        </div>
        <div class="audit-cats">
          <div v-for="c in auditDetail.categories" :key="c.key" class="audit-cat">
            <span>{{ c.label }}</span>
            <el-tag :type="gradeTagType(c.grade)" size="small">{{ c.score }} · {{ c.grade }}</el-tag>
          </div>
        </div>
        <h4>{{ t('autoOps.websiteFindings') }}</h4>
        <div v-for="(f, i) in auditDetail.findings" :key="i" class="finding-card">
          <div class="finding-head">
            <el-tag :type="severityTagType(f.severity)" size="small">{{ f.severity }}</el-tag>
            <strong>{{ f.title }}</strong>
            <span class="finding-cat">{{ f.category }}</span>
          </div>
          <p class="finding-detail">{{ f.detail }}</p>
          <p v-if="f.fix_hint" class="finding-fix">{{ t('autoOps.websiteFixHint') }}: {{ f.fix_hint }}</p>
        </div>
        <el-empty v-if="!auditDetail.findings?.length" :description="t('autoOps.websiteNoIssues')" />
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.auto-ops-page { width: 100%; }
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.page-header h2 { margin: 0 0 4px; }
.hint { margin: 0; font-size: 13px; color: var(--el-text-color-secondary); }
.last-scan { margin: 4px 0 0; font-size: 12px; color: var(--el-text-color-secondary); }
.header-stats { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.toolbar { margin-bottom: 12px; display: flex; gap: 8px; flex-wrap: wrap; }
.watch-hint, .overview-hint { margin-bottom: 12px; }
.event-time { margin-left: 8px; font-size: 12px; color: var(--el-text-color-secondary); }
.form-hint { margin-left: 12px; font-size: 12px; color: var(--el-text-color-secondary); }
.guide-list { margin: 8px 0 0; padding-left: 18px; line-height: 1.7; }
.last-scan-inline { font-size: 12px; color: var(--el-text-color-secondary); align-self: center; }
.issue-critical { color: var(--el-color-danger); margin-right: 8px; font-size: 12px; }
.issue-warn { color: var(--el-color-warning); font-size: 12px; }
.audit-drawer { display: flex; flex-direction: column; gap: 12px; }
.audit-head { display: flex; flex-direction: column; gap: 6px; }
.audit-meta { font-size: 12px; color: var(--el-text-color-secondary); word-break: break-all; }
.audit-cats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.audit-cat { display: flex; justify-content: space-between; align-items: center; padding: 8px 10px; border-radius: 8px; background: var(--el-fill-color-light); font-size: 12px; }
.finding-card { border: 1px solid var(--el-border-color-lighter); border-radius: 8px; padding: 10px 12px; margin-bottom: 8px; }
.finding-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
.finding-cat { margin-left: auto; font-size: 11px; color: var(--el-text-color-secondary); text-transform: uppercase; }
.finding-detail { margin: 0; font-size: 13px; color: var(--el-text-color-regular); }
.finding-fix { margin: 6px 0 0; font-size: 12px; color: var(--el-color-primary); }
.table-app-name { display: flex; align-items: center; gap: 8px; }
:deep(.highlight-row) { background-color: var(--el-color-primary-light-9) !important; }
.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}
.resource-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  grid-column: 1 / -1;
}
@media (max-width: 720px) {
  .resource-row { grid-template-columns: 1fr; }
}
.stat-card { text-align: center; }
.stat-card-resource { padding: 12px 8px 14px; }
.resource-gauge {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.gauge-val {
  font-size: 20px;
  font-weight: 700;
  line-height: 1;
  color: var(--el-text-color-primary);
}
.resource-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.resource-label .el-icon {
  font-size: 16px;
  color: var(--el-color-primary);
}
.stat-icon {
  font-size: 22px;
  color: var(--el-color-primary);
  margin-bottom: 8px;
}
.stat-card.clickable { cursor: pointer; }
.stat-card.clickable:hover { border-color: var(--el-color-primary); }
.stat-label { font-size: 13px; color: var(--el-text-color-secondary); margin-bottom: 8px; }
.stat-val { margin-top: 6px; font-weight: 600; }
.stat-big { font-size: 28px; font-weight: 700; margin: 8px 0; }
.stat-warn { font-size: 12px; color: var(--el-color-danger); }
.section-title { margin: 0 0 12px; font-size: 15px; }
.link-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 10px; margin-bottom: 16px; }
.link-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 10px;
  background: var(--el-bg-color);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.link-card:hover { border-color: var(--el-color-primary); box-shadow: 0 2px 8px rgba(0,0,0,0.06); }
.link-icon { font-size: 22px; color: var(--el-color-primary); }
.link-body { flex: 1; min-width: 0; }
.link-title { font-weight: 600; font-size: 14px; }
.link-desc { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; }
.link-stat { font-size: 12px; color: var(--el-text-color-secondary); }
</style>
