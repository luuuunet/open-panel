<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import SystemMonitorPanel from '@/components/SystemMonitorPanel.vue'
import TrafficMap from '@/components/TrafficMap.vue'
import HealthScoreCard from '@/components/HealthScoreCard.vue'
import AlertCenter from '@/components/AlertCenter.vue'

const { t } = useI18n()
const auth = useAuthStore()
const monitorStats = ref<any>(null)
const heroTime = ref(new Date())

const greeting = computed(() => {
  const h = heroTime.value.getHours()
  if (h < 12) return t('dashboard.heroMorning')
  if (h < 18) return t('dashboard.heroAfternoon')
  return t('dashboard.heroEvening')
})

const displayName = computed(() => auth.user?.username || t('dashboard.heroGuest'))

const quickChips = computed(() => {
  const s = monitorStats.value
  if (!s) return []
  const chips: { label: string; value: string; tone?: string }[] = []
  if (s.system?.hostname) {
    chips.push({ label: t('dashboard.hostname'), value: s.system.hostname })
  }
  if (s.cpu?.usage_percent != null) {
    const pct = Math.round(s.cpu.usage_percent)
    chips.push({
      label: 'CPU',
      value: `${pct}%`,
      tone: pct >= 85 ? 'critical' : pct >= 70 ? 'warn' : 'ok',
    })
  }
  if (s.memory?.used_percent != null) {
    const pct = Math.round(s.memory.used_percent)
    chips.push({
      label: t('dashboard.memoryShort'),
      value: `${pct}%`,
      tone: pct >= 85 ? 'critical' : pct >= 70 ? 'warn' : 'ok',
    })
  }
  return chips
})

function onMonitorStats(stats: any) {
  monitorStats.value = stats
}

onMounted(() => {
  const tick = () => { heroTime.value = new Date() }
  tick()
  setInterval(tick, 60_000)
})
</script>

<template>
  <div class="dashboard intel-dashboard">
    <header class="dash-hero">
      <div class="dash-hero-copy">
        <p class="dash-hero-kicker">{{ t('dashboard.heroKicker') }}</p>
        <h1 class="dash-hero-title">{{ greeting }}, {{ displayName }}</h1>
        <p class="dash-hero-sub">{{ t('dashboard.heroSubtitle') }}</p>
      </div>
      <div v-if="quickChips.length" class="dash-hero-chips">
        <span
          v-for="chip in quickChips"
          :key="chip.label"
          class="hero-chip"
          :class="chip.tone"
        >
          <em>{{ chip.label }}</em>{{ chip.value }}
        </span>
      </div>
    </header>

    <div class="dash-grid">
      <SystemMonitorPanel layout="dashboard" hide-health @stats="onMonitorStats">
        <template #overview-top>
          <div class="overview-top-row">
            <HealthScoreCard embedded compact class="overview-health" />
            <AlertCenter
              :stats="monitorStats"
              compact
              show-ok
              :poll-sec="15"
              class="overview-alerts"
            />
          </div>
        </template>
      </SystemMonitorPanel>

      <section class="dash-traffic intel-surface">
        <div class="intel-surface-head">
          <div>
            <h3 class="intel-surface-title">{{ t('traffic.title') }}</h3>
            <p class="intel-surface-desc">{{ t('dashboard.trafficHint') }}</p>
          </div>
        </div>
        <div class="intel-surface-body">
          <TrafficMap compact dashboard />
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.intel-dashboard {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
  position: relative;
}

.intel-dashboard::before {
  content: '';
  position: absolute;
  inset: -24px -24px 40%;
  z-index: 0;
  pointer-events: none;
  background:
    radial-gradient(ellipse 80% 60% at 10% -10%, rgba(99, 102, 241, 0.12), transparent 55%),
    radial-gradient(ellipse 60% 50% at 95% 0%, rgba(246, 130, 31, 0.1), transparent 50%),
    radial-gradient(ellipse 50% 40% at 50% 100%, rgba(14, 165, 233, 0.06), transparent 60%);
}

.dash-hero,
.dash-grid {
  position: relative;
  z-index: 1;
}

.dash-hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
  padding: 4px 2px 8px;
}

.dash-hero-kicker {
  margin: 0 0 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--intel-accent, #6366f1);
}

.dash-hero-title {
  margin: 0;
  font-size: clamp(1.35rem, 2.5vw, 1.75rem);
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.15;
  color: var(--cf-text);
}

.dash-hero-sub {
  margin: 8px 0 0;
  font-size: 14px;
  line-height: 1.5;
  color: var(--cf-text-muted);
  max-width: 52ch;
}

