<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import SoftwareIcon from '@/components/SoftwareIcon.vue'
import SoftwareInstallLogDialog from '@/components/SoftwareInstallLogDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Link, Refresh, Setting, CircleCheck, VideoPlay } from '@element-plus/icons-vue'
import type { ExtensionInfo } from '@/stores/extensions'
import { useExtensionsStore } from '@/stores/extensions'

interface FeaturedPack {
  id: string
  name: string
  name_en?: string
  description: string
  description_en?: string
  category: string
  icon: string
  accent: string
  app_key: string
  config_route?: string
  tags: string[]
  install_api?: string
  uninstall_api?: string
  logs_api?: string
  installed: boolean
  running: boolean
  status: string
  access_url?: string
}

const router = useRouter()
const { t, locale } = useI18n()
const extStore = useExtensionsStore()

const featured = ref<FeaturedPack[]>([])
const list = ref<ExtensionInfo[]>([])
const loading = ref(false)
const featuredLoading = ref(false)
const reloading = ref(false)
const extensionsDir = ref('')
const loadError = ref('')
const categoryFilter = ref('all')

const installLogVisible = ref(false)
const installTrigger = ref(false)
const installTarget = ref<FeaturedPack | null>(null)
const uninstallingKey = ref('')

const categories = computed(() => [
  { key: 'all', label: t('extensions.categoryAll') },
  { key: 'analytics', label: t('extensions.categoryAnalytics') },
  { key: 'monitoring', label: t('extensions.categoryMonitoring') },
  { key: 'middleware', label: t('extensions.categoryMiddleware') },
  { key: 'automation', label: t('extensions.categoryAutomation') },
  { key: 'devtools', label: t('extensions.categoryDevtools') },
])

const filteredFeatured = computed(() => {
  if (categoryFilter.value === 'all') return featured.value
  return featured.value.filter((item) => item.category === categoryFilter.value)
})

const installedCount = computed(() => featured.value.filter((item) => item.installed).length)
const runningCount = computed(() => featured.value.filter((item) => item.running).length)

function packName(item: FeaturedPack) {
  return locale.value.startsWith('zh') ? item.name : (item.name_en || item.name)
}

function packDesc(item: FeaturedPack) {
  return locale.value.startsWith('zh') ? item.description : (item.description_en || item.description)
}

function statusLabel(item: FeaturedPack) {
  if (!item.installed) return t('extensions.statusNotInstalled')
  if (item.running) return t('extensions.statusRunning')
  return t('extensions.statusInstalled')
}

function statusType(item: FeaturedPack) {
  if (!item.installed) return 'info'
  if (item.running) return 'success'
  return 'warning'
}

async function loadFeatured() {
  featuredLoading.value = true
  try {
    const res: any = await api.get('/extensions/featured')
    featured.value = res.data?.items || []
  } catch {
    featured.value = []
  } finally {
    featuredLoading.value = false
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const res: any = await api.get('/extensions')
    const data = res.data
    if (Array.isArray(data)) {
      list.value = data
      extensionsDir.value = ''
    } else {
      list.value = data?.items || []
      extensionsDir.value = data?.dir || ''
    }
  } catch (e: any) {
    list.value = []
    loadError.value = e?.error || e?.message || t('extensions.loadFailed')
  } finally {
    loading.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadFeatured(), load()])
}

async function reloadAll() {
  reloading.value = true
  try {
    const res: any = await api.post('/extensions/reload')
    ElMessage.success(res.data?.message || t('extensions.reloaded'))
    await refreshAll()
    await extStore.fetchMenu()
  } catch (e: any) {
    ElMessage.error(e?.error || t('extensions.reloadFailed'))
  } finally {
    reloading.value = false
  }
}

async function toggleEnabled(row: ExtensionInfo, enabled: boolean) {
  try {
    await api.patch(`/extensions/${row.id}/enabled`, { enabled })
    row.enabled = enabled
    ElMessage.success(enabled ? t('extensions.enabled') : t('extensions.disabled'))
    await extStore.fetchMenu()
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
    await load()
  }
}

function installPack(item: FeaturedPack) {
  if (item.app_key === 'huggingface-ai' && item.config_route) {
    router.push(item.config_route)
    return
  }
  installTarget.value = item
  installLogVisible.value = true
  installTrigger.value = true
}

