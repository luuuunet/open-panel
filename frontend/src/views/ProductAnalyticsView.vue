<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api, { resolveApiError } from '@/api'
import { ElMessage } from 'element-plus'
import {
  Connection,
  CopyDocument,
  DataAnalysis,
  Link,
  RefreshRight,
  VideoPlay,
} from '@element-plus/icons-vue'

const { t } = useI18n()
const router = useRouter()

const status = ref({
  installed: false,
  running: false,
  dashboard_url: 'http://localhost:3300',
  api_url: 'http://localhost:3333/api',
})
const websites = ref<any[]>([])
const selectedSiteId = ref<number | null>(null)
const loading = ref(false)
const deploying = ref(false)
const saving = ref(false)

const trackingForm = ref({
  product_analytics_enabled: false,
  product_analytics_client_id: '',
  product_analytics_api_url: 'http://localhost:3333/api',
})

const snippet = ref('')
const features = computed(() => [
  t('productAnalytics.featureFunnels'),
  t('productAnalytics.featureReplay'),
  t('productAnalytics.featureCohorts'),
  t('productAnalytics.featureAB'),
  t('productAnalytics.featureCookieless'),
])

const selectedSite = computed(() => websites.value.find((w) => w.id === selectedSiteId.value) || null)

async function loadStatus() {
  const res: any = await api.get('/product-analytics/status')
  status.value = res.data || status.value
}

async function loadWebsites() {
  const res: any = await api.get('/websites')
  websites.value = res.data || []
  if (!selectedSiteId.value && websites.value.length) {
    selectedSiteId.value = websites.value[0].id
  }
}

async function loadSnippet() {
  const res: any = await api.get('/product-analytics/tracking-snippet', {
    params: {
      client_id: trackingForm.value.product_analytics_client_id || undefined,
      api_url: trackingForm.value.product_analytics_api_url || undefined,
    },
  })
  snippet.value = res.data?.snippet || ''
}

function applySiteToForm(site: any) {
  trackingForm.value = {
    product_analytics_enabled: !!site.product_analytics_enabled,
    product_analytics_client_id: site.product_analytics_client_id || '',
    product_analytics_api_url: site.product_analytics_api_url || status.value.api_url,
  }
}

async function refreshAll() {
  loading.value = true
  try {
    await Promise.all([loadStatus(), loadWebsites()])
    if (selectedSite.value) applySiteToForm(selectedSite.value)
    await loadSnippet()
  } finally {
    loading.value = false
  }
}

async function deployFromStore() {
  deploying.value = true
  try {
    await api.post('/software/openpanel-analytics/install', {})
    ElMessage.success(t('productAnalytics.installStarted'))
    router.push({ path: '/software', query: { tab: 'installed', key: 'openpanel-analytics' } })
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('productAnalytics.installFailed')))
  } finally {
    deploying.value = false
  }
}

async function deployFromCompose() {
  deploying.value = true
  try {
    await api.post('/compose', {
      name: 'OpenPanel Analytics',
      path: '/opt/compose/openpanel-analytics',
      scaffold: true,
      template: 'openpanel',
      auto_start: true,
    })
    ElMessage.success(t('productAnalytics.composeStarted'))
    router.push('/compose')
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('productAnalytics.composeFailed')))
  } finally {
    deploying.value = false
  }
}

async function saveTracking() {
  if (!selectedSiteId.value) return
  saving.value = true
  try {
    const res: any = await api.put(`/websites/${selectedSiteId.value}/product-analytics`, trackingForm.value)
    const idx = websites.value.findIndex((w) => w.id === selectedSiteId.value)
    if (idx >= 0) websites.value[idx] = res.data
    ElMessage.success(t('common.saved'))
    await loadSnippet()
  } catch (e: any) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    saving.value = false
  }
}

async function copySnippet() {
  if (!snippet.value) return
  try {
    await navigator.clipboard.writeText(snippet.value)
    ElMessage.success(t('productAnalytics.snippetCopied'))
  } catch {
    ElMessage.error(t('productAnalytics.copyFailed'))
  }
}

function openDashboard() {
  window.open(status.value.dashboard_url, '_blank', 'noopener')
}

watch(selectedSiteId, (id) => {
  const site = websites.value.find((w) => w.id === id)
  if (site) {
    applySiteToForm(site)
    loadSnippet()
  }
})

watch(
  () => [trackingForm.value.product_analytics_client_id, trackingForm.value.product_analytics_api_url],
  () => {
    loadSnippet()
  },
)

onMounted(refreshAll)
</script>

