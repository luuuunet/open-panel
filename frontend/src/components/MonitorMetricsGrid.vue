<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Cpu, Odometer, Coin, Connection, FolderOpened, Sort } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{
  stats: {
    cpu?: { usage_percent?: number; cores?: number }
    memory?: { used?: number; total?: number; used_percent?: number }
    load?: { load1?: number; load5?: number; load15?: number }
    swap?: { used?: number; total?: number; used_percent?: number }
    network?: { upload_rate?: number; download_rate?: number }
    disk?: { mount: string; used: number; total: number; used_percent?: number }[]
  }
  freeingMemory?: boolean
  optimizing?: boolean
  dense?: boolean
  hideActions?: boolean
}>(), { freeingMemory: false, optimizing: false, dense: false, hideActions: false })

const emit = defineEmits<{ freeMemory: []; optimize: []; viewProcesses: [sort: 'cpu' | 'memory'] }>()

const { t } = useI18n()

function pctColor(p: number) {
  if (p >= 85) return '#ef4444'
  if (p >= 70) return '#f59e0b'
  return '#22c55e'
}

function pctLevel(p: number) {
  if (p >= 85) return 'critical'
  if (p >= 70) return 'warn'
  return 'ok'
}

function formatGB(bytes: number) {
  if (!bytes) return '0 GB'
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

function formatRate(bps: number) {
  if (!bps || bps < 0) return '0 B/s'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  const i = Math.min(Math.floor(Math.log(bps) / Math.log(1024)), units.length - 1)
  return `${(bps / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function formatMount(mount: string) {
  const m = (mount || '').trim()
  if (m === '/' || m === '\\') return t('dashboard.rootDisk')
  if (/^C:\\?$/i.test(m)) return t('dashboard.systemDisk')
  return m || t('dashboard.rootDisk')
}

function loadHint(pct: number) {
  if (pct < 30) return t('dashboard.loadSmooth')
  if (pct < 70) return t('dashboard.loadModerate')
  return t('dashboard.loadHeavy')
}

const cpuPct = computed(() => Math.round(props.stats.cpu?.usage_percent ?? 0))
const memPct = computed(() => Math.round(props.stats.memory?.used_percent ?? 0))
const loadPct = computed(() => {
  const load1 = props.stats.load?.load1 ?? 0
  const cores = props.stats.cpu?.cores || 1
  return Math.min(100, Math.round((load1 / cores) * 100))
})

const primaryDisk = computed(() => {
  const disks = props.stats.disk || []
  if (!disks.length) return null
  return disks.find(d => d.mount === '/' || d.mount === 'C:' || d.mount === 'C:\\') || disks[0]
})

const secondaryTiles = computed(() => {
  const tiles: Array<{
    key: string
    icon: typeof Cpu
    label: string
    value: string
    sub: string
    pct?: number
    tip?: string
  }> = [
    {
      key: 'load',
      icon: Odometer,
      label: t('dashboard.loadStatus'),
      value: (props.stats.load?.load1 ?? 0).toFixed(2),
      sub: loadHint(loadPct.value),
      pct: loadPct.value,
      tip: t('dashboard.loadDetail', {
        l1: (props.stats.load?.load1 ?? 0).toFixed(2),
        l5: (props.stats.load?.load5 ?? 0).toFixed(2),
        l15: (props.stats.load?.load15 ?? 0).toFixed(2),
      }),
    },
    {
      key: 'network',
      icon: Connection,
      label: t('dashboard.networkRealtime'),
      value: formatRate((props.stats.network?.upload_rate ?? 0) + (props.stats.network?.download_rate ?? 0)),
      sub: `↑ ${formatRate(props.stats.network?.upload_rate ?? 0)} · ↓ ${formatRate(props.stats.network?.download_rate ?? 0)}`,
    },
  ]

  if (primaryDisk.value) {
    const d = primaryDisk.value
    tiles.push({
      key: 'disk',
      icon: FolderOpened,
      label: formatMount(d.mount),
      value: `${Math.round(d.used_percent ?? 0)}%`,
      sub: `${formatGB(d.used)} / ${formatGB(d.total)}`,
      pct: Math.round(d.used_percent ?? 0),
      tip: d.mount,
    })
  }

  if ((props.stats.swap?.total ?? 0) > 0) {
    tiles.push({
      key: 'swap',
      icon: Sort,
      label: t('dashboard.swapUsage'),
      value: `${Math.round(props.stats.swap?.used_percent ?? 0)}%`,
      sub: `${formatGB(props.stats.swap?.used ?? 0)} / ${formatGB(props.stats.swap?.total ?? 0)}`,
      pct: Math.round(props.stats.swap?.used_percent ?? 0),
    })
  }

  return tiles
})

</script>

<template>
  <div class="monitor-metrics" :class="{ dense }">
    <div class="metrics-grid">
      <article class="metric-card metric-card--hero metric-card--clickable" role="button" tabindex="0" @click="emit('viewProcesses', 'cpu')" @keydown.enter="emit('viewProcesses', 'cpu')">
        <div class="metric-icon cpu"><el-icon :size="18"><Cpu /></el-icon></div>
        <div class="metric-body">
          <div class="metric-head">
            <span class="metric-label">{{ t('dashboard.cpuUsage') }}</span>
            <span class="metric-value" :class="pctLevel(cpuPct)">{{ cpuPct }}%</span>
          </div>
          <div class="metric-track">
            <div class="metric-fill" :style="{ width: `${cpuPct}%`, background: pctColor(cpuPct) }" />
          </div>
          <p class="metric-sub">{{ t('dashboard.cores', { n: stats.cpu?.cores ?? 0 }) }} · {{ t('dashboard.viewTopCpu') }}</p>
        </div>
      </article>

      <article class="metric-card metric-card--hero metric-card--clickable" role="button" tabindex="0" @click="emit('viewProcesses', 'memory')" @keydown.enter="emit('viewProcesses', 'memory')">
        <div class="metric-icon memory"><el-icon :size="18"><Coin /></el-icon></div>
        <div class="metric-body">
          <div class="metric-head">
            <span class="metric-label">{{ t('dashboard.memoryUsage') }}</span>
            <span class="metric-value" :class="pctLevel(memPct)">{{ memPct }}%</span>
          </div>
          <div class="metric-track">
            <div class="metric-fill" :style="{ width: `${memPct}%`, background: pctColor(memPct) }" />
          </div>
          <div class="metric-sub-row">
            <span class="metric-sub">{{ formatGB(stats.memory?.used ?? 0) }} / {{ formatGB(stats.memory?.total ?? 0) }} · {{ t('dashboard.viewTopMemory') }}</span>
            <el-button
              class="free-mem-link"
              type="primary"
              link
              size="small"
              :loading="freeingMemory"
              @click.stop="emit('freeMemory')"
            >
              {{ t('dashboard.freeMemory') }}
            </el-button>
          </div>
        </div>
      </article>

      <el-tooltip
        v-for="tile in secondaryTiles"
        :key="tile.key"
        :content="tile.tip"
        :disabled="!tile.tip"
        placement="top"
      >
        <article class="metric-card metric-card--tile">
          <div class="metric-icon" :class="tile.key"><el-icon :size="16"><component :is="tile.icon" /></el-icon></div>
          <div class="metric-body">
            <span class="metric-label">{{ tile.label }}</span>
            <span class="metric-value metric-value--sm" :class="tile.pct != null ? pctLevel(tile.pct) : ''">{{ tile.value }}</span>
            <span class="metric-sub">{{ tile.sub }}</span>
            <div v-if="tile.pct != null" class="metric-track metric-track--thin">
              <div class="metric-fill" :style="{ width: `${tile.pct}%`, background: pctColor(tile.pct) }" />
            </div>
          </div>
        </article>
      </el-tooltip>
    </div>

    <div v-if="!hideActions" class="metrics-actions">
      <el-button type="primary" size="small" plain :loading="optimizing" @click="emit('optimize')">
        {{ t('dashboard.optimizeOneClick') }}
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.monitor-metrics {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.monitor-metrics.dense .metrics-grid {
  grid-template-columns: repeat(auto-fit, minmax(148px, 1fr));
}

.monitor-metrics.dense .metric-card--hero {
  padding: 10px 10px 8px;
}

.monitor-metrics.dense .metric-value {
  font-size: 20px;
}

.monitor-metrics.dense .metric-card--tile {
  padding: 10px 8px 8px;
}

@media (max-width: 1200px) {
  .monitor-metrics.dense .metrics-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .metrics-grid,
  .monitor-metrics.dense .metrics-grid {
    grid-template-columns: 1fr;
  }
}

.metric-card {
  border: 1px solid var(--intel-metric-border, rgba(99, 102, 241, 0.08));
  border-radius: 16px;
  background: var(--intel-metric-bg, linear-gradient(160deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.95)));
  transition: border-color 0.2s, box-shadow 0.2s, transform 0.2s;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}

.metric-card--hero {
  background: var(--intel-metric-hero-bg, linear-gradient(145deg, rgba(99, 102, 241, 0.06), rgba(255, 255, 255, 0.98)));
}

.metric-card--hero:nth-child(2) {
  background: linear-gradient(145deg, rgba(34, 197, 94, 0.07), rgba(255, 255, 255, 0.98));
}

.metric-card:hover {
  border-color: rgba(99, 102, 241, 0.16);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
  transform: translateY(-1px);
}

.metric-card--hero {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 12px 10px;
}

.metric-card--clickable {
  cursor: pointer;
}

.metric-card--clickable:hover {
  border-color: var(--el-color-primary-light-5);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);
}

.metric-card--clickable:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}

.metric-card--tile {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 10px 8px;
  min-width: 0;
}

.metric-icon {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.35);
}

.metric-card--tile .metric-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
}

.metric-icon.cpu { background: rgba(246, 130, 31, 0.12); color: var(--cf-orange, #f6821f); }
.metric-icon.memory { background: rgba(34, 197, 94, 0.12); color: #22c55e; }
.metric-icon.load { background: rgba(0, 81, 195, 0.12); color: var(--cf-link, #0051c3); }
.metric-icon.network { background: rgba(14, 165, 233, 0.12); color: #0ea5e9; }
.metric-icon.disk { background: rgba(98, 106, 239, 0.12); color: #626aef; }
.metric-icon.swap { background: rgba(168, 85, 247, 0.12); color: #a855f7; }

.metric-body {
  flex: 1;
  min-width: 0;
}

.metric-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.metric-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.metric-value {
  font-size: 26px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  line-height: 1;
  letter-spacing: -0.03em;
  color: var(--el-text-color-primary);
}

.metric-value--sm {
  display: block;
  font-size: 18px;
  font-weight: 700;
  margin: 2px 0;
}

.metric-value.ok { color: #22c55e; }
.metric-value.warn { color: #f59e0b; }
.metric-value.critical { color: #ef4444; }

.metric-track {
  height: 6px;
  border-radius: 999px;
  background: var(--el-fill-color);
  overflow: hidden;
}

.metric-track--thin {
  height: 3px;
  margin-top: 6px;
}

.metric-fill {
  height: 100%;
  border-radius: inherit;
  transition: width 0.35s ease, background 0.2s;
}

.metric-sub {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-card--tile .metric-sub {
  margin-top: 2px;
  white-space: normal;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.metric-sub-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-top: 6px;
}

.free-mem-link {
  padding: 0;
  font-size: 11px;
  height: auto;
}

.metrics-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 2px;
}

@media (max-width: 520px) {
  .metrics-grid {
    grid-template-columns: 1fr;
  }
}
</style>