function onInstallDone(payload: { success: boolean }) {
  installTrigger.value = false
  if (payload.success) {
    loadFeatured()
  }
}

async function uninstallPack(item: FeaturedPack) {
  try {
    await ElMessageBox.confirm(
      t('extensions.uninstallConfirm', { name: packName(item) }),
      t('common.warning'),
      { type: 'warning', confirmButtonText: t('common.uninstall'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }

  uninstallingKey.value = item.app_key
  try {
    const path = item.uninstall_api || `/software/${item.app_key}/uninstall`
    await api.post(path)
    ElMessage.success(t('extensions.uninstallDone', { name: packName(item) }))
    await loadFeatured()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('extensions.uninstallFailed'))
  } finally {
    uninstallingKey.value = ''
  }
}

function openConfig(item: FeaturedPack) {
  if (item.config_route) {
    router.push(item.config_route)
    return
  }
  if (item.access_url) {
    window.open(item.access_url, '_blank', 'noopener')
  }
}

const hookEvents = computed(() => [
  'panel.startup',
  'website.created',
  'website.deleted',
  'app.installed',
  'app.uninstalled',
  'backup.completed',
])

onMounted(refreshAll)
</script>

<template>
  <div class="extensions-page">
    <div class="page-header">
      <div>
        <h2>{{ t('extensions.title') }}</h2>
        <p class="subtitle">{{ t('extensions.subtitleNew') }}</p>
      </div>
      <div class="header-actions">
        <el-button :icon="Refresh" :loading="featuredLoading || loading" @click="refreshAll">
          {{ t('common.refresh') }}
        </el-button>
        <el-button type="primary" :loading="reloading" @click="reloadAll">{{ t('extensions.reload') }}</el-button>
      </div>
    </div>

    <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" class="mb-16" />

    <div class="stats-strip">
      <div class="stat-card">
        <span class="stat-label">{{ t('extensions.featuredTotal') }}</span>
        <strong class="stat-value">{{ featured.length }}</strong>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('extensions.featuredInstalled') }}</span>
        <strong class="stat-value">{{ installedCount }}</strong>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('extensions.featuredRunning') }}</span>
        <strong class="stat-value">{{ runningCount }}</strong>
      </div>
      <div class="stat-card stat-card--wide" v-if="extensionsDir">
        <span class="stat-label">{{ t('extensions.customDir') }}</span>
        <code class="dir-code">{{ extensionsDir }}</code>
      </div>
    </div>

    <section class="featured-section">
      <div class="section-head">
        <div>
          <h3>{{ t('extensions.featuredTitle') }}</h3>
          <p class="section-desc">{{ t('extensions.featuredDesc') }}</p>
        </div>
        <el-radio-group v-model="categoryFilter" size="small" class="category-filter">
          <el-radio-button v-for="cat in categories" :key="cat.key" :value="cat.key">
            {{ cat.label }}
          </el-radio-button>
        </el-radio-group>
      </div>

      <div v-loading="featuredLoading" class="featured-grid">
        <article
          v-for="item in filteredFeatured"
          :key="item.id"
          class="pack-card"
          :class="{ 'pack-card--installed': item.installed, 'pack-card--running': item.running }"
        >
          <div class="pack-accent" :style="{ background: `linear-gradient(135deg, ${item.accent}, ${item.accent}88)` }" />
          <div class="pack-body">
            <div class="pack-top">
              <SoftwareIcon :app-key="item.app_key" :size="44" />
              <el-tag :type="statusType(item)" size="small" effect="plain" class="pack-status">
                <el-icon v-if="item.running" class="status-icon"><CircleCheck /></el-icon>
                <el-icon v-else-if="item.installed" class="status-icon"><VideoPlay /></el-icon>
                {{ statusLabel(item) }}
              </el-tag>
            </div>
            <h4 class="pack-title">{{ packName(item) }}</h4>
            <p class="pack-desc">{{ packDesc(item) }}</p>
            <div class="pack-tags">
              <el-tag v-for="tag in item.tags" :key="tag" size="small" round effect="light">{{ tag }}</el-tag>
            </div>
            <div class="pack-actions">
              <el-button
                v-if="!item.installed"
                type="primary"
                @click="installPack(item)"
              >
                {{ t('extensions.installOneClick') }}
              </el-button>
              <template v-else>
                <el-button
                  v-if="item.config_route || item.access_url"
                  type="primary"
                  plain
                  :icon="item.config_route ? Setting : Link"
                  @click="openConfig(item)"
                >
                  {{ item.config_route ? t('extensions.openConfig') : t('extensions.openService') }}
                </el-button>
                <el-button
                  type="danger"
                  plain
                  :loading="uninstallingKey === item.app_key"
                  @click="uninstallPack(item)"
                >
                  {{ t('extensions.removeFeature') }}
                </el-button>
              </template>
            </div>
          </div>
        </article>
      </div>

      <el-empty v-if="!featuredLoading && !filteredFeatured.length" :description="t('extensions.featuredEmpty')" />
    </section>

    <section class="custom-section">
      <div class="section-head">
        <div>
          <h3>{{ t('extensions.customTitle') }}</h3>
          <p class="section-desc">{{ t('extensions.hint') }}</p>
        </div>
      </div>

      <el-table v-loading="loading" :data="list" stripe class="custom-table">
        <el-table-column prop="name" :label="t('common.name')" min-width="140" />
        <el-table-column prop="id" label="ID" width="120" />
        <el-table-column prop="version" :label="t('common.version')" width="80" />
        <el-table-column prop="description" :label="t('common.description')" show-overflow-tooltip />
        <el-table-column :label="t('extensions.hooks')" width="160">
          <template #default="{ row }">
            <el-tag v-for="h in row.hooks" :key="h" size="small" class="hook-tag">{{ h }}</el-tag>
            <span v-if="!row.hooks?.length" class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.status')" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: boolean) => toggleEnabled(row, v)" />
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && !list.length" :description="t('extensions.customEmpty')" />
    </section>

    <el-card shadow="never" class="events-card">
      <template #header>{{ t('extensions.eventsTitle') }}</template>
      <el-tag v-for="ev in hookEvents" :key="ev" class="event-tag">{{ ev }}</el-tag>
    </el-card>

    <SoftwareInstallLogDialog
      v-if="installTarget"
      v-model="installLogVisible"
      :app-key="installTarget.app_key"
      :app-name="packName(installTarget)"
      :trigger-install="installTrigger"
      :install-api-path="installTarget.install_api"
      :logs-api-path="installTarget.logs_api"
      @done="onInstallDone"
    />
  </div>
