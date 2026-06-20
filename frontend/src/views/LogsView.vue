<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MagicStick } from '@element-plus/icons-vue'
import SoftwareIcon from '@/components/SoftwareIcon.vue'
import LogViewer from '@/components/LogViewer.vue'
import AIChatPanel, { type AIChatMessage } from '@/components/AIChatPanel.vue'
import { categoryLabel as softwareCategoryLabel } from '@/locales'
import api, { AI_REQUEST_TIMEOUT } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const { t } = useI18n()

interface LogSource {
  id: string
  category: string
  name: string
  path: string
  enabled: boolean
  exists: boolean
  size: number
  virtual?: boolean
  app_key?: string
  app_name?: string
  log_kind?: string
}

interface InstalledAppBoard {
  key: string
  name: string
  icon?: string
  icon_url?: string
  status: string
  live_status?: string
  version?: string
  port?: number
  category?: string
  logs: LogSource[]
}

const loading = ref(false)
const sources = ref<LogSource[]>([])
const installedApps = ref<InstalledAppBoard[]>([])
const activeId = ref('')
const logContent = ref('')
const logPath = ref('')
const logSize = ref(0)
const lines = ref(300)
const keyword = ref('')
const autoRefresh = ref(false)
const saving = ref<string | null>(null)
const cleanupLoading = ref(false)
const clearAllLoading = ref(false)
const retentionDays = ref(7)
const autoCleanup = ref(false)
const maxSizeMB = ref(50)
const maxRotatedFiles = ref(5)
const compressRotated = ref(true)
const loggingEnabled = ref(true)
const savingRetention = ref(false)
const savingLogging = ref(false)
const aiOpen = ref(true)
const chatBySource = ref<Record<string, AIChatMessage[]>>({})
let timer: ReturnType<typeof setInterval> | undefined

const otherCategoryOrder = ['panel', 'system', 'website', 'cache', 'cluster', 'waf']

const enabledSources = computed(() => sources.value.filter((s) => s.enabled))
const activeSource = computed(() => sources.value.find((s) => s.id === activeId.value))
const otherSources = computed(() => sources.value.filter((s) => s.category !== 'software'))

const aiChatMessages = computed({
  get: () => chatBySource.value[activeId.value] || [],
  set: (v: AIChatMessage[]) => {
    if (activeId.value) chatBySource.value[activeId.value] = v
  },
})

