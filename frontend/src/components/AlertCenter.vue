<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { WarningFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import api from '@/api'

export interface ResourceAlert {
  type: string
  level: 'warning' | 'critical'
  value: number
  threshold: number
  message: string
}

const props = withDefaults(defineProps<{
  stats?: {
    cpu?: { usage_percent?: number }
    memory?: { used_percent?: number }
    disk?: { used_percent?: number; mount?: string }[]
  } | null
  pollSec?: number
  compact?: boolean
  showOk?: boolean
}>(), {
  pollSec: 15,
  compact: false,
  showOk: false,
})

const { t } = useI18n()
const router = useRouter()

const alerts = ref<ResourceAlert[]>([])
const thresholds = ref({ cpu: 85, mem: 85, disk: 90 })
const loading = ref(true)
let timer: ReturnType<typeof setInterval> | undefined

const hasCritical = computed(() => alerts.value.some(a => a.level === 'critical'))
const visible = computed(() => alerts.value.length > 0)

function maxDiskPct(disks: { used_percent?: number }[] | undefined) {
  if (!disks?.length) return 0
  return Math.max(...disks.map(d => d.used_percent ?? 0))
}

function buildLocalAlerts() {
  if (!props.stats) return []
  const th = thresholds.value
  const items: ResourceAlert[] = []
  const push = (type: string, value: number, threshold: number) => {
    const warnAt = threshold * 0.88
    if (value < warnAt) return
    items.push({
      type,
      level: value >= threshold ? 'critical' : 'warning',
      value: Math.round(value * 10) / 10,
      threshold,
      message: t(`alertCenter.${type}Msg`, { value: value.toFixed(1), threshold }),
    })
  }
  push('cpu', props.stats.cpu?.usage_percent ?? 0, th.cpu)
  push('memory', props.stats.memory?.used_percent ?? 0, th.mem)
  push('disk', maxDiskPct(props.stats.disk), th.disk)
  return items
}

async function loadFromApi() {
  try {
    const res: any = await api.get('/dashboard/alerts')
    thresholds.value = res.data?.thresholds || thresholds.value
    alerts.value = res.data?.alerts || []
  } catch {
    alerts.value = buildLocalAlerts()
  } finally {
    loading.value = false
  }
}

async function loadThresholds() {
  try {
    const res: any = await api.get('/dashboard/alerts')
    if (res.data?.thresholds) {
      thresholds.value = res.data.thresholds
    }
  } catch { /* use defaults */ }
}

function refresh() {
  if (props.stats) {
    alerts.value = buildLocalAlerts()
    loading.value = false
    return
  }
  loadFromApi()
}

watch(() => props.stats, () => refresh(), { deep: true })

onMounted(async () => {
  await loadThresholds()
  refresh()
  timer = setInterval(refresh, props.pollSec * 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

function alertTitle(a: ResourceAlert) {
  if (a.level === 'critical') {
    return t(`alertCenter.${a.type}Critical`, { value: a.value, threshold: a.threshold })
  }
  return t(`alertCenter.${a.type}Warning`, { value: a.value, threshold: a.threshold })
}

function goSettings() {
  router.push({ path: '/auto-ops', query: { tab: 'settings' } })
}

defineExpose({ refresh, alerts, hasCritical })
</script>

<template>
  <div
    v-if="visible || (showOk && compact)"
    class="alert-center"
    :class="{ compact, critical: hasCritical, ok: !visible && showOk }"
  >
    <div class="alert-center-head">
      <span class="alert-center-title">
        {{ visible ? t('alertCenter.title') : t('dashboard.allSystemsOk') }}
      </span>
      <el-button v-if="!compact && visible" link type="primary" size="small" @click="goSettings">
        {{ t('alertCenter.configure') }}
      </el-button>
    </div>
    <div v-if="visible" v-loading="loading && !alerts.length" class="alert-list">
      <div
        v-for="(a, i) in alerts"
        :key="`${a.type}-${i}`"
        class="alert-item"
        :class="a.level"
      >
        <el-icon class="alert-icon">
          <CircleCloseFilled v-if="a.level === 'critical'" />
          <WarningFilled v-else />
        </el-icon>
        <div class="alert-body">
          <strong>{{ alertTitle(a) }}</strong>
          <span class="alert-detail">{{ a.message }}</span>
        </div>
      </div>
    </div>
    <p v-else-if="showOk" class="alert-ok-msg">{{ t('dashboard.allSystemsOkHint') }}</p>
  </div>
</template>

<style scoped>
.alert-center {
  border-radius: 16px;
  border: 1px solid var(--el-color-warning-light-5);
  background: var(--el-color-warning-light-9);
  padding: 14px 16px;
  box-shadow: var(--apple-shadow-sm, 0 1px 3px rgba(0, 0, 0, 0.04));
}
.alert-center.critical {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}
.alert-center.compact {
  padding: 8px 10px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
}
.alert-center.ok {
  border-color: rgba(34, 197, 94, 0.25);
  background: linear-gradient(145deg, rgba(34, 197, 94, 0.08), rgba(255, 255, 255, 0.92));
}
.alert-ok-msg {
  margin: 0;
  flex: 1;
  display: flex;
  align-items: center;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.45;
}
.alert-center-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}
.alert-center-title {
  font-weight: 700;
  font-size: 13px;
}
.alert-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 24px;
}
.alert-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 12px;
}
.alert-item.warning {
  background: rgba(230, 162, 60, 0.15);
  color: var(--el-color-warning-dark-2);
}
.alert-item.critical {
  background: rgba(245, 108, 108, 0.18);
  color: var(--el-color-danger-dark-2);
}
.alert-icon {
  flex-shrink: 0;
  margin-top: 1px;
  font-size: 16px;
}
.alert-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.alert-detail {
  opacity: 0.85;
  word-break: break-word;
}
</style>