</template>

<style scoped>
.extensions-page {
  min-height: 100%;
  padding-bottom: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
  gap: 16px;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.subtitle {
  color: var(--el-text-color-secondary);
  margin: 4px 0 0;
  font-size: 13px;
  max-width: 640px;
}

.mb-16 { margin-bottom: 16px; }

.stats-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 24px;
}

.stat-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  padding: 14px 16px;
}

.stat-card--wide {
  grid-column: 1 / -1;
}

.stat-label {
  display: block;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 6px;
}

.stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}

.dir-code {
  font-size: 12px;
  word-break: break-all;
}

.featured-section,
.custom-section {
  margin-bottom: 24px;
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.section-head h3 {
  margin: 0 0 4px;
  font-size: 17px;
  font-weight: 600;
}

.section-desc {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.category-filter {
  flex-wrap: wrap;
}

.featured-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  min-height: 80px;
}

.pack-card {
  position: relative;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 14px;
  overflow: hidden;
  transition: box-shadow 0.2s, transform 0.2s;
}

.pack-card:hover {
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08);
  transform: translateY(-2px);
}

.pack-card--installed {
  border-color: var(--el-color-warning-light-5);
}

.pack-card--running {
  border-color: var(--el-color-success-light-5);
}

.pack-accent {
  height: 4px;
}

.pack-body {
  padding: 16px;
}

.pack-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
}

.pack-status {
  flex-shrink: 0;
}

.status-icon {
  margin-right: 2px;
  vertical-align: -2px;
}

.pack-title {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 600;
  line-height: 1.3;
}

.pack-desc {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--el-text-color-secondary);
  min-height: 40px;
}

.pack-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 14px;
  min-height: 24px;
}

.pack-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.custom-table {
  border-radius: 12px;
  overflow: hidden;
}

.hook-tag { margin: 2px; }
.event-tag { margin: 4px; }
.muted { color: var(--el-text-color-secondary); }

.events-card {
  border-radius: 12px;
}

@media (max-width: 768px) {
  .stats-strip {
    grid-template-columns: 1fr 1fr;
  }

  .page-header {
    flex-direction: column;
  }
}
</style>
