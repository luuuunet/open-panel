<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { EChartsOption } from 'echarts'
import api, { resolveApiError } from '@/api'
import EChart from '@/components/EChart.vue'
import RunningSoftwarePanel, { type InstalledAppMetric } from '@/components/RunningSoftwarePanel.vue'
import MonitorMetricsGrid from '@/components/MonitorMetricsGrid.vue'
import ProcessTopDrawer, { type ProcessSort } from '@/components/ProcessTopDrawer.vue'
import SystemStatusGauges from '@/components/SystemStatusGauges.vue'
import { useChartTheme } from '@/composables/useChartTheme'
import { useLocaleStore } from '@/stores/locale'
import { useAuthStore } from '@/stores/auth'
import { isChineseLocale } from '@/locales'
import { ElMessage, ElMessageBox } from 'element-plus'
import { MagicStick } from '@element-plus/icons-vue'
import { usePerformanceProfile } from '@/composables/usePerformanceProfile'

const props = withDefaults(defineProps<{ compact?: boolean; sidebar?: boolean; layout?: 'default' | 'dashboard'; hideHealth?: boolean }>(), {
  compact: false,
  sidebar: false,
  layout: 'default',
  hideHealth: false,
})

const emit = defineEmits<{ stats: [any] }>()

const { t } = useI18n()
const localeStore = useLocaleStore()
const auth = useAuthStore()
const isAdmin = computed(() => !auth.user?.role || auth.user.role === 'admin')
const { colors, themeKey, axisStyle, titleStyle, tooltipStyle, isDark } = useChartTheme()

const loading = ref(true)
const freeingMemory = ref(false)
const optimizing = ref(false)
const processDrawerVisible = ref(false)
const processSort = ref<ProcessSort>('cpu')
const optimizeDialog = ref(false)
const optimizeResult = ref<any>(null)
const current = ref<any>(null)
const history = ref<any[]>([])
const installedApps = ref<InstalledAppMetric[]>([])
const aiModels = ref<any[]>([])
const hours = ref(1)
const autoRefresh = ref(true)
const lastUpdatedAt = ref(0)
const nowTick = ref(Date.now())
let tickTimer: ReturnType<typeof setInterval> | undefined
const compareOverlay = ref(false)
const overlayMetrics = ref<Array<'cpu' | 'memory' | 'load'>>(['cpu', 'memory'])
const { load: loadPerf, liteIntervalSec, fullIntervalSec } = usePerformanceProfile()
const effectiveLiteSec = computed(() => (props.layout === 'dashboard' ? 5 : liteIntervalSec.value))
const effectiveFullSec = computed(() => (props.layout === 'dashboard' ? 30 : fullIntervalSec.value))
const useMonitorApi = ref(true)
let liteTimer: ReturnType<typeof setInterval> | undefined
let fullTimer: ReturnType<typeof setInterval> | undefined

const localHistory = ref<any[]>([])
let prevSample: { at: number; netSent: number; netRecv: number } | null = null

function markUpdated() {
  lastUpdatedAt.value = Date.now()
}

const lastUpdatedLabel = computed(() => {
  nowTick.value
  if (!lastUpdatedAt.value) return t('dashboard.neverUpdated')
  const sec = Math.max(0, Math.floor((Date.now() - lastUpdatedAt.value) / 1000))
  return t('dashboard.lastUpdated', { sec })
})

function loadSeriesForLoad(): [number, number][] {
  const cores = current.value?.cpu?.cores || 1
  return history.value.map(p => [p.time * 1000, Math.min(100, ((p.load1 ?? 0) / cores) * 100)])
}