const otherGrouped = computed(() => {
  const map = new Map<string, LogSource[]>()
  for (const src of otherSources.value) {
    if (!map.has(src.category)) map.set(src.category, [])
    map.get(src.category)!.push(src)
  }
  const out: { key: string; title: string; items: LogSource[] }[] = []
  for (const key of otherCategoryOrder) {
    const items = map.get(key)
    if (items?.length) out.push({ key, title: logCategoryLabel(key), items: items.sort((a, b) => a.name.localeCompare(b.name)) })
    map.delete(key)
  }
  for (const [key, items] of [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
    out.push({ key, title: logCategoryLabel(key), items })
  }
  return out
})

const filteredContent = computed(() => {
  const raw = logContent.value
  if (!keyword.value.trim()) return raw
  const kw = keyword.value.trim().toLowerCase()
  return raw.split('\n').filter((line) => line.toLowerCase().includes(kw)).join('\n')
})

function logCategoryLabel(key: string) {
  const k = `logsPage.categories.${key}`
  const msg = t(k)
  return msg === k ? key : msg
}

function logLabel(src: LogSource) {
  if (src.log_kind) {
    const k = `logsPage.kinds.${src.log_kind}`
    const msg = t(k)
    if (msg !== k) return msg
  }
  const parts = src.name.split(' · ')
  const fallback = parts.length > 1 ? parts[parts.length - 1] : src.name
  const lower = fallback.toLowerCase()
  if (lower.includes('error') || lower.endsWith('.err')) {
    const k = 'logsPage.kinds.error'
    const msg = t(k)
    if (msg !== k) return msg
  }
  if (lower.includes('access')) {
    const k = 'logsPage.kinds.access'
    const msg = t(k)
    if (msg !== k) return msg
  }
  return fallback
}

function emptyFileHint(src?: LogSource) {
  const kind = src?.log_kind?.toLowerCase() || ''
  if (kind === 'php_cgi') return t('logsPage.emptyFilePhp')
  if (kind === 'error' || kind === 'php_error' || kind === 'php_ini_error') return t('logsPage.emptyFileError')
  if (kind === 'access') return t('logsPage.emptyFileAccess')
  return t('logsPage.emptyFile')
}

function statusType(s: string) {
  if (s === 'running') return 'success'
  if (s === 'installing') return 'warning'
  if (s === 'failed') return 'danger'
  return 'info'
}

function statusLabel(s: string) {
  if (s === 'running') return t('common.running')
  if (s === 'stopped') return t('common.stopped')
  if (s === 'installing') return t('software.installing')
  if (s === 'failed') return t('software.failed')
  return s
}

function formatSize(n: number) {
  if (!n) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

function isAppActive(app: InstalledAppBoard) {
  return app.logs.some((l) => l.id === activeId.value)
}

function pickDefaultSource() {
  const enabled = enabledSources.value
  const withData = enabled.find((s) => s.exists)
  if (withData) return withData.id
  if (enabled.length) return enabled[0].id
  return ''
}

function syncFromResponse(data: any) {
  sources.value = data?.sources || []
  installedApps.value = data?.installed_apps || []
}

async function loadSources() {
  const res: any = await api.get('/logs/sources')
  syncFromResponse(res.data)
  if (!activeId.value || !enabledSources.value.some((s) => s.id === activeId.value)) {
    activeId.value = pickDefaultSource()
  }
}

async function loadRetention() {
  try {
    const res: any = await api.get('/logs/retention')
    const data = res.data || {}
    retentionDays.value = data.retention_days ?? 7
    autoCleanup.value = !!data.auto_cleanup
    maxSizeMB.value = data.max_size_mb ?? 50
    maxRotatedFiles.value = data.max_rotated_files ?? 5
    compressRotated.value = data.compress_rotated !== false
    loggingEnabled.value = data.logging_enabled !== false
    if (!loggingEnabled.value) {
      autoRefresh.value = false
    }
  } catch {
    /* keep defaults */
  }
}

async function saveLoggingEnabled(enabled: boolean) {
  if (savingLogging.value) return
  if (!enabled) {
    try {
      await ElMessageBox.confirm(t('logsPage.loggingDisabledConfirm'), t('common.warning'), { type: 'warning' })
    } catch {
      return
    }
  }
  savingLogging.value = true
  try {
    const res: any = await api.put('/logs/retention', {
      retention_days: retentionDays.value,
      auto_cleanup: autoCleanup.value,
      logging_enabled: enabled,
      max_size_mb: maxSizeMB.value,
      max_rotated_files: maxRotatedFiles.value,
      compress_rotated: compressRotated.value,
    })
    const data = res.data || {}
    loggingEnabled.value = data.logging_enabled !== false
    if (!loggingEnabled.value) {
      autoRefresh.value = false
      activeId.value = ''
      logContent.value = t('logsPage.loggingDisabledHint')
    }
    await loadSources()
    if (loggingEnabled.value) {
      await loadContent()
    }
    ElMessage.success(enabled ? t('logsPage.loggingEnabledSuccess') : t('logsPage.loggingDisabledSuccess'))
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.saveFailed'))
  } finally {
    savingLogging.value = false
  }
}

async function saveRetention() {
  if (savingRetention.value) return
  savingRetention.value = true
  try {
    const res: any = await api.put('/logs/retention', {
      retention_days: retentionDays.value,
      auto_cleanup: autoCleanup.value,
      logging_enabled: loggingEnabled.value,
      max_size_mb: maxSizeMB.value,
      max_rotated_files: maxRotatedFiles.value,
      compress_rotated: compressRotated.value,
    })
    const data = res.data || {}
    retentionDays.value = data.retention_days ?? retentionDays.value
    autoCleanup.value = !!data.auto_cleanup
    ElMessage.success(t('logsPage.retentionSaved'))
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.saveFailed'))
  } finally {
    savingRetention.value = false
  }
}

function formatCleanupResult(data: { cleared_files?: number; bytes_freed?: number; skipped?: number; deleted_files?: number; trimmed_files?: number }) {
  const freed = data.bytes_freed || 0
  return t('logsPage.cleanupResult', {
    cleared: data.cleared_files ?? 0,
    deleted: data.deleted_files ?? 0,
    trimmed: data.trimmed_files ?? 0,
    skipped: data.skipped ?? 0,
    freed: formatSize(freed),
  })
}

async function clearAllLogs() {
  try {
    await ElMessageBox.confirm(t('logsPage.clearAllConfirm'), t('common.warning'), { type: 'warning' })
  } catch {
    return
  }
  clearAllLoading.value = true
  try {
    const res: any = await api.post('/logs/clear-all')
    ElMessage.success(formatCleanupResult(res.data || {}))
    await loadSources()
    await loadContent()
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.loadFailed'))
  } finally {
    clearAllLoading.value = false
  }
}

async function cleanupOldLogs() {
  const days = retentionDays.value
  if (!days || days <= 0) {
    ElMessage.warning(t('logsPage.retentionDays'))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('logsPage.cleanupOldConfirm', { days }),
      t('common.warning'),
      { type: 'warning' },
    )
  } catch {
    return
  }
  cleanupLoading.value = true
  try {
    const res: any = await api.post('/logs/cleanup', { days })
    ElMessage.success(formatCleanupResult(res.data || {}))
    await loadSources()
    await loadContent()
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.loadFailed'))
  } finally {
    cleanupLoading.value = false
  }
}

async function loadContent() {
  if (!loggingEnabled.value) {
    logContent.value = t('logsPage.loggingDisabledHint')
    return
  }
  if (!activeId.value) {
    logContent.value = ''
    return
  }
  const src = findSource(activeId.value)
  if (!src?.enabled) {
    logContent.value = ''
    return
  }
  loading.value = true
  try {
    const res: any = await api.get(`/logs/tail/${encodeURIComponent(activeId.value)}`, {
      params: { lines: lines.value },
    })
    const data = res.data || {}
    logPath.value = data.path || ''
    logSize.value = data.size || 0
    if (!data.exists) {
      logContent.value = t('logsPage.fileMissing')
    } else if (!String(data.content || '').trim()) {
      logContent.value = emptyFileHint(src)
    } else {
      logContent.value = data.content
    }
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('logsPage.loadFailed'))
  } finally {
    loading.value = false
  }
}