.dash-hero-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.hero-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  background: var(--intel-chip-bg, rgba(255, 255, 255, 0.85));
  border: 1px solid var(--intel-chip-border, rgba(99, 102, 241, 0.12));
  box-shadow: var(--apple-shadow-sm, 0 1px 3px rgba(0, 0, 0, 0.04));
  backdrop-filter: blur(12px);
}

.hero-chip em {
  font-style: normal;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--cf-text-muted);
}

.hero-chip.ok { border-color: rgba(34, 197, 94, 0.25); }
.hero-chip.warn { border-color: rgba(245, 158, 11, 0.35); background: rgba(254, 243, 199, 0.5); }
.hero-chip.critical { border-color: rgba(239, 68, 68, 0.35); background: rgba(254, 226, 226, 0.45); }

.dash-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(340px, 0.92fr);
  gap: 18px;
  grid-template-areas:
    "overview traffic"
    "trend traffic"
    "apps apps";
  align-items: stretch;
}

.overview-top-row {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: 14px;
  min-height: 96px;
}

.overview-top-row :deep(.overview-health),
.overview-top-row :deep(.overview-alerts) {
  min-width: 0;
}

.overview-top-row :deep(.health-gauge-card) {
  border: 1px solid var(--intel-chip-border, rgba(99, 102, 241, 0.1));
  border-radius: 16px;
  background: linear-gradient(145deg, rgba(99, 102, 241, 0.06), rgba(255, 255, 255, 0.92));
  min-height: 100%;
  padding: 14px 16px;
  box-shadow: var(--apple-shadow-sm);
  transition: box-shadow 0.25s ease, transform 0.25s ease;
}

.overview-top-row :deep(.health-gauge-card:hover) {
  transform: translateY(-1px);
  box-shadow: var(--apple-shadow-md);
  border-color: rgba(99, 102, 241, 0.18);
}

.intel-surface {
  grid-area: traffic;
  min-width: 0;
  align-self: stretch;
  display: flex;
  flex-direction: column;
  border-radius: var(--apple-radius-lg, 18px);
  background: var(--cf-surface);
  border: 1px solid var(--intel-chip-border, rgba(99, 102, 241, 0.08));
  box-shadow: var(--apple-shadow-sm);
  overflow: hidden;
}

.intel-surface-head {
  padding: 16px 18px 12px;
  border-bottom: 1px solid var(--apple-glass-border, rgba(0, 0, 0, 0.06));
}

.intel-surface-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.intel-surface-desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--cf-text-muted);
}

.intel-surface-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.dashboard :deep(.system-monitor.dashboard) {
  display: contents;
}

.dashboard :deep(.dash-unified-card),
.dashboard :deep(.dash-trend-card),
.dashboard :deep(.dash-apps-card) {
  border: 1px solid var(--intel-chip-border, rgba(99, 102, 241, 0.08)) !important;
  border-radius: var(--apple-radius-lg, 18px) !important;
  box-shadow: var(--apple-shadow-sm) !important;
  overflow: hidden;
}

.dashboard :deep(.dash-unified-card) {
  grid-area: overview;
  min-width: 0;
}

.dashboard :deep(.dash-unified-card .el-card__body) {
  padding: 16px 18px 18px !important;
}

.dashboard :deep(.dash-trend-card) {
  grid-area: trend;
}

.dashboard :deep(.dash-apps-card) {
  grid-area: apps;
}

.dashboard :deep(.dash-trend-card .el-card__body),
.dashboard :deep(.dash-apps-card .el-card__body) {
  padding: 14px 18px 18px !important;
}

.dashboard :deep(.dash-unified-card .el-card__header),
.dashboard :deep(.dash-trend-card .el-card__header),
.dashboard :deep(.dash-apps-card .el-card__header) {
  display: none;
}

@media (max-width: 1100px) {
  .overview-top-row {
    grid-template-columns: 1fr;
    min-height: 0;
  }

  .dash-grid {
    grid-template-columns: 1fr;
    grid-template-areas:
      "overview"
      "traffic"
      "trend"
      "apps";
  }
}

@media (max-width: 640px) {
  .dash-hero {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

<style>
html.dark .intel-dashboard::before {
  background:
    radial-gradient(ellipse 80% 60% at 10% -10%, rgba(99, 102, 241, 0.18), transparent 55%),
    radial-gradient(ellipse 60% 50% at 95% 0%, rgba(246, 130, 31, 0.12), transparent 50%);
}

html.dark .hero-chip {
  background: rgba(30, 34, 48, 0.85);
  border-color: rgba(99, 102, 241, 0.2);
}

html.dark .overview-top-row .health-gauge-card {
  background: linear-gradient(145deg, rgba(99, 102, 241, 0.12), rgba(22, 26, 38, 0.95)) !important;
}
</style>