function formatBytes(bytes: number) {
  if (!bytes || bytes < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function normalizeStats(raw: any) {
  if (!raw) return null
  return {
    cpu: raw.cpu || { usage_percent: 0, cores: 0 },
    memory: raw.memory || { total: 0, used: 0, free: 0, used_percent: 0 },
    load: raw.load || { load1: 0, load5: 0, load15: 0 },
    swap: raw.swap || { total: 0, used: 0, free: 0, used_percent: 0 },
    disk: raw.disk || [],
    disk_io: raw.disk_io || { read_rate: 0, write_rate: 0, read_bytes: 0, write_bytes: 0 },
    network: {
      bytes_sent: raw.network?.bytes_sent ?? 0,
      bytes_recv: raw.network?.bytes_recv ?? 0,
      upload_rate: raw.network?.upload_rate ?? 0,
      download_rate: raw.network?.download_rate ?? 0,
    },
    system: raw.system || { hostname: '', os: '', platform: '', platform_version: '', uptime: 0 },
  }
}

function pushLocalHistory(stats: any) {
  const now = Math.floor(Date.now() / 1000)
  let netUp = stats.network.upload_rate || 0
  let netDown = stats.network.download_rate || 0
  if (prevSample && stats.network.bytes_sent != null) {
    const dt = Math.max(1, now - prevSample.at)
    if (stats.network.bytes_sent >= prevSample.netSent) {
      netUp = (stats.network.bytes_sent - prevSample.netSent) / dt
    }
    if (stats.network.bytes_recv >= prevSample.netRecv) {
      netDown = (stats.network.bytes_recv - prevSample.netRecv) / dt
    }
    stats.network.upload_rate = netUp
    stats.network.download_rate = netDown
  }
  if (stats.network.bytes_sent != null) {
    prevSample = { at: now, netSent: stats.network.bytes_sent, netRecv: stats.network.bytes_recv }
  }
  const point = {
    time: now,
    cpu: stats.cpu.usage_percent ?? 0,
    memory: stats.memory.used_percent ?? 0,
    load1: stats.load.load1 ?? 0,
    net_up: netUp,
    net_down: netDown,
    disk_read: stats.disk_io.read_rate ?? 0,
    disk_write: stats.disk_io.write_rate ?? 0,
  }
  localHistory.value = [...localHistory.value, point].slice(-240)
  if (localHistory.value.length >= 2) {
    history.value = [...localHistory.value]
  }
}

function mapInstalledApps(raw: any[]): InstalledAppMetric[] {
  return (raw || []).map(a => ({
    key: a.key,
    name: a.name,
    category: a.category,
    version: a.version,
    port: a.port,
    live_status: a.live_status || a.status || 'stopped',
    cpu: a.cpu || 0,
    memory: a.memory || 0,
  }))
}

function applyInstalledApps(raw: any[]) {
  installedApps.value = mapInstalledApps(raw)
}

async function loadExtras() {
  try {
    const apps: any = await api.get('/software/installed')
    applyInstalledApps((apps.data || []).map((a: any) => ({
      ...a,
      live_status: a.status,
      cpu: 0,
      memory: 0,
    })))
  } catch { /* ignore */ }
}

function isValidMonitorPayload(res: any) {
  return res?.data?.current?.cpu != null && typeof res.data.current.cpu.usage_percent === 'number'
}

async function loadFromStats() {
  const statsRes: any = await api.get('/dashboard/stats')
  const stats = normalizeStats(statsRes.data)
  if (!stats) return
  current.value = stats
  pushLocalHistory(stats)
  await loadExtras()
}

function pushLiveHistoryTick(fromPoll = false) {
  if (!current.value) return
  const now = Math.floor(Date.now() / 1000)
  const last = history.value[history.value.length - 1]
  const point = {
    time: now,
    cpu: current.value.cpu.usage_percent ?? 0,
    memory: current.value.memory.used_percent ?? 0,
    load1: current.value.load.load1 ?? 0,
    net_up: current.value.network.upload_rate ?? 0,
    net_down: current.value.network.download_rate ?? 0,
    disk_read: current.value.disk_io.read_rate ?? 0,
    disk_write: current.value.disk_io.write_rate ?? 0,
  }
  if (last?.time === now) {
    if (fromPoll) history.value = [...history.value.slice(0, -1), point]
    return
  }
  if (last && now <= last.time) return
  if (!fromPoll && last && now - last.time < 1) return
  history.value = [...history.value, point].slice(-240)
}

async function loadLite() {
  try {
    const res: any = await api.get('/dashboard/monitor', { params: { hours: hours.value, lite: 1 }, timeout: 15000 })
    if (isValidMonitorPayload(res)) {
      current.value = normalizeStats(res.data.current)
      if (res.data.history?.length >= 2) {
        history.value = res.data.history
      }
      pushLiveHistoryTick(true)
      markUpdated()
    }
  } catch {
    /* keep last sample */
  }
}

async function loadFull() {
  try {
    if (useMonitorApi.value) {
      const res: any = await api.get('/dashboard/monitor', { params: { hours: hours.value }, timeout: 30000 })
      if (isValidMonitorPayload(res)) {
        current.value = normalizeStats(res.data.current)
        if (res.data.history?.length >= 2) {
          history.value = res.data.history
        } else if (localHistory.value.length >= 2) {
          history.value = [...localHistory.value]
        }
        pushLiveHistoryTick(true)
        aiModels.value = res.data.ai_models || []
        if (res.data.installed_apps?.length) {
          applyInstalledApps(res.data.installed_apps)
        } else {
          await loadExtras()
        }
        markUpdated()
        return
      }
    }
    await loadFromStats()
    markUpdated()
  } finally {
    loading.value = false
  }
}

async function load() {
  await loadFull()
}

function formatBytesShort(bytes: number) {
  return formatBytes(bytes)
}

async function onFreeMemory() {
  try {
    await ElMessageBox.confirm(t('dashboard.freeMemoryConfirm'), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  freeingMemory.value = true
  try {
    const res: any = await api.post('/dashboard/free-memory')
    const data = res.data || {}
    if (!data.supported) {
      ElMessage.warning(data.message || t('dashboard.freeMemoryUnsupported'))
      return
    }
    const freed = data.freed_bytes || 0
    if (freed > 0) {
      ElMessage.success(t('dashboard.freeMemorySuccess', { size: formatBytesShort(freed) }))
    } else {
      ElMessage.success(data.message || t('dashboard.freeMemoryDone'))
    }
    await loadLite()
    await loadFull()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('dashboard.freeMemoryFailed')))
  } finally {
    freeingMemory.value = false
  }
}

function openProcessDrawer(sort: ProcessSort) {
  processSort.value = sort
  processDrawerVisible.value = true
}

function stepStatusLabel(status: string) {
  switch (status) {
    case 'success': return t('dashboard.optimizeStepSuccess')
    case 'failed': return t('dashboard.optimizeStepFailed')
    case 'partial': return t('dashboard.optimizeStepPartial')
    default: return t('dashboard.optimizeStepSkipped')
  }
}

function stepTagType(status: string) {
  switch (status) {
    case 'success': return 'success'
    case 'failed': return 'danger'
    case 'partial': return 'warning'
    default: return 'info'
  }
}

async function onOptimize() {
  try {
    await ElMessageBox.confirm(t('dashboard.optimizeConfirm'), t('dashboard.optimizeOneClick'), { type: 'warning' })
  } catch {
    return
  }
  optimizing.value = true
  try {
    const res: any = await api.post('/dashboard/optimize')
    optimizeResult.value = res.data
    optimizeDialog.value = true
    ElMessage.success(t('dashboard.optimizeSuccess'))
    await loadLite()
    await loadFull()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('dashboard.optimizeFailed')))
  } finally {
    optimizing.value = false
  }
}