function findSource(id: string): LogSource | undefined {
  return sources.value.find((s) => s.id === id)
}

async function toggleSource(src: LogSource, selectAfter = false) {
  if (!loggingEnabled.value) return
  const enabled = !src.enabled
  saving.value = src.id
  try {
    const res: any = await api.put('/logs/sources', { enabled: { [src.id]: enabled } })
    syncFromResponse(res.data)
    const updated = findSource(src.id)
    if (enabled && selectAfter && updated) {
      activeId.value = updated.id
    } else if (!enabled && activeId.value === src.id) {
      activeId.value = pickDefaultSource()
      await loadContent()
    }
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.saveFailed'))
  } finally {
    saving.value = null
  }
}

async function onLogPillClick(src: LogSource) {
  if (!loggingEnabled.value) return
  const id = src.id
  const current = findSource(id) || src
  if (!current.enabled) {
    await toggleSource(current, true)
  } else {
    activeId.value = id
  }
  await loadContent()
}

async function onAppCardClick(app: InstalledAppBoard) {
  if (!loggingEnabled.value) return
  const logs = app.logs || []
  if (!logs.length) {
    ElMessage.info(t('logsPage.noAppLogs'))
    return
  }
  const pick = logs.find((l) => l.enabled) || logs[0]
  if (!pick.enabled) {
    await toggleSource(pick, true)
  } else {
    activeId.value = pick.id
  }
  await loadContent()
}