<template>
  <div class="product-analytics-page">
    <div class="page-header">
      <div>
        <h2>{{ t('productAnalytics.title') }}</h2>
        <p class="subtitle">{{ t('productAnalytics.subtitle') }}</p>
      </div>
      <el-button :icon="RefreshRight" :loading="loading" @click="refreshAll">{{ t('common.refresh') }}</el-button>
    </div>

    <el-row :gutter="16" class="feature-row">
      <el-col v-for="item in features" :key="item" :xs="24" :sm="12" :md="8" :lg="4">
        <el-card shadow="never" class="feature-card">
          <el-icon class="feature-icon"><DataAnalysis /></el-icon>
          <span>{{ item }}</span>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :xs="24" :lg="12">
        <el-card shadow="never" class="section-card">
          <template #header>
            <span>{{ t('productAnalytics.installStatus') }}</span>
          </template>
          <div class="status-grid">
            <div class="status-item">
              <span class="label">{{ t('productAnalytics.installed') }}</span>
              <el-tag :type="status.installed ? 'success' : 'info'">
                {{ status.installed ? t('common.yes') : t('common.no') }}
              </el-tag>
            </div>
            <div class="status-item">
              <span class="label">{{ t('productAnalytics.running') }}</span>
              <el-tag :type="status.running ? 'success' : 'warning'">
                {{ status.running ? t('productAnalytics.runningYes') : t('productAnalytics.runningNo') }}
              </el-tag>
            </div>
            <div class="status-item">
              <span class="label">{{ t('productAnalytics.dashboardUrl') }}</span>
              <el-link :href="status.dashboard_url" target="_blank" type="primary">{{ status.dashboard_url }}</el-link>
            </div>
            <div class="status-item">
              <span class="label">{{ t('productAnalytics.apiUrl') }}</span>
              <code>{{ status.api_url }}</code>
            </div>
          </div>
          <div class="action-row">
            <el-button type="primary" :icon="Link" :disabled="!status.running" @click="openDashboard">
              {{ t('productAnalytics.openDashboard') }}
            </el-button>
            <el-button :icon="VideoPlay" :loading="deploying" @click="deployFromStore">
              {{ t('productAnalytics.installFromStore') }}
            </el-button>
            <el-button :icon="Connection" :loading="deploying" @click="deployFromCompose">
              {{ t('productAnalytics.deployCompose') }}
            </el-button>
          </div>
          <el-alert type="info" :closable="false" show-icon class="docs-alert">
            <template #title>
              <a href="https://openpanel.dev/docs" target="_blank" rel="noopener">{{ t('productAnalytics.docsLink') }}</a>
              ·
              <a href="https://github.com/Openpanel-dev/openpanel" target="_blank" rel="noopener">{{ t('productAnalytics.githubLink') }}</a>
            </template>
          </el-alert>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="12">
        <el-card shadow="never" class="section-card">
          <template #header>
            <span>{{ t('productAnalytics.websiteTracking') }}</span>
          </template>
          <el-form label-width="120px">
            <el-form-item :label="t('productAnalytics.selectWebsite')">
              <el-select v-model="selectedSiteId" filterable style="width: 100%">
                <el-option v-for="site in websites" :key="site.id" :label="site.domain" :value="site.id" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('productAnalytics.enabled')">
              <el-switch v-model="trackingForm.product_analytics_enabled" />
            </el-form-item>
            <el-form-item :label="t('productAnalytics.clientId')">
              <el-input v-model="trackingForm.product_analytics_client_id" :placeholder="t('productAnalytics.clientIdHint')" />
            </el-form-item>
            <el-form-item :label="t('productAnalytics.apiUrlField')">
              <el-input v-model="trackingForm.product_analytics_api_url" placeholder="http://localhost:3333/api" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" :disabled="!selectedSiteId" @click="saveTracking">
                {{ t('common.save') }}
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="section-card snippet-card">
      <template #header>
        <div class="snippet-header">
          <span>{{ t('productAnalytics.trackingSnippet') }}</span>
          <el-button text type="primary" :icon="CopyDocument" @click="copySnippet">{{ t('productAnalytics.copySnippet') }}</el-button>
        </div>
      </template>
      <p class="snippet-hint">{{ t('productAnalytics.snippetHint') }}</p>
      <pre class="snippet-box">{{ snippet }}</pre>
    </el-card>
  </div>
</template>

<style scoped>
.product-analytics-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.page-header h2 {
  margin: 0;
}

.subtitle {
  margin: 6px 0 0;
  color: var(--cf-text-muted);
  font-size: 14px;
}

.feature-row {
  margin-bottom: 0;
}

.feature-card {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 56px;
}

.feature-card :deep(.el-card__body) {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
}

.feature-icon {
  color: var(--cf-orange);
  font-size: 18px;
}

.section-card {
  margin-bottom: 16px;
}

.status-grid {
  display: grid;
  gap: 12px;
  margin-bottom: 16px;
}

.status-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.status-item .label {
  color: var(--cf-text-muted);
  font-size: 13px;
}

.action-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.docs-alert {
  margin-top: 8px;
}

.docs-alert a {
  color: var(--cf-orange);
}

.snippet-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.snippet-hint {
  margin: 0 0 12px;
  color: var(--cf-text-muted);
  font-size: 13px;
}

.snippet-box {
  margin: 0;
  padding: 14px 16px;
  background: var(--cf-bg-muted, #f5f7fa);
  border-radius: 8px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