function formatRate(bps: number) {
  return `${formatBytes(Math.max(0, bps))}/s`
}

function formatUptime(seconds: number) {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return t('dashboard.uptimeFormat', { d, h, m })
}

function formatDiskMount(mount: string) {
  const m = (mount || '').trim()
  if (m === '/' || m === '\\') return t('dashboard.rootDisk')
  if (/^C:\\?$/i.test(m)) return t('dashboard.systemDisk')
  return m || t('dashboard.rootDisk')
}

function timeLocale() {
  return isChineseLocale(localeStore.locale) ? 'zh-CN' : 'en-US'
}

function formatModelSize(bytes: number) {
  if (!bytes) return '—'
  return formatBytes(bytes)
}

watch(hours, () => {
  loading.value = true
  load()
})

watch(current, (v) => {
  if (v) emit('stats', v)
}, { immediate: true })

function seriesData(key: string): [number, number][] {
  return history.value.map(p => [p.time * 1000, p[key] ?? 0])
}

function pctChart(title: string, key: string, color: string, currentVal?: number): EChartsOption {
  const val = currentVal ?? 0
  const axis = axisStyle()
  return {
    backgroundColor: 'transparent',
    title: titleStyle(`${title}  ${val.toFixed(1)}%`),
    legend: { show: false },
    tooltip: {
      ...tooltipStyle(),
      formatter: (params: any) => {
        const p = params[0]
        const d = new Date(p.value[0])
        return `${d.toLocaleString(timeLocale())}<br/>${title}: ${Number(p.value[1]).toFixed(1)}%`
      },
    },
    grid: { left: 44, right: 16, top: 36, bottom: 24 },
    xAxis: { type: 'time', ...axis },
    yAxis: { type: 'value', min: 0, max: 100, splitNumber: 5, axisLabel: { ...axis.axisLabel, formatter: '{value}%' }, splitLine: axis.splitLine, axisLine: axis.axisLine },
    series: [{
      name: title,
      type: 'line',
      smooth: 0.35,
      showSymbol: false,
      areaStyle: { opacity: 0.22, color },
      lineStyle: { width: 2, color },
      itemStyle: { color },
      data: seriesData(key),
    }],
  }
}

function loadChart(): EChartsOption {
  const val = current.value?.load?.load1 ?? 0
  const lineColor = isDark.value ? colors.value.chart : colors.value.link
  const axis = axisStyle()
  return {
    backgroundColor: 'transparent',
    title: titleStyle(`${t('dashboard.loadAvg')}  ${val.toFixed(2)}`),
    legend: { show: false },
    tooltip: {
      ...tooltipStyle(),
      formatter: (params: any) => {
        const p = params[0]
        return `${new Date(p.value[0]).toLocaleString(timeLocale())}<br/>${t('dashboard.loadAvg')}: ${Number(p.value[1]).toFixed(2)}`
      },
    },
    grid: { left: 44, right: 16, top: 36, bottom: 24 },
    xAxis: { type: 'time', ...axis },
    yAxis: { type: 'value', min: 0, splitNumber: 4, ...axis },
    series: [{
      type: 'line', smooth: 0.35, showSymbol: false,
      areaStyle: { opacity: 0.22, color: lineColor },
      lineStyle: { width: 2, color: lineColor },
      data: seriesData('load1'),
    }],
  }
}

function ioChart(
  title: string,
  upKey: string,
  downKey: string,
  upLabel: string,
  downLabel: string,
  upColor: string,
  downColor: string,
  currentUp: number,
  currentDown: number,
): EChartsOption {
  const axis = axisStyle()
  return {
    backgroundColor: 'transparent',
    title: titleStyle(`${title}  ↑${formatRate(currentUp)} ↓${formatRate(currentDown)}`, 12),
    tooltip: {
      ...tooltipStyle(),
      formatter: (params: any) => {
        const time = new Date(params[0]?.value?.[0]).toLocaleString(timeLocale())
        return params.reduce((s: string, p: any) => `${s}${p.seriesName}: ${formatRate(p.value[1])}<br/>`, `${time}<br/>`)
      },
    },
    legend: { data: [upLabel, downLabel], bottom: 0, itemWidth: 10, itemHeight: 8, textStyle: { fontSize: 11, color: colors.value.textSecondary } },
    grid: { left: 52, right: 16, top: 40, bottom: 32 },
    xAxis: { type: 'time', ...axis },
    yAxis: { type: 'value', axisLabel: { ...axis.axisLabel, formatter: (v: number) => formatRate(v) }, splitLine: axis.splitLine, axisLine: axis.axisLine },
    series: [
      { name: upLabel, type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: 0.16 }, lineStyle: { width: 1.5 }, itemStyle: { color: upColor }, data: seriesData(upKey) },
      { name: downLabel, type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: 0.16 }, lineStyle: { width: 1.5 }, itemStyle: { color: downColor }, data: seriesData(downKey) },
    ],
  }
}