async function toggleCategory(key: string, enabled: boolean) {
  if (!loggingEnabled.value) return
  const items = sources.value.filter((s) => s.category === key)
  const payload: Record<string, boolean> = {}
  for (const s of items) payload[s.id] = enabled
  saving.value = key
  try {
    const res: any = await api.put('/logs/sources', { enabled: payload })
    syncFromResponse(res.data)
    if (!enabledSources.value.some((s) => s.id === activeId.value)) {
      activeId.value = pickDefaultSource()
      await loadContent()
    }
  } catch (e: any) {
    ElMessage.error(e?.error || t('logsPage.saveFailed'))
    await loadSources()
  } finally {
    saving.value = null
  }
}

function categorySwitchOn(group: { items: LogSource[] }) {
  return group.items.length > 0 && group.items.every((s) => s.enabled)
}

async function onCategorySwitch(group: { key: string; items: LogSource[] }) {
  await toggleCategory(group.key, !categorySwitchOn(group))
}

function selectSource(src: LogSource) {
  if (!src.enabled) return
  activeId.value = src.id
}

function logsStreamBody(message: string, history: AIChatMessage[]) {
  const src = activeSource.value
  return {
    message,
    source_id: src?.id || '',
    source_name: src?.name || '',
    category: src?.category || '',
    path: logPath.value || src?.path || '',
    log_content: logContent.value,
    history: history.filter((m) => !m.streaming).slice(-10).map((m) => ({ role: m.role, content: m.content })),
  }
}

async function logsSendFallback(message: string, history: AIChatMessage[], signal: AbortSignal) {
  if (!logContent.value.trim()) {
    throw new Error(t('logsPage.noLogForAi'))
  }
  const res: any = await api.post('/logs/ai/chat', logsStreamBody(message, history), {
    timeout: AI_REQUEST_TIMEOUT,
    signal,
  } as any)
  return res.data?.reply || ''
}

function setupAutoRefresh() {
  if (timer) clearInterval(timer)
  if (autoRefresh.value && loggingEnabled.value) timer = setInterval(loadContent, 5000)
}