const cpuOption = computed(() => {
  themeKey.value
  const lineColor = isDark.value ? colors.value.chart : colors.value.orange
  return pctChart(t('dashboard.cpuUsage'), 'cpu', lineColor, current.value?.cpu?.usage_percent)
})
const memOption = computed(() => {
  themeKey.value
  return pctChart(t('dashboard.memoryUsage'), 'memory', colors.value.success, current.value?.memory?.used_percent)
})
const loadOption = computed(() => {
  themeKey.value
  return loadChart()
})
const netOption = computed(() => {
  themeKey.value
  return ioChart(
    t('dashboard.networkIO'), 'net_up', 'net_down',
    t('dashboard.upload'), t('dashboard.download'), colors.value.chart, colors.value.chartSecondary,
    current.value?.network?.upload_rate ?? 0,
    current.value?.network?.download_rate ?? 0,
  )
})
const diskIOOption = computed(() => {
  themeKey.value
  return ioChart(
    t('dashboard.diskIO'), 'disk_read', 'disk_write',
    t('dashboard.diskRead'), t('dashboard.diskWrite'), '#626aef', colors.value.danger,
    current.value?.disk_io?.read_rate ?? 0,
    current.value?.disk_io?.write_rate ?? 0,
  )
})

const diskItems = computed(() => current.value?.disk || [])

const hasHistory = computed(() => history.value.length >= 2)
const hasCurrentData = computed(() => current.value?.cpu?.usage_percent != null)

type MetricKey = 'cpu' | 'memory' | 'load' | 'disk_io' | 'network'
const metricTab = ref<MetricKey>('cpu')

const metricTabs = computed(() => [
  { key: 'cpu' as MetricKey, label: t('dashboard.cpuUsage') },
  { key: 'memory' as MetricKey, label: t('dashboard.memoryUsage') },
  { key: 'load' as MetricKey, label: t('dashboard.loadAvg') },
  { key: 'disk_io' as MetricKey, label: t('dashboard.diskIO') },
  { key: 'network' as MetricKey, label: t('dashboard.networkIO') },
])

function overlayChart(): EChartsOption {
  const axis = axisStyle()
  const defs: Record<string, { label: string; color: string; data: () => [number, number][] }> = {
    cpu: { label: t('dashboard.cpuUsage'), color: isDark.value ? colors.value.chart : colors.value.orange, data: () => seriesData('cpu') },
    memory: { label: t('dashboard.memoryUsage'), color: colors.value.success, data: () => seriesData('memory') },
    load: { label: t('dashboard.loadAvg'), color: colors.value.link, data: () => loadSeriesForLoad() },
  }
  const picked = overlayMetrics.value.map(k => defs[k]).filter(Boolean)
  return {
    backgroundColor: 'transparent',
    title: titleStyle(t('dashboard.overlayTitle')),
    tooltip: { ...tooltipStyle(), trigger: 'axis' },
    legend: { data: picked.map(p => p.label), bottom: 0, textStyle: { fontSize: 11, color: colors.value.textSecondary } },
    grid: { left: 44, right: 16, top: 36, bottom: 36 },
    xAxis: { type: 'time', ...axis },
    yAxis: { type: 'value', min: 0, max: 100, splitNumber: 5, axisLabel: { ...axis.axisLabel, formatter: '{value}%' }, splitLine: axis.splitLine, axisLine: axis.axisLine },
    series: picked.map(p => ({
      name: p.label,
      type: 'line',
      smooth: 0.35,
      showSymbol: false,
      lineStyle: { width: 2, color: p.color },
      itemStyle: { color: p.color },
      data: p.data(),
    })),
  }
}

const activeTrendOption = computed(() => {
  themeKey.value
  if (compareOverlay.value && overlayMetrics.value.length > 0) {
    return overlayChart()
  }
  switch (metricTab.value) {
    case 'cpu': return cpuOption.value
    case 'memory': return memOption.value
    case 'load': return loadOption.value
    case 'disk_io': return diskIOOption.value
    case 'network': return netOption.value
    default: return cpuOption.value
  }
})

function diskDonutOption(d: { mount: string; used: number; total: number; used_percent?: number }): EChartsOption {
  const pct = Math.round(d.used_percent ?? 0)
  const color = pct > 80 ? colors.value.danger : pct > 60 ? colors.value.warning : (isDark.value ? colors.value.chart : colors.value.orange)
  const free = Math.max(0, d.total - d.used)
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: colors.value.tooltipBg,
      borderColor: colors.value.tooltipBorder,
      textStyle: { color: colors.value.text },
      formatter: () => `${d.mount}<br/>${formatBytes(d.used)} / ${formatBytes(d.total)} (${pct}%)`,
    },
    series: [{
      type: 'pie',
      radius: ['58%', '78%'],
      center: ['50%', '50%'],
      label: {
        show: true,
        position: 'center',
        formatter: `${pct}%`,
        fontSize: 16,
        fontWeight: 700,
        color: colors.value.text,
      },
      labelLine: { show: false },
      data: [
        { value: d.used, name: 'used', itemStyle: { color } },
        { value: free, name: 'free', itemStyle: { color: colors.value.pieFree } },
      ],
    }],
  }
}

function clearTimers() {
  if (liteTimer) clearInterval(liteTimer)
  if (fullTimer) clearInterval(fullTimer)
  liteTimer = fullTimer = undefined
}

function startTimers() {
  clearTimers()
  if (!autoRefresh.value) return
  liteTimer = setInterval(loadLite, effectiveLiteSec.value * 1000)
  fullTimer = setInterval(loadFull, effectiveFullSec.value * 1000)
}

function onAutoRefreshChange(on: boolean) {
  autoRefresh.value = on
  if (on) {
    loadFull()
    startTimers()
  } else {
    clearTimers()
  }
}

function onVisibilityChange() {
  if (document.hidden) {
    clearTimers()
  } else if (autoRefresh.value) {
    loadFull()
    startTimers()
  }
}

function onTick() {
  nowTick.value = Date.now()
  if (autoRefresh.value && !document.hidden && current.value) {
    pushLiveHistoryTick(false)
  }
}

onMounted(async () => {
  await loadPerf(true)
  await loadFull()
  startTimers()
  tickTimer = setInterval(onTick, 1000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

watch([effectiveLiteSec, effectiveFullSec], () => {
  if (!document.hidden && autoRefresh.value) startTimers()
})

watch(autoRefresh, (on) => {
  if (!on) clearTimers()
  else if (!document.hidden) startTimers()
})

onUnmounted(() => {
  clearTimers()
  if (tickTimer) clearInterval(tickTimer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="system-monitor" :class="[layout, { compact, sidebar }]">
    <template v-if="layout === 'dashboard'">
      <el-card shadow="hover" class="dash-unified-card">
        <slot name="overview-top" />
        <div v-if="$slots['overview-top']" class="overview-divider" />
        <div class="dash-card-header dash-unified-monitor-head">
          <div class="dash-header-main">
            <span class="dash-title">{{ t('dashboard.monitorTitle') }}</span>
            <div v-if="current" class="dash-meta-chips">
              <span class="status-chip">
                <em>{{ t('dashboard.uptime') }}</em>{{ formatUptime(current.system?.uptime ?? 0) }}
              </span>
              <span class="status-chip">
                <em>{{ t('dashboard.networkIO') }}</em>↑{{ formatRate(current.network?.upload_rate ?? 0) }} ↓{{ formatRate(current.network?.download_rate ?? 0) }}
              </span>
            </div>
          </div>
          <div class="dash-header-tools">
            <span class="last-updated">{{ lastUpdatedLabel }}</span>
            <label class="auto-refresh-toggle">
              <span>{{ t('dashboard.autoRefresh') }}</span>
              <el-switch :model-value="autoRefresh" size="small" @change="onAutoRefreshChange" />
            </label>
            <el-radio-group v-model="hours" size="small" class="dash-range-group">
              <el-radio-button :value="1">{{ t('dashboard.range1h') }}</el-radio-button>
              <el-radio-button :value="6">{{ t('dashboard.range6h') }}</el-radio-button>
              <el-radio-button :value="24">{{ t('dashboard.range24h') }}</el-radio-button>
            </el-radio-group>
            <el-button type="primary" size="small" plain :loading="optimizing" @click="onOptimize">
              <el-icon class="btn-icon"><MagicStick /></el-icon>
              {{ t('dashboard.optimizeOneClick') }}
            </el-button>
          </div>
        </div>
        <div v-loading="loading" class="dash-stats-body">
          <MonitorMetricsGrid
            v-if="current"
            dense
            :stats="current"
            :freeing-memory="freeingMemory"
            :optimizing="optimizing"
            hide-actions
            @free-memory="onFreeMemory"
            @optimize="onOptimize"
            @view-processes="openProcessDrawer"
          />
        </div>
      </el-card>

      <el-card shadow="hover" class="dash-trend-card">
        <template #header>
          <div class="dash-card-header dash-trend-header">
            <span>{{ t('dashboard.trendTitle') }}</span>
            <div class="trend-toolbar-right">
              <el-switch
                v-model="compareOverlay"
                size="small"
                :active-text="t('dashboard.overlayCompare')"
              />
              <el-checkbox-group v-if="compareOverlay" v-model="overlayMetrics" size="small" class="overlay-checks">
                <el-checkbox label="cpu">CPU</el-checkbox>
                <el-checkbox label="memory">{{ t('dashboard.memoryShort') }}</el-checkbox>
                <el-checkbox label="load">{{ t('dashboard.loadShort') }}</el-checkbox>
              </el-checkbox-group>
              <div v-if="!compareOverlay" class="metric-pills">
                <button
                  v-for="tab in metricTabs"
                  :key="tab.key"
                  type="button"
                  class="metric-pill"
                  :class="{ active: metricTab === tab.key }"
                  @click="metricTab = tab.key"
                >
                  {{ tab.label }}
                </button>
              </div>
            </div>
          </div>
        </template>
        <el-alert
          v-if="!loading && !hasHistory && !hasCurrentData"
          :title="t('dashboard.collectingHint')"
          type="info"
          :closable="false"
          show-icon
          style="margin-bottom: 8px"
        />
        <EChart :key="`${metricTab}-${compareOverlay}-${themeKey}`" :option="activeTrendOption" height="220px" />
      </el-card>

      <el-card shadow="hover" class="dash-apps-card">
        <template #header>{{ t('dashboard.runningApps') }}</template>
        <RunningSoftwarePanel :apps="installedApps" @refresh="load" />
        <template v-if="aiModels.length">
          <div class="panel-title dash-models-title">{{ t('dashboard.runningModels') }}</div>
          <el-table :data="aiModels" size="small" stripe max-height="160">
            <el-table-column prop="name" :label="t('dashboard.modelName')" show-overflow-tooltip />
            <el-table-column :label="t('dashboard.modelMemory')" width="100">
              <template #default="{ row }">{{ formatModelSize(row.size) }}</template>
            </el-table-column>
            <el-table-column prop="provider" :label="t('dashboard.modelProvider')" width="72" />
          </el-table>
        </template>
      </el-card>
    </template>

    <template v-else>
    <div v-if="!compact" class="monitor-toolbar">
      <div class="live-stats" v-if="current">
        <span class="live-item"><em>CPU</em>{{ current.cpu.usage_percent.toFixed(1) }}%</span>
        <span class="live-item"><em>{{ t('dashboard.memoryUsage') }}</em>{{ current.memory.used_percent.toFixed(1) }}%</span>
        <span class="live-item"><em>{{ t('dashboard.loadAvg') }}</em>{{ (current.load?.load1 ?? 0).toFixed(2) }}</span>
        <span class="live-item"><em>{{ t('dashboard.networkIO') }}</em>↑{{ formatRate(current.network?.upload_rate ?? 0) }} ↓{{ formatRate(current.network?.download_rate ?? 0) }}</span>
        <span class="live-item"><em>{{ t('dashboard.uptime') }}</em>{{ formatUptime(current.system?.uptime ?? 0) }}</span>
      </div>
      <el-radio-group v-model="hours" size="small">
        <el-radio-button :value="1">{{ t('dashboard.range1h') }}</el-radio-button>
        <el-radio-button :value="6">{{ t('dashboard.range6h') }}</el-radio-button>
        <el-radio-button :value="24">{{ t('dashboard.range24h') }}</el-radio-button>
      </el-radio-group>
    </div>

    <div v-else class="compact-toolbar" :class="{ 'compact-toolbar-sidebar': sidebar }">
      <div class="compact-toolbar-left">
        <span v-if="!sidebar">{{ t('dashboard.monitorTitle') }}</span>
        <div v-if="current" class="compact-live">
          <span>{{ t('dashboard.uptime') }} {{ formatUptime(current.system?.uptime ?? 0) }}</span>
          <span>↑{{ formatRate(current.network?.upload_rate ?? 0) }} ↓{{ formatRate(current.network?.download_rate ?? 0) }}</span>
        </div>
      </div>
      <el-radio-group v-model="hours" size="small">
        <el-radio-button :value="1">{{ t('dashboard.range1h') }}</el-radio-button>
        <el-radio-button :value="6">{{ t('dashboard.range6h') }}</el-radio-button>
        <el-radio-button :value="24">{{ t('dashboard.range24h') }}</el-radio-button>
      </el-radio-group>
    </div>

    <div v-loading="loading" class="monitor-body">
      <el-alert
        v-if="!loading && !hasHistory && !hasCurrentData"
        :title="t('dashboard.collectingHint')"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 12px"
      />

      <div class="trend-panel">
        <div class="trend-toolbar">
          <div class="trend-toolbar-left">
            <span class="panel-title">{{ t('dashboard.trendTitle') }}</span>
            <span class="trend-tip">{{ t('dashboard.loadStatusTip') }}</span>
          </div>
          <div class="trend-toolbar-right">
            <div class="metric-pills">
              <button
                v-for="tab in metricTabs"
                :key="tab.key"
                type="button"
                class="metric-pill"
                :class="{ active: metricTab === tab.key }"
                @click="metricTab = tab.key"
              >
                {{ tab.label }}
              </button>
            </div>
            <el-button type="primary" size="small" :loading="optimizing" @click="onOptimize">
              <el-icon class="btn-icon"><MagicStick /></el-icon>
              {{ t('dashboard.optimizeOneClick') }}
            </el-button>
          </div>
        </div>

        <SystemStatusGauges
          v-if="current"
          embedded
          compact-row
          :sidebar="sidebar"
          :show-head="false"
          :stats="current"
          :freeing-memory="freeingMemory"
          :optimizing="optimizing"
          @free-memory="onFreeMemory"
          @optimize="onOptimize"
        />

        <EChart :key="`${metricTab}-${themeKey}`" :option="activeTrendOption" :height="sidebar ? '180px' : (compact ? '220px' : '280px')" />
      </div>

      <el-row v-if="diskItems.length" :gutter="12" class="disk-row">
        <el-col :span="24">
          <div class="disk-panel">
            <div class="panel-title">{{ t('dashboard.diskTitle') }}</div>
            <div class="disk-grid">
              <div v-for="(d, i) in diskItems" :key="d.mount + i" class="disk-card">
                <div class="disk-mount" :title="d.mount">{{ formatDiskMount(d.mount) }}</div>
                <EChart :key="`${d.mount}-${themeKey}`" :option="diskDonutOption(d)" height="140px" />
                <div class="disk-meta">{{ formatBytes(d.used) }} / {{ formatBytes(d.total) }}</div>
              </div>
            </div>
          </div>
        </el-col>
      </el-row>

      <!-- 已安装软件 -->
      <el-row :gutter="12" class="running-row">
        <el-col :span="24">
          <div class="disk-panel">
            <div class="panel-title">{{ t('dashboard.runningApps') }}</div>
            <RunningSoftwarePanel :apps="installedApps" @refresh="load" />
          </div>
        </el-col>
      </el-row>

      <el-row v-if="aiModels.length" :gutter="12" class="running-row">
        <el-col :span="24">
          <div class="disk-panel">
            <div class="panel-title">{{ t('dashboard.runningModels') }}</div>
            <el-table v-if="aiModels.length" :data="aiModels" size="small" stripe max-height="200">
              <el-table-column prop="name" :label="t('dashboard.modelName')" show-overflow-tooltip />
              <el-table-column :label="t('dashboard.modelMemory')" width="100">
                <template #default="{ row }">{{ formatModelSize(row.size) }}</template>
              </el-table-column>
              <el-table-column prop="provider" :label="t('dashboard.modelProvider')" width="72" />
            </el-table>
            <el-empty v-else :description="t('dashboard.noRunningModels')" :image-size="48" />
          </div>
        </el-col>
      </el-row>

      <el-row v-if="!compact" :gutter="12" style="margin-top: 8px">
        <el-col :span="24">
          <div class="disk-panel info-panel" v-if="current">
            <div class="panel-title">{{ t('dashboard.systemInfo') }}</div>
            <el-descriptions :column="3" size="small" border>
              <el-descriptions-item :label="t('dashboard.hostname')">{{ current.system.hostname }}</el-descriptions-item>
              <el-descriptions-item :label="t('dashboard.system')">{{ current.system.os }} / {{ current.system.platform }}</el-descriptions-item>
              <el-descriptions-item :label="t('dashboard.platformVersion')">{{ current.system.platform_version }}</el-descriptions-item>
              <el-descriptions-item :label="t('dashboard.uptime')">{{ formatUptime(current.system?.uptime ?? 0) }}</el-descriptions-item>
              <el-descriptions-item :label="t('dashboard.loadAvg')">{{ (current.load?.load1 ?? 0).toFixed(2) }} / {{ (current.load?.load5 ?? 0).toFixed(2) }} / {{ (current.load?.load15 ?? 0).toFixed(2) }}</el-descriptions-item>
              <el-descriptions-item :label="t('dashboard.sampleInterval')">{{ liteIntervalSec }}s</el-descriptions-item>
            </el-descriptions>
          </div>
        </el-col>
      </el-row>
    </div>
    </template>

    <ProcessTopDrawer
      v-model:visible="processDrawerVisible"
      :sort="processSort"
      :is-admin="isAdmin"
      @refresh="loadLite"
    />

    <el-dialog v-model="optimizeDialog" :title="t('dashboard.optimizeResultTitle')" width="520px">
      <div v-if="optimizeResult" class="optimize-result">
        <p class="optimize-summary">{{ optimizeResult.summary }}</p>
        <p v-if="optimizeResult.improved" class="optimize-improved">
          <el-tag type="success" size="small">{{ t('dashboard.optimizeImproved') }}</el-tag>
          <span>{{ t('dashboard.loadAvg') }} {{ optimizeResult.before_load }} → {{ optimizeResult.after_load }} · {{ t('dashboard.memoryUsage') }} {{ optimizeResult.before_mem_pct }}% → {{ optimizeResult.after_mem_pct }}%</span>
        </p>
        <div class="panel-title">{{ t('dashboard.optimizeSteps') }}</div>
        <div v-for="step in optimizeResult.steps || []" :key="step.key" class="optimize-step">
          <div class="optimize-step-head">
            <span>{{ step.title }}</span>
            <el-tag :type="stepTagType(step.status)" size="small">{{ stepStatusLabel(step.status) }}</el-tag>
          </div>
          <p class="optimize-step-msg">{{ step.message }}</p>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.system-monitor { width: 100%; }
.monitor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.live-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  font-size: 13px;
}
.live-item em {
  font-style: normal;
  color: var(--el-text-color-secondary);
  margin-right: 6px;
}
.compact-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
  font-weight: 600;
  flex-wrap: wrap;
}
.compact-toolbar-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.compact-live {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 12px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.disk-panel {
  border: 1px solid var(--cf-border);
  border-radius: 8px;
  padding: 8px 8px 0;
  background: var(--cf-surface);
}
.panel-title {
  font-size: 14px;
  font-weight: 600;
  padding: 4px 8px 8px;
  color: var(--cf-text);
}
.info-panel :deep(.el-descriptions) { margin: 0 8px 12px; }
.running-row { margin-top: 12px; }
.running-row .disk-panel { min-height: 120px; }
.running-row :deep(.el-empty) { padding: 12px 0; }
.trend-panel {
  border: 1px solid var(--cf-border);
  border-radius: 12px;
  padding: 12px 12px 4px;
  margin-bottom: 12px;
  background: var(--cf-surface);
}
.trend-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.trend-toolbar-left {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.trend-toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.trend-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.btn-icon { margin-right: 4px; vertical-align: -2px; }
.metric-pills { display: flex; flex-wrap: wrap; gap: 6px; }
.metric-pill {
  border: 1px solid var(--cf-border);
  background: var(--el-fill-color-lighter);
  color: var(--el-text-color-regular);
  font-size: 12px;
  font-weight: 600;
  padding: 5px 14px;
  border-radius: 999px;
  cursor: pointer;
  transition: all 0.15s;
}
.metric-pill:hover { border-color: var(--cf-orange, #f6821f); color: var(--cf-orange, #f6821f); }
.metric-pill.active {
  background: var(--cf-orange, #f6821f);
  border-color: var(--cf-orange, #f6821f);
  color: #fff;
}
.disk-row { margin-top: 0; }
.disk-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
  padding: 0 8px 12px;
}
.disk-card {
  text-align: center;
  border: 1px solid var(--cf-border);
  border-radius: 10px;
  padding: 8px 6px 4px;
  background: var(--el-fill-color-lighter);
}
.disk-mount { font-size: 12px; font-weight: 600; color: var(--el-text-color-regular); }
.disk-meta { font-size: 11px; color: var(--el-text-color-secondary); margin-top: -6px; padding-bottom: 4px; }
.optimize-result { display: flex; flex-direction: column; gap: 10px; }
.optimize-summary { margin: 0; font-size: 14px; color: var(--el-text-color-primary); }
.optimize-improved { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 0; font-size: 12px; color: var(--el-text-color-secondary); }
.optimize-step { border: 1px solid var(--cf-border); border-radius: 8px; padding: 8px 10px; }
.optimize-step-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: 13px; font-weight: 600; }
.optimize-step-msg { margin: 6px 0 0; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.45; }
.system-monitor.sidebar .trend-toolbar {
  flex-direction: column;
  align-items: stretch;
}
.system-monitor.sidebar .trend-toolbar-right {
  justify-content: space-between;
}
.system-monitor.sidebar .metric-pills {
  flex: 1;
}
.system-monitor.sidebar .disk-grid {
  grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
}
.compact-toolbar-sidebar {
  margin-bottom: 6px;
}
.compact-toolbar-sidebar .compact-toolbar-left {
  flex-direction: row;
  align-items: center;
  gap: 12px;
}
.system-monitor.sidebar .trend-panel {
  padding: 10px 10px 4px;
}
.system-monitor.sidebar .trend-tip {
  display: none;
}
.system-monitor.sidebar .disk-row {
  display: none;
}

/* Dashboard home layout */
.system-monitor.dashboard :deep(.dash-unified-card .el-card__body) {
  display: flex;
  flex-direction: column;
  gap: 0;
}
.system-monitor.dashboard :deep(.overview-divider) {
  height: 1px;
  margin: 14px 0;
  background: linear-gradient(90deg, transparent, var(--el-border-color-lighter), transparent);
}
.system-monitor.dashboard :deep(.dash-unified-monitor-head) {
  padding-bottom: 12px;
  margin-bottom: 6px;
  border-bottom: none;
}
.system-monitor.dashboard :deep(.dash-title) {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--cf-text-muted);
}
.system-monitor.dashboard :deep(.status-chip) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-regular);
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.06);
  border: 1px solid rgba(99, 102, 241, 0.1);
  white-space: nowrap;
}
.system-monitor.dashboard :deep(.metric-pill) {
  padding: 6px 14px;
  font-size: 12px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: var(--el-fill-color-light);
}
.system-monitor.dashboard :deep(.metric-pill.active) {
  background: linear-gradient(135deg, var(--cf-orange), #e56f10);
  border-color: transparent;
  box-shadow: 0 4px 12px rgba(246, 130, 31, 0.25);
}
.system-monitor.dashboard :deep(.status-chip em) {
  font-style: normal;
  font-weight: 700;
  font-size: 10px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}
.system-monitor.dashboard :deep(.dash-stats-body) {
  padding-top: 4px;
}
.system-monitor.dashboard :deep(.dash-card-header) {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  font-weight: 600;
}
.system-monitor.dashboard :deep(.dash-header-main) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.system-monitor.dashboard :deep(.dash-meta-chips) {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.system-monitor.dashboard :deep(.dash-trend-header) {
  align-items: center;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--cf-text-muted);
}
.system-monitor.dashboard :deep(.dash-header-tools) {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-weight: 400;
}
.system-monitor.dashboard :deep(.dash-range-group) {
  margin-left: auto;
}
.system-monitor.dashboard :deep(.last-updated) {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.system-monitor.dashboard :deep(.auto-refresh-toggle) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--el-text-color-regular);
  cursor: pointer;
}
.system-monitor.dashboard :deep(.overlay-checks) {
  display: inline-flex;
  gap: 4px;
}
.system-monitor.dashboard :deep(.dash-models-title) {
  margin-top: 16px;
  padding-left: 0;
}
.system-monitor.dashboard :deep(.metric-pills) {
  gap: 4px;
}
.system-monitor.dashboard :deep(.metric-pill) {
  padding: 4px 12px;
  font-size: 11px;
}
@media (max-width: 640px) {
  .system-monitor.dashboard :deep(.dash-header-tools) {
    width: 100%;
    justify-content: flex-start;
  }
  .system-monitor.dashboard :deep(.trend-toolbar-right) {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