watch(activeId, () => loadContent())
watch(lines, () => loadContent())
watch(autoRefresh, setupAutoRefresh)
watch(loggingEnabled, (on) => {
  if (!on) {
    autoRefresh.value = false
    setupAutoRefresh()
  }
})

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([loadSources(), loadRetention()])
    await loadContent()
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="logs-page">
    <div class="page-header">
      <div>
        <h2>{{ t('logsPage.title') }}</h2>
        <p class="hint">{{ t('logsPage.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <div class="logging-toggle">
          <span class="logging-toggle-label">{{ t('logsPage.loggingEnabled') }}</span>
          <el-switch
            :model-value="loggingEnabled"
            :loading="savingLogging"
            :active-text="t('logsPage.loggingOn')"
            :inactive-text="t('logsPage.loggingOff')"
            @change="(v: boolean) => saveLoggingEnabled(v)"
          />
        </div>
        <el-divider direction="vertical" />
        <el-input-number v-model="lines" :min="50" :max="2000" :step="50" size="small" />
        <el-input v-model="keyword" :placeholder="t('logsPage.search')" clearable size="small" style="width:180px" />
        <el-switch v-model="autoRefresh" :active-text="t('logsPage.autoRefresh')" :disabled="!loggingEnabled" />
        <el-button :loading="loading" :disabled="!loggingEnabled" @click="loadSources(); loadContent()">{{ t('logsPage.refresh') }}</el-button>
        <el-divider direction="vertical" />
        <span class="retention-label">{{ t('logsPage.retentionDays') }}</span>
        <el-input-number
          v-model="retentionDays"
          :min="0"
          :max="3650"
          :step="1"
          size="small"
          controls-position="right"
          @change="saveRetention"
        />
        <el-switch
          v-model="autoCleanup"
          :active-text="t('logsPage.autoCleanup')"
          :loading="savingRetention"
          :disabled="!loggingEnabled"
          @change="saveRetention"
        />
        <span class="retention-label">{{ t('logsPage.maxSizeMB') }}</span>
        <el-input-number v-model="maxSizeMB" :min="10" :max="10240" size="small" @change="saveRetention" />
        <span class="retention-label">{{ t('logsPage.maxRotatedFiles') }}</span>
        <el-input-number v-model="maxRotatedFiles" :min="1" :max="20" size="small" @change="saveRetention" />
        <el-switch
          v-model="compressRotated"
          :active-text="t('logsPage.compressRotated')"
          :loading="savingRetention"
          :disabled="!loggingEnabled"
          @change="saveRetention"
        />
        <el-button
          :loading="cleanupLoading"
          :disabled="!loggingEnabled"
          @click="cleanupOldLogs"
        >
          {{ t('logsPage.cleanupOld', { days: retentionDays || 7 }) }}
        </el-button>
        <el-button
          type="danger"
          plain
          :loading="clearAllLoading"
          :disabled="!loggingEnabled"
          @click="clearAllLogs"
        >
          {{ t('logsPage.clearAll') }}
        </el-button>
        <el-button type="primary" plain :icon="MagicStick" :disabled="!loggingEnabled" @click="aiOpen = !aiOpen">{{ t('logsPage.aiAssistant') }}</el-button>
      </div>
    </div>

    <el-alert
      v-if="!loggingEnabled"
      type="warning"
      :closable="false"
      show-icon
      class="logging-off-banner"
      :title="t('logsPage.loggingDisabledBanner')"
      :description="t('logsPage.loggingDisabledHint')"
    />

    <section class="apps-board">
      <div class="board-head">
        <h3>{{ t('logsPage.installedApps') }}</h3>
        <span class="board-hint">{{ t('logsPage.installedAppsHint') }}</span>
      </div>
      <el-empty v-if="!installedApps.length && !loading" :description="t('logsPage.noInstalled')" :image-size="56" />
      <div v-else class="apps-grid">
        <div
          v-for="app in installedApps"
          :key="app.key"
          class="app-card"
          :class="{ active: isAppActive(app), running: app.status === 'running' }"
          @click="onAppCardClick(app)"
        >
          <div class="app-card-head" @click.stop="onAppCardClick(app)">
            <SoftwareIcon :app-key="app.key" :icon-url="app.icon_url" :size="40" />
            <div class="app-meta">
              <div class="app-name">{{ app.name }}</div>
              <div class="app-sub">
                <span v-if="app.version">v{{ app.version }}</span>
                <span v-if="app.port"> · :{{ app.port }}</span>
                <span v-if="app.category"> · {{ softwareCategoryLabel(app.category, t) }}</span>
              </div>
            </div>
            <el-tag :type="statusType(app.status)" size="small" effect="plain" class="status-tag">
              {{ statusLabel(app.status) }}
            </el-tag>
          </div>
          <div v-if="app.logs?.length" class="log-pills">
            <button
              v-for="log in app.logs"
              :key="log.id"
              type="button"
              class="log-pill"
              :class="{
                on: log.enabled,
                selected: activeId === log.id,
                missing: !log.exists,
                loading: saving === log.id,
              }"
              @click.stop="onLogPillClick(log)"
              @contextmenu.prevent="toggleSource(log)"
            >
              {{ logLabel(log) }}
            </button>
          </div>
          <p v-else class="no-logs">{{ t('logsPage.noAppLogs') }}</p>
        </div>
      </div>
    </section>

    <div class="logs-layout" :class="{ 'with-ai': aiOpen }">
      <aside v-if="otherGrouped.length" class="sources-panel">
        <div class="sources-head">
          <span>{{ t('logsPage.otherLogs') }}</span>
        </div>
        <el-collapse accordion>
          <el-collapse-item v-for="group in otherGrouped" :key="group.key" :name="group.key">
            <template #title>
              <div class="group-title" @click.stop>
                <span>{{ group.title }}</span>
                <button
                  type="button"
                  class="ios-switch"
                  :class="{ on: categorySwitchOn(group), loading: saving === group.key }"
                  :disabled="!loggingEnabled || saving === group.key"
                  role="switch"
                  :aria-checked="categorySwitchOn(group)"
                  @click.stop="onCategorySwitch(group)"
                >
                  <span class="ios-switch-knob" />
                </button>
              </div>
            </template>
            <div
              v-for="src in group.items"
              :key="src.id"
              class="source-item"
              :class="{ active: activeId === src.id, off: !src.enabled }"
              @click="selectSource(src)"
            >
              <span class="source-name">{{ src.name }}</span>
              <button
                type="button"
                class="ios-switch sm"
                :class="{ on: src.enabled, loading: saving === src.id }"
                :disabled="!loggingEnabled || saving === src.id"
                role="switch"
                :aria-checked="src.enabled"
                @click.stop="toggleSource(src)"
              >
                <span class="ios-switch-knob" />
              </button>
            </div>
          </el-collapse-item>
        </el-collapse>
      </aside>

      <section class="viewer-panel" v-loading="loading">
        <div class="viewer-head">
          <div class="viewer-title">
            <SoftwareIcon
              v-if="activeSource?.app_key"
              :app-key="activeSource.app_key"
              :size="28"
            />
            <div>
              <strong>{{ activeSource?.enabled ? logLabel(activeSource) : t('logsPage.pickSource') }}</strong>
              <span v-if="activeSource?.app_name" class="app-from">{{ activeSource.app_name }}</span>
              <span v-if="logPath" class="path">{{ logPath }}</span>
            </div>
          </div>
          <span v-if="logSize" class="size">{{ formatSize(logSize) }}</span>
        </div>
        <LogViewer
          :content="keyword.trim() ? (filteredContent || '') : (logContent || '')"
          :kind="activeSource?.log_kind === 'access' ? 'access' : activeSource?.log_kind === 'error' ? 'error' : 'generic'"
          :empty-text="keyword.trim() ? t('logsPage.empty') : (logContent ? t('logsPage.empty') : t('logsPage.pickSource'))"
          max-height="calc(100vh - 320px)"
        />
      </section>

      <aside v-show="aiOpen" class="ai-panel">
        <AIChatPanel
          v-model="aiChatMessages"
          :welcome="t('logsPage.aiWelcome')"
          :placeholder="t('logsPage.aiPlaceholder')"
          :context-label="activeSource?.name"
          :quick-prompts="[t('logsPage.aiPromptErrors'), t('logsPage.aiPromptSummary')]"
          stream-url="/logs/ai/chat/stream"
          :stream-body="logsStreamBody"
          :send-fallback="logsSendFallback"
          :disabled="!activeSource?.enabled"
          height="100%"
        />
      </aside>
    </div>
  </div>
</template>

<style scoped>
.logs-page { height: 100%; display: flex; flex-direction: column; gap: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; flex-wrap: wrap; }
.page-header h2 { margin: 0; }
.hint { margin: 4px 0 0; color: var(--el-text-color-secondary); font-size: 13px; }
.header-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.logging-toggle { display: flex; align-items: center; gap: 8px; }
.logging-toggle-label { font-size: 13px; font-weight: 600; white-space: nowrap; }
.logging-off-banner { margin-top: -4px; }
.retention-label { font-size: 12px; color: var(--el-text-color-secondary); white-space: nowrap; }

.apps-board {
  border: 1px solid var(--el-border-color-lighter); border-radius: 10px;
  background: var(--el-bg-color); padding: 14px 16px;
}
.board-head { display: flex; align-items: baseline; gap: 12px; margin-bottom: 12px; flex-wrap: wrap; }
.board-head h3 { margin: 0; font-size: 15px; font-weight: 600; }
.board-hint { font-size: 12px; color: var(--el-text-color-secondary); }
.apps-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px;
}
.app-card {
  border: 1px solid var(--el-border-color-lighter); border-radius: 10px; padding: 12px;
  background: var(--el-fill-color-blank); transition: border-color 0.15s, box-shadow 0.15s;
}
.app-card.running { border-color: var(--el-color-success-light-5); }
.app-card.active { border-color: var(--cf-orange, #f6821f); box-shadow: 0 0 0 1px var(--cf-orange, #f6821f); }
.app-card-head { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.app-meta { flex: 1; min-width: 0; }
.app-name { font-weight: 600; font-size: 14px; line-height: 1.3; }
.app-sub { font-size: 11px; color: var(--el-text-color-secondary); margin-top: 2px; }
.status-tag { flex-shrink: 0; }
.log-pills { display: flex; flex-wrap: wrap; gap: 6px; }
.log-pill {
  border: 1px solid var(--cf-border, #e2e8f0); background: #fff;
  color: var(--el-text-color-regular); font-size: 11px; font-weight: 600;
  padding: 4px 12px; border-radius: 999px; cursor: pointer; transition: all 0.15s;
}
.log-pill:hover { border-color: var(--cf-orange, #f6821f); color: var(--cf-orange, #f6821f); }
.log-pill.on { background: var(--el-color-primary-light-9); border-color: var(--cf-orange, #f6821f); color: #c05600; }
.log-pill.selected { background: var(--cf-orange, #f6821f); border-color: var(--cf-orange, #f6821f); color: #fff; }
.log-pill.missing { opacity: 0.7; border-style: dashed; }
.log-pill.loading { opacity: 0.5; pointer-events: none; }
.no-logs { margin: 0; font-size: 12px; color: var(--el-text-color-placeholder); }

.logs-layout { display: grid; grid-template-columns: 1fr; gap: 16px; flex: 1; min-height: 0; }
.logs-layout:has(.sources-panel) { grid-template-columns: 260px 1fr; }
.logs-layout.with-ai:has(.sources-panel) { grid-template-columns: 240px 1fr 300px; }
.logs-layout.with-ai:not(:has(.sources-panel)) { grid-template-columns: 1fr 300px; }

.sources-panel {
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px; overflow: auto; max-height: calc(100vh - 420px);
}
.sources-head {
  padding: 10px 14px; font-weight: 600; font-size: 13px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  position: sticky; top: 0; background: var(--el-bg-color); z-index: 1;
}
.source-item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 8px 14px; cursor: pointer; border-bottom: 1px solid var(--el-border-color-extra-light);
}
.source-item:hover { background: var(--el-fill-color-light); }
.source-item.active { background: var(--el-color-primary-light-9); }
.source-item.off { opacity: 0.55; cursor: default; }
.source-name { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-title { display: flex; align-items: center; justify-content: space-between; width: 100%; padding-right: 8px; font-size: 13px; gap: 10px; }
.ios-switch {
  position: relative;
  width: 44px;
  height: 26px;
  border: none;
  border-radius: 999px;
  background: #e9e9ea;
  padding: 0;
  cursor: pointer;
  transition: background-color 0.25s ease;
  flex-shrink: 0;
}
.ios-switch.on { background: #34c759; }
.ios-switch:disabled,
.ios-switch.loading { opacity: 0.45; cursor: not-allowed; }
.ios-switch-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.28);
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.ios-switch.on .ios-switch-knob { transform: translateX(18px); }
.ios-switch.sm { width: 38px; height: 22px; }
.ios-switch.sm .ios-switch-knob { width: 18px; height: 18px; top: 2px; left: 2px; }
.ios-switch.sm.on .ios-switch-knob { transform: translateX(16px); }

.viewer-panel {
  display: flex; flex-direction: column; border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px; overflow: hidden; min-height: 360px;
}
.viewer-head {
  display: flex; justify-content: space-between; align-items: center;
  padding: 10px 14px; border-bottom: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-blank);
}
.viewer-title { display: flex; align-items: flex-start; gap: 10px; font-size: 13px; }
.app-from { display: block; font-size: 11px; color: var(--el-text-color-secondary); margin-top: 2px; }
.path { display: block; font-size: 11px; color: var(--el-text-color-secondary); margin-top: 2px; word-break: break-all; }
.size { color: var(--el-text-color-secondary); font-size: 12px; flex-shrink: 0; }
.log-pre {
  flex: 1; margin: 0; padding: 14px; overflow: auto; min-height: 280px;
  background: #1e1e1e; color: #d4d4d4; font-family: Consolas, 'Courier New', monospace;
  font-size: 12px; line-height: 1.5; white-space: pre-wrap; word-break: break-all;
}
.ai-panel {
  width: 380px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  max-height: calc(100vh - 420px);
}

@media (max-width: 1100px) {
  .logs-layout.with-ai { grid-template-columns: 1fr !important; }
}
</style>
