<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { isChineseLocale } from '@/locales'
import api, { resolveApiError } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import TrafficMap from '@/components/TrafficMap.vue'
import CrawlerIcon from '@/components/CrawlerIcon.vue'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const { t, locale } = useI18n()

const activeTab = ref('rate')
const loading = ref(false)
const applying = ref(false)
const preview = ref('')
const rules = ref<any[]>([])
const blacklist = ref<any[]>([])
const whitelist = ref<any[]>([])
const status = ref<any>({})
const geoStatus = ref<any>({})
const countries = ref<{ code: string; name: string; zh: string }[]>([])
const selectedCountries = ref<string[]>([])
const securityLogContent = ref('')
const securityLogLoading = ref(false)
const securityLogMeta = ref({ path: '', size: 0, exists: false })

const websites = ref<{ id: number; domain: string }[]>([])
const crawlerSiteId = ref(0)
const crawlers = ref<any[]>([])
const crawlerSaving = ref(false)
const crawlerApplying = ref(false)

const config = reactive({
  rate_limit_enabled: true,
  rate_limit_rate: '10r/s',
  rate_limit_burst: 20,
  rate_limit_nodelay: true,
  conn_limit_enabled: true,
  conn_limit_per_ip: 50,
  geo_block_enabled: false,
  geo_mode: 'block',
  blocked_countries: '',
  geo_db_path: '',
  blacklist_enabled: true,
  whitelist_enabled: false,
  allow_search_bots: true,
  block_headless_bots: true,
  block_http_methods: 'TRACE,TRACK,DEBUG,CONNECT',
  slow_attack_enabled: true,
  client_body_timeout_sec: 12,
  client_header_timeout_sec: 12,
  api_rate_limit_enabled: false,
  api_rate_limit_rate: '30r/s',
  api_rate_limit_burst: 60,
  hotlink_enabled: false,
  hotlink_allow_empty: true,
  hotlink_allow_domains: '',
  header_preset: 'custom',
  filter_enabled: true,
  block_bad_user_agent: true,
  block_scanner_ua: true,
  headers_enabled: true,
  csp: "default-src 'self'; script-src 'self' 'unsafe-inline'",
  x_frame_options: 'SAMEORIGIN',
  hsts_enabled: true,
  hsts_max_age: 31536000,
  x_content_type_options: true,
  referrer_policy: 'strict-origin-when-cross-origin',
  log_format_enabled: true,
  security_log_path: '',
})

const countryLabel = computed(() => (c: { code: string; name: string; zh: string }) =>
  isChineseLocale(locale.value) ? `${c.zh} (${c.code})` : `${c.name} (${c.code})`,
)

const geoDbSizeText = computed(() => {
  const size = geoStatus.value?.db_size || 0
  if (size <= 0) return '-'
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(2)} MB`
})

function syncCountriesFromConfig() {
  selectedCountries.value = (config.blocked_countries || '')
    .split(',')
    .map((s) => s.trim().toUpperCase())
    .filter(Boolean)
}

function syncConfigFromCountries() {
  config.blocked_countries = selectedCountries.value.join(',')
}

async function loadSecurityLog() {
  securityLogLoading.value = true
  try {
    const res: any = await api.get('/waf/logs/tail', { params: { lines: 300 } })
    const data = res.data || {}
    securityLogMeta.value = {
      path: data.path || config.security_log_path,
      size: data.size || 0,
      exists: !!data.exists,
    }
    if (!data.exists) {
      securityLogContent.value = t('waf.logFileMissing', { path: securityLogMeta.value.path })
    } else if (!String(data.content || '').trim()) {
      securityLogContent.value = t('waf.logEmpty')
    } else {
      securityLogContent.value = data.content
    }
  } catch {
    securityLogContent.value = t('waf.logFileMissing', { path: config.security_log_path })
  } finally {
    securityLogLoading.value = false
  }
}

watch(activeTab, (tab) => {
  if (tab === 'log') loadSecurityLog()
  if (tab === 'crawlers') loadCrawlerRules()
})

async function loadWebsites() {
  try {
    const res: any = await api.get('/websites')
    websites.value = (res.data || []).map((w: any) => ({ id: w.id, domain: w.domain }))
  } catch {
    websites.value = []
  }
}

async function loadCrawlerRules() {
  loading.value = true
  try {
    const res: any = await api.get('/waf/crawlers/rules', { params: { website_id: crawlerSiteId.value } })
    crawlers.value = (res.data?.crawlers || []).map((c: any) => ({
      ...c,
      action: c.configured_action || (crawlerSiteId.value === 0 ? c.default_action : 'inherit'),
    }))
  } finally {
    loading.value = false
  }
}

async function persistCrawlerRules() {
  await api.put('/waf/crawlers/rules', {
    website_id: Number(crawlerSiteId.value),
    rules: crawlers.value.map((c) => ({ crawler_id: c.id, action: c.action })),
  })
}

async function saveCrawlerRules() {
  crawlerSaving.value = true
  try {
    await persistCrawlerRules()
    ElMessage.success(t('waf.crawlerSaved'))
    await loadCrawlerRules()
    loadAll()
  } catch (e: unknown) {
    ElMessage.error(resolveApiError(e, t('common.failed')))
  } finally {
    crawlerSaving.value = false
  }
}

async function applyCrawlerRules() {
  crawlerApplying.value = true
  try {
    await persistCrawlerRules()
    const res: any = await api.post(
      '/waf/crawlers/apply',
      { website_id: Number(crawlerSiteId.value) },
      { timeout: 120000 },
    )
    ElMessage.success(res?.data?.message || t('waf.crawlerApplied'))
    await loadCrawlerRules()
    loadAll()
  } catch (e: unknown) {
    ElMessage.error(resolveApiError(e, t('waf.applyFailed')))
  } finally {
    crawlerApplying.value = false
  }
}

function crawlerActionLabel(action: string) {
  if (action === 'allow') return t('waf.crawlerAllow')
  if (action === 'block') return t('waf.crawlerBlock')
  return t('waf.crawlerInherit')
}

function crawlerActionTag(action: string): 'success' | 'danger' | 'info' {
  if (action === 'allow') return 'success'
  if (action === 'block') return 'danger'
  return 'info'
}

function crawlerDisplayName(row: { icon?: string; name?: string }) {
  const key = `waf.crawlerIcons.${row.icon || ''}`
  const translated = t(key)
  return translated === key ? (row.name || row.icon || '') : translated
}

async function loadAll() {
  loading.value = true
  try {
    const [cfgRes, rulesRes, blRes, wlRes, stRes, geoRes, countriesRes]: any[] = await Promise.all([
      api.get('/waf/config'),
      api.get('/waf'),
      api.get('/waf/blacklist'),
      api.get('/waf/whitelist'),
      api.get('/waf/status'),
      api.get('/waf/geoip/status'),
      api.get('/waf/geoip/countries'),
    ])
    Object.assign(config, cfgRes.data || {})
    syncCountriesFromConfig()
    rules.value = rulesRes.data || []
    blacklist.value = blRes.data || []
    whitelist.value = wlRes.data || []
    status.value = stRes.data || {}
    geoStatus.value = geoRes.data || {}
    countries.value = countriesRes.data || []
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  syncConfigFromCountries()
  await api.put('/waf/config', config)
  ElMessage.success(t('waf.saved'))
  loadAll()
}

async function applyConfig() {
  applying.value = true
  try {
    syncConfigFromCountries()
    await saveConfig()
    const res: any = await api.post('/waf/apply')
    ElMessage.success(res.data?.message || res.message || t('waf.applied'))
    if (res.data?.preview) preview.value = res.data.preview
  } catch (e: any) {
    ElMessage.error(e?.error || t('waf.applyFailed'))
  } finally {
    applying.value = false
  }
}

async function loadPreview() {
  const res: any = await api.get('/waf/preview')
  preview.value = res.data?.preview || ''
}

async function addRule() {
  await api.post('/waf', ruleForm.value)
  ruleDialog.value = false
  ElMessage.success(t('common.success'))
  loadAll()
}

async function toggleRule(row: any) {
  await api.patch(`/waf/${row.id}/toggle`, { enabled: !row.enabled })
  loadAll()
}

async function deleteRule(id: number) {
  await ElMessageBox.confirm(t('common.confirmDelete'), t('common.warning'), { type: 'warning' })
  await api.delete(`/waf/${id}`)
  loadAll()
}

async function addBlacklist() {
  await api.post('/waf/blacklist', blForm.value)
  blForm.value = { ip: '', reason: '' }
  ElMessage.success(t('common.success'))
  loadAll()
}

async function importBlacklist() {
  const res: any = await api.post('/waf/blacklist/import', { text: blBatch.value, reason: 'batch' })
  ElMessage.success(t('waf.imported', { n: res.data?.imported || 0 }))
  blBatch.value = ''
  loadAll()
}

async function deleteBlacklist(id: number) {
  await api.delete(`/waf/blacklist/${id}`)
  loadAll()
}

async function addWhitelist() {
  await api.post('/waf/whitelist', wlForm.value)
  wlForm.value = { ip: '', reason: '' }
  ElMessage.success(t('common.success'))
  loadAll()
}

async function importWhitelist() {
  const res: any = await api.post('/waf/whitelist/import', { text: wlBatch.value, reason: 'batch' })
  ElMessage.success(t('waf.imported', { n: res.data?.imported || 0 }))
  wlBatch.value = ''
  loadAll()
}

async function deleteWhitelist(id: number) {
  await api.delete(`/waf/whitelist/${id}`)
  loadAll()
}

const headerPresets = {
  strict: {
    csp: "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'",
    x_frame_options: 'DENY',
    hsts_enabled: true,
    hsts_max_age: 63072000,
    x_content_type_options: true,
    referrer_policy: 'no-referrer',
  },
  balanced: {
    csp: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:",
    x_frame_options: 'SAMEORIGIN',
    hsts_enabled: true,
    hsts_max_age: 31536000,
    x_content_type_options: true,
    referrer_policy: 'strict-origin-when-cross-origin',
  },
} as const

function applyHeaderPreset(preset: string) {
  if (preset === 'custom') return
  if (preset === 'none') {
    config.headers_enabled = false
    return
  }
  config.headers_enabled = true
  const p = headerPresets[preset as keyof typeof headerPresets]
  if (!p) return
  Object.assign(config, p)
}

const headerFieldsReadonly = computed(() =>
  config.header_preset !== 'custom' && config.header_preset !== 'none',
)

watch(() => config.header_preset, (preset, prev) => {
  if (prev === undefined) return
  applyHeaderPreset(preset)
})

const ruleForm = ref({ name: '', type: 'uri', pattern: '', action: 'block', enabled: true })
const ruleDialog = ref(false)
const blForm = ref({ ip: '', reason: '' })
const blBatch = ref('')
const wlForm = ref({ ip: '', reason: '' })
const wlBatch = ref('')

const guideSteps = [
  'waf.guide.step1',
  'waf.guide.step2',
  'waf.guide.step3',
  'waf.guide.step4',
  'waf.guide.step5',
  'waf.guide.step6',
]

const featureMapping = [
  { capability: 'waf.guide.map.kona', openpanel: 'waf.guide.map.konaOp' },
  { capability: 'waf.guide.map.botManager', openpanel: 'waf.guide.map.botManagerOp' },
  { capability: 'waf.guide.map.clientRep', openpanel: 'waf.guide.map.clientRepOp' },
  { capability: 'waf.guide.map.geoFence', openpanel: 'waf.guide.map.geoFenceOp' },
  { capability: 'waf.guide.map.rateControl', openpanel: 'waf.guide.map.rateControlOp' },
  { capability: 'waf.guide.map.apiProtect', openpanel: 'waf.guide.map.apiProtectOp' },
  { capability: 'waf.guide.map.hotlink', openpanel: 'waf.guide.map.hotlinkOp' },
  { capability: 'waf.guide.map.slowPost', openpanel: 'waf.guide.map.slowPostOp' },
  { capability: 'waf.guide.map.ipWhite', openpanel: 'waf.guide.map.ipWhiteOp' },
  { capability: 'waf.guide.map.securityHeaders', openpanel: 'waf.guide.map.securityHeadersOp' },
]

type StatusItem = {
  key: string
  label: string
  value: string
  tagType?: 'success' | 'warning' | 'info' | 'danger'
  hint?: string
}

const statusItems = computed<StatusItem[]>(() => {
  const s = status.value || {}
  const on = (v: boolean) => (v ? t('waf.status.enabled') : t('waf.status.disabled'))
  const onTag = (v: boolean): StatusItem['tagType'] => (v ? 'success' : 'info')

  const geoEnabled = !!s.geo_block
  const geoValue = geoEnabled
    ? t('waf.status.countries', { n: s.geo_countries || 0 })
    : t('waf.status.disabled')
  const geoHint = s.geo_db_exists ? t('waf.status.geoDbReady') : t('waf.status.geoDbMissing')

  return [
    {
      key: 'rate_limit',
      label: t('waf.status.rateLimit'),
      value: on(!!s.rate_limit),
      tagType: onTag(!!s.rate_limit),
    },
    {
      key: 'conn_limit',
      label: t('waf.status.connLimit'),
      value: on(!!s.conn_limit),
      tagType: onTag(!!s.conn_limit),
    },
    {
      key: 'whitelist',
      label: t('waf.status.whitelist'),
      value: t('waf.status.ips', { n: s.whitelist_count ?? 0 }),
      tagType: (s.whitelist_count ?? 0) > 0 ? 'success' : 'info',
    },
    {
      key: 'blacklist',
      label: t('waf.status.blacklist'),
      value: t('waf.status.ips', { n: s.blacklist_count ?? 0 }),
      tagType: (s.blacklist_count ?? 0) > 0 ? 'warning' : 'info',
    },
    {
      key: 'edge',
      label: t('waf.status.edgePolicies'),
      value: edgePoliciesSummary(s),
      tagType: edgePoliciesTag(s),
    },
    {
      key: 'geo',
      label: t('waf.status.geo'),
      value: geoValue,
      tagType: geoEnabled ? 'success' : 'info',
      hint: geoHint,
    },
    {
      key: 'crawler',
      label: t('waf.status.crawler'),
      value: t('waf.status.crawlerSummary', {
        blocked: s.crawler_rules?.global_blocked ?? 0,
        overrides: s.crawler_rules?.site_overrides ?? 0,
      }),
      tagType: (s.crawler_rules?.global_blocked ?? 0) > 0 ? 'warning' : 'success',
    },
    {
      key: 'filter',
      label: t('waf.status.filter'),
      value: t('waf.status.rules', { n: s.filter_rules ?? 0 }),
      tagType: (s.filter_rules ?? 0) > 0 ? 'success' : 'warning',
    },
    {
      key: 'headers',
      label: t('waf.status.headers'),
      value: on(!!s.headers),
      tagType: onTag(!!s.headers),
    },
    {
      key: 'security_log',
      label: t('waf.status.securityLog'),
      value: on(!!s.security_log),
      tagType: onTag(!!s.security_log),
    },
    {
      key: 'conf',
      label: t('waf.status.nginxConf'),
      value: s.conf_exists ? t('waf.status.confReady') : t('waf.status.confMissing'),
      tagType: s.conf_exists ? 'success' : 'warning',
    },
  ]
})

function edgePoliciesSummary(s: Record<string, any>) {
  const ep = s.edge_policies || {}
  let n = 0
  if (ep.whitelist_enabled) n++
  if (ep.allow_search_bots || ep.block_headless_bots) n++
  if (ep.block_http_methods) n++
  if (ep.slow_attack) n++
  if (ep.api_rate_limit) n++
  if (ep.hotlink) n++
  if (ep.header_preset && ep.header_preset !== 'custom' && ep.header_preset !== 'none') n++
  return n > 0 ? t('waf.status.edgeActive', { n }) : t('waf.status.disabled')
}

function edgePoliciesTag(s: Record<string, any>): StatusItem['tagType'] {
  const ep = s.edge_policies || {}
  const active = ep.whitelist_enabled || ep.slow_attack || ep.api_rate_limit || ep.hotlink
  return active ? 'success' : 'info'
}

onMounted(async () => {
  await loadWebsites()
  loadAll()
})
</script>

<template>
  <div>
    <div class="page-header" :class="{ 'page-header--embedded': props.embedded }">
      <h2 v-if="!props.embedded">{{ t('page.waf') }}</h2>
      <div class="header-actions">
        <el-button @click="loadPreview">{{ t('waf.preview') }}</el-button>
        <el-button type="primary" :loading="applying" @click="applyConfig">{{ t('waf.apply') }}</el-button>
      </div>
    </div>

    <el-card shadow="never" class="status-overview" style="margin-bottom: 16px">
      <template #header>
        <span class="status-overview-title">{{ t('waf.statusOverview') }}</span>
      </template>
      <el-row :gutter="12">
        <el-col v-for="item in statusItems" :key="item.key" :xs="12" :sm="8" :md="6" :lg="3">
          <div class="status-item">
            <div class="status-item-label">{{ item.label }}</div>
            <el-tag :type="item.tagType" size="small" effect="light" round>{{ item.value }}</el-tag>
            <div v-if="item.hint" class="status-item-hint">{{ item.hint }}</div>
          </div>
        </el-col>
      </el-row>
    </el-card>

    <el-tabs v-model="activeTab" type="border-card">
      <el-tab-pane :label="t('waf.tab.rate')" name="rate">
        <el-form label-width="140px" style="max-width: 560px">
          <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.rate_limit_enabled" /></el-form-item>
          <el-form-item :label="t('waf.rate')"><el-input v-model="config.rate_limit_rate" placeholder="10r/s" /></el-form-item>
          <el-form-item :label="t('waf.burst')"><el-input-number v-model="config.rate_limit_burst" :min="1" :max="1000" /></el-form-item>
          <el-form-item :label="t('waf.nodelay')"><el-switch v-model="config.rate_limit_nodelay" /></el-form-item>
          <el-alert :title="t('waf.rateHint')" type="info" :closable="false" show-icon />
        </el-form>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.conn')" name="conn">
        <el-form label-width="160px" style="max-width: 560px">
          <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.conn_limit_enabled" /></el-form-item>
          <el-form-item :label="t('waf.connPerIp')"><el-input-number v-model="config.conn_limit_per_ip" :min="1" :max="500" /></el-form-item>
          <el-alert :title="t('waf.connHint')" type="info" :closable="false" show-icon />
        </el-form>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.edge')" name="edge">
        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionWhitelist') }}</span></template>
          <el-form label-width="160px" style="max-width: 720px">
            <el-form-item :label="t('waf.whitelistEnabled')"><el-switch v-model="config.whitelist_enabled" /></el-form-item>
          </el-form>
          <div class="sub-header"><span>{{ t('waf.ipWhitelist') }}</span></div>
          <el-row :gutter="12" style="margin-bottom: 12px">
            <el-col :span="8"><el-input v-model="wlForm.ip" :placeholder="t('waf.ipPlaceholder')" /></el-col>
            <el-col :span="10"><el-input v-model="wlForm.reason" :placeholder="t('waf.reason')" /></el-col>
            <el-col :span="6"><el-button type="primary" @click="addWhitelist">{{ t('waf.addIp') }}</el-button></el-col>
          </el-row>
          <el-input v-model="wlBatch" type="textarea" :rows="3" :placeholder="t('waf.batchPlaceholder')" style="margin-bottom: 8px" />
          <el-button size="small" @click="importWhitelist">{{ t('waf.batchImport') }}</el-button>
          <el-table :data="whitelist" stripe style="margin-top: 16px">
            <el-table-column prop="ip" :label="t('waf.ip')" width="160" />
            <el-table-column prop="reason" :label="t('waf.reason')" />
            <el-table-column prop="source" :label="t('waf.source')" width="100" />
            <el-table-column :label="t('common.actions')" width="100">
              <template #default="{ row }">
                <el-button type="danger" text size="small" @click="deleteWhitelist(row.id)">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionBotManager') }}</span></template>
          <el-form label-width="200px" style="max-width: 560px">
            <el-form-item :label="t('waf.allowSearchBots')"><el-switch v-model="config.allow_search_bots" /></el-form-item>
            <el-form-item :label="t('waf.blockHeadlessBots')"><el-switch v-model="config.block_headless_bots" /></el-form-item>
            <el-alert :title="t('waf.botManagerHint')" type="info" :closable="false" show-icon />
          </el-form>
        </el-card>

        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionHttpMethods') }}</span></template>
          <el-form label-width="200px" style="max-width: 640px">
            <el-form-item :label="t('waf.blockHttpMethods')">
              <el-input v-model="config.block_http_methods" placeholder="TRACE,TRACK,DEBUG,CONNECT" />
            </el-form-item>
            <el-alert :title="t('waf.blockHttpMethodsHint')" type="info" :closable="false" show-icon />
          </el-form>
        </el-card>

        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionSlowAttack') }}</span></template>
          <el-form label-width="200px" style="max-width: 560px">
            <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.slow_attack_enabled" /></el-form-item>
            <el-form-item :label="t('waf.clientBodyTimeout')"><el-input-number v-model="config.client_body_timeout_sec" :min="1" :max="120" /></el-form-item>
            <el-form-item :label="t('waf.clientHeaderTimeout')"><el-input-number v-model="config.client_header_timeout_sec" :min="1" :max="120" /></el-form-item>
            <el-alert :title="t('waf.slowAttackHint')" type="info" :closable="false" show-icon />
          </el-form>
        </el-card>

        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionApiRateLimit') }}</span></template>
          <el-form label-width="200px" style="max-width: 560px">
            <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.api_rate_limit_enabled" /></el-form-item>
            <el-form-item :label="t('waf.apiRateLimitRate')"><el-input v-model="config.api_rate_limit_rate" placeholder="30r/s" /></el-form-item>
            <el-form-item :label="t('waf.apiRateLimitBurst')"><el-input-number v-model="config.api_rate_limit_burst" :min="1" :max="2000" /></el-form-item>
            <el-alert :title="t('waf.apiRateLimitHint')" type="info" :closable="false" show-icon />
          </el-form>
        </el-card>

        <el-card shadow="never" class="edge-section">
          <template #header><span class="section-title">{{ t('waf.sectionHotlink') }}</span></template>
          <el-form label-width="200px" style="max-width: 640px">
            <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.hotlink_enabled" /></el-form-item>
            <el-form-item :label="t('waf.hotlinkAllowEmpty')"><el-switch v-model="config.hotlink_allow_empty" /></el-form-item>
            <el-form-item :label="t('waf.hotlinkAllowDomains')">
              <el-input v-model="config.hotlink_allow_domains" type="textarea" :rows="2" :placeholder="t('waf.hotlinkDomainsPlaceholder')" />
            </el-form-item>
            <el-alert :title="t('waf.hotlinkHint')" type="info" :closable="false" show-icon />
          </el-form>
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.access')" name="access">
        <el-form label-width="140px" style="max-width: 640px; margin-bottom: 20px">
          <el-form-item :label="t('waf.blacklistEnabled')"><el-switch v-model="config.blacklist_enabled" /></el-form-item>
        </el-form>
        <div class="sub-header">
          <span>{{ t('waf.ipBlacklist') }}</span>
        </div>
        <el-row :gutter="12" style="margin-bottom: 12px">
          <el-col :span="8"><el-input v-model="blForm.ip" :placeholder="t('waf.ipPlaceholder')" /></el-col>
          <el-col :span="10"><el-input v-model="blForm.reason" :placeholder="t('waf.reason')" /></el-col>
          <el-col :span="6"><el-button type="primary" @click="addBlacklist">{{ t('waf.addIp') }}</el-button></el-col>
        </el-row>
        <el-input v-model="blBatch" type="textarea" :rows="3" :placeholder="t('waf.batchPlaceholder')" style="margin-bottom: 8px" />
        <el-button size="small" @click="importBlacklist">{{ t('waf.batchImport') }}</el-button>
        <el-table :data="blacklist" stripe style="margin-top: 16px">
          <el-table-column prop="ip" :label="t('waf.ip')" width="160" />
          <el-table-column prop="reason" :label="t('waf.reason')" />
          <el-table-column prop="source" :label="t('waf.source')" width="100" />
          <el-table-column :label="t('common.actions')" width="100">
            <template #default="{ row }">
              <el-button type="danger" text size="small" @click="deleteBlacklist(row.id)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 国家/地区访问限制 -->
      <el-tab-pane :label="t('waf.tab.geo')" name="geo">
        <el-form label-width="160px" style="max-width: 760px">
          <el-form-item :label="t('waf.enabled')">
            <el-switch v-model="config.geo_block_enabled" />
          </el-form-item>
          <el-form-item :label="t('waf.geoMode')">
            <el-radio-group v-model="config.geo_mode">
              <el-radio value="block">{{ t('waf.geoModeBlock') }}</el-radio>
              <el-radio value="allow">{{ t('waf.geoModeAllow') }}</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item :label="t('waf.geoCountries')">
            <el-select
              v-model="selectedCountries"
              multiple
              filterable
              collapse-tags
              collapse-tags-tooltip
              :placeholder="t('waf.geoCountries')"
              style="width: 100%"
            >
              <el-option
                v-for="c in countries"
                :key="c.code"
                :label="countryLabel(c)"
                :value="c.code"
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('waf.geoDbPath')">
            <el-input v-model="config.geo_db_path" :placeholder="geoStatus.db_path || ''" />
          </el-form-item>
          <el-form-item :label="t('waf.geoDbStatus')">
            <el-tag :type="geoStatus.db_exists ? 'success' : 'warning'">
              {{ geoStatus.db_exists ? t('waf.geoDbReady') : t('waf.geoDbMissing') }}
            </el-tag>
            <span v-if="geoStatus.db_exists" class="geo-meta">{{ t('waf.geoDbSize') }}: {{ geoDbSizeText }}</span>
          </el-form-item>
          <el-alert :title="t('waf.geoSelectHint')" type="info" :closable="false" show-icon style="margin-bottom: 12px" />
          <el-alert :title="geoStatus.setup_hint || t('waf.geoSetupHint')" type="warning" :closable="false" show-icon />
        </el-form>

        <el-divider>{{ t('traffic.title') }}</el-divider>
        <TrafficMap compact />
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.filter')" name="filter">
        <el-form label-width="140px" style="max-width: 560px; margin-bottom: 16px">
          <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.filter_enabled" /></el-form-item>
          <el-form-item :label="t('waf.blockScanner')"><el-switch v-model="config.block_scanner_ua" /></el-form-item>
          <el-form-item :label="t('waf.blockBadUa')"><el-switch v-model="config.block_bad_user_agent" /></el-form-item>
        </el-form>
        <div class="sub-header">
          <span>{{ t('waf.filterRules') }}</span>
          <el-button size="small" type="primary" @click="ruleDialog = true">{{ t('waf.addRule') }}</el-button>
        </div>
        <el-table :data="rules" stripe>
          <el-table-column prop="name" :label="t('waf.ruleName')" />
          <el-table-column prop="type" :label="t('common.type')" width="80" />
          <el-table-column prop="pattern" :label="t('waf.pattern')" show-overflow-tooltip />
          <el-table-column prop="enabled" :label="t('waf.enabled')" width="80">
            <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggleRule(row)" /></template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="90">
            <template #default="{ row }"><el-button type="danger" text size="small" @click="deleteRule(row.id)">{{ t('common.delete') }}</el-button></template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.crawlers')" name="crawlers">
        <el-alert :title="t('waf.crawlerHint')" type="info" :closable="false" show-icon style="margin-bottom: 16px" />
        <el-form inline style="margin-bottom: 16px">
          <el-form-item :label="t('waf.crawlerScope')">
            <el-select v-model="crawlerSiteId" style="width: 280px" @change="loadCrawlerRules">
              <el-option :label="t('waf.crawlerGlobal')" :value="0" />
              <el-option v-for="w in websites" :key="w.id" :label="w.domain" :value="w.id" />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="crawlerSaving" @click="saveCrawlerRules">{{ t('common.save') }}</el-button>
            <el-button type="success" :loading="crawlerApplying" @click="applyCrawlerRules">{{ t('waf.crawlerApply') }}</el-button>
          </el-form-item>
        </el-form>
        <el-table :data="crawlers" stripe v-loading="loading">
          <el-table-column :label="t('waf.crawlerName')" min-width="200">
            <template #default="{ row }">
              <div class="crawler-cell">
                <CrawlerIcon :icon="row.icon" :name="crawlerDisplayName(row)" :size="26" />
                <div class="crawler-cell-text">
                  <span class="crawler-name">{{ crawlerDisplayName(row) }}</span>
                  <span v-if="row.patterns?.[0]" class="crawler-sub">{{ row.patterns[0] }}</span>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('waf.crawlerPatterns')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ (row.patterns || []).join(', ') }}</template>
          </el-table-column>
          <el-table-column :label="t('waf.crawlerDefault')" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="crawlerActionTag(row.default_action)">{{ crawlerActionLabel(row.default_action) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('waf.crawlerEffective')" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="crawlerActionTag(row.effective_action)">{{ crawlerActionLabel(row.effective_action) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('waf.crawlerAction')" width="160">
            <template #default="{ row }">
              <el-select v-model="row.action" size="small" style="width: 120px">
                <el-option v-if="crawlerSiteId > 0" :label="t('waf.crawlerInherit')" value="inherit" />
                <el-option :label="t('waf.crawlerAllow')" value="allow" />
                <el-option :label="t('waf.crawlerBlock')" value="block" />
              </el-select>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.guide')" name="guide">
        <el-card shadow="never" class="waf-guide">
          <template #header><span class="section-title">{{ t('waf.guide.title') }}</span></template>
          <p class="guide-intro">{{ t('waf.guide.intro') }}</p>
          <ol class="guide-steps">
            <li v-for="step in guideSteps" :key="step">{{ t(step) }}</li>
          </ol>

          <h4 class="guide-subtitle">{{ t('waf.guide.mappingTitle') }}</h4>
          <p class="guide-intro">{{ t('waf.guide.mappingIntro') }}</p>
          <el-table :data="featureMapping" size="small" stripe class="guide-table">
            <el-table-column :label="t('waf.guide.mappingCapability')" min-width="200">
              <template #default="{ row }">{{ t(row.capability) }}</template>
            </el-table-column>
            <el-table-column :label="t('waf.guide.mappingOWPanel')" min-width="280">
              <template #default="{ row }">{{ t(row.openpanel) }}</template>
            </el-table-column>
          </el-table>

          <h4 class="guide-subtitle">{{ t('waf.guide.nginxTitle') }}</h4>
          <p class="guide-intro">{{ t('waf.guide.nginxIntro') }}</p>
          <pre class="preview-box guide-code">{{ t('waf.guide.nginxSnippet') }}</pre>

          <p class="guide-footer">
            {{ t('waf.guide.protectionHint') }}
            <RouterLink to="/protection?tab=security">{{ t('waf.guide.protectionLink') }}</RouterLink>
          </p>
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.headers')" name="headers">
        <el-form label-width="180px" style="max-width: 720px">
          <el-form-item :label="t('waf.headerPreset')">
            <el-select v-model="config.header_preset" style="width: 220px">
              <el-option :label="t('waf.headerPresetStrict')" value="strict" />
              <el-option :label="t('waf.headerPresetBalanced')" value="balanced" />
              <el-option :label="t('waf.headerPresetCustom')" value="custom" />
              <el-option :label="t('waf.headerPresetNone')" value="none" />
            </el-select>
          </el-form-item>
          <el-alert
            v-if="headerFieldsReadonly"
            :title="t('waf.headerPresetReadonly', { preset: t(`waf.headerPreset${config.header_preset === 'strict' ? 'Strict' : 'Balanced'}`) })"
            type="info"
            :closable="false"
            show-icon
            style="margin-bottom: 16px"
          />
          <el-alert
            v-else-if="config.header_preset === 'none'"
            :title="t('waf.headerPresetNoneHint')"
            type="warning"
            :closable="false"
            show-icon
            style="margin-bottom: 16px"
          />
          <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.headers_enabled" :disabled="config.header_preset === 'none'" /></el-form-item>
          <el-form-item label="Content-Security-Policy">
            <el-input v-model="config.csp" type="textarea" :rows="2" :readonly="headerFieldsReadonly" :disabled="config.header_preset === 'none'" />
          </el-form-item>
          <el-form-item label="X-Frame-Options">
            <el-input v-model="config.x_frame_options" :readonly="headerFieldsReadonly" :disabled="config.header_preset === 'none'" />
          </el-form-item>
          <el-form-item :label="t('waf.hsts')"><el-switch v-model="config.hsts_enabled" :disabled="headerFieldsReadonly || config.header_preset === 'none'" /></el-form-item>
          <el-form-item :label="t('waf.hstsMaxAge')"><el-input-number v-model="config.hsts_max_age" :min="0" :disabled="headerFieldsReadonly || config.header_preset === 'none'" /></el-form-item>
          <el-form-item label="X-Content-Type-Options"><el-switch v-model="config.x_content_type_options" :disabled="headerFieldsReadonly || config.header_preset === 'none'" /></el-form-item>
          <el-form-item label="Referrer-Policy"><el-input v-model="config.referrer_policy" :readonly="headerFieldsReadonly" :disabled="config.header_preset === 'none'" /></el-form-item>
        </el-form>
      </el-tab-pane>

      <el-tab-pane :label="t('waf.tab.log')" name="log">
        <el-form label-width="140px" style="max-width: 560px">
          <el-form-item :label="t('waf.enabled')"><el-switch v-model="config.log_format_enabled" /></el-form-item>
          <el-form-item :label="t('waf.logPath')"><el-input v-model="config.security_log_path" /></el-form-item>
          <el-alert :title="t('waf.logHint')" type="info" :closable="false" show-icon />
        </el-form>
        <el-card shadow="never" style="margin-top: 16px">
          <template #header>
            <div class="log-view-header">
              <span>{{ t('waf.status.securityLog') }}</span>
              <el-button size="small" :loading="securityLogLoading" @click="loadSecurityLog">{{ t('waf.logRefresh') }}</el-button>
            </div>
          </template>
          <p v-if="securityLogMeta.path" class="log-meta">
            <code>{{ securityLogMeta.path }}</code>
            <span v-if="securityLogMeta.size"> · {{ (securityLogMeta.size / 1024).toFixed(1) }} KB</span>
          </p>
          <pre class="preview-box log-viewer">{{ securityLogContent || t('waf.logEmpty') }}</pre>
        </el-card>
        <el-card v-if="preview" shadow="never" style="margin-top: 16px">
          <template #header>{{ t('waf.nginxPreview') }}</template>
          <pre class="preview-box">{{ preview }}</pre>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <div style="margin-top: 16px">
      <el-button type="primary" @click="saveConfig">{{ t('common.save') }}</el-button>
    </div>

    <el-dialog v-model="ruleDialog" :title="t('waf.addRule')" width="500px">
      <el-form :model="ruleForm" label-width="80px">
        <el-form-item :label="t('waf.ruleName')"><el-input v-model="ruleForm.name" /></el-form-item>
        <el-form-item :label="t('common.type')">
          <el-select v-model="ruleForm.type">
            <el-option label="uri" value="uri" /><el-option label="sql" value="sql" />
            <el-option label="xss" value="xss" /><el-option label="path" value="path" />
            <el-option label="header" value="header" /><el-option label="ua" value="ua" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('waf.pattern')"><el-input v-model="ruleForm.pattern" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="addRule">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.header-actions { display: flex; gap: 10px; }
.status-overview-title { font-weight: 600; font-size: 15px; }
.status-item {
  padding: 12px 10px;
  margin-bottom: 8px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
  min-height: 72px;
}
.status-item-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
}
.status-item-hint {
  margin-top: 6px;
  font-size: 11px;
  color: var(--el-text-color-placeholder);
}
.sub-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; font-weight: 600; }
.section-title { font-weight: 600; font-size: 14px; }
.edge-section { margin-bottom: 16px; }
.waf-guide { margin-bottom: 8px; }
.guide-intro { margin: 0 0 10px; font-size: 13px; color: var(--el-text-color-secondary); line-height: 1.6; }
.guide-steps { margin: 0 0 14px; padding-left: 20px; line-height: 1.8; font-size: 13px; }
.guide-subtitle { margin: 16px 0 8px; font-size: 14px; font-weight: 600; }
.guide-table { margin-bottom: 12px; }
.crawler-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.crawler-cell-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.crawler-name { font-weight: 500; line-height: 1.3; }
.crawler-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.2;
}
.guide-code { max-height: 120px; margin: 8px 0 12px; }
.guide-footer { margin: 16px 0 0; font-size: 13px; color: var(--el-text-color-secondary); }
.guide-footer a { color: var(--el-color-primary); text-decoration: none; }
.guide-footer a:hover { text-decoration: underline; }
.log-view-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.log-meta { margin: 0 0 10px; font-size: 12px; color: var(--el-text-color-secondary); }
.log-viewer { max-height: 420px; min-height: 160px; }
.preview-box { background: #1e1e1e; color: #d4d4d4; padding: 16px; border-radius: 8px; font-size: 12px; overflow: auto; max-height: 400px; white-space: pre-wrap; }
.geo-meta { margin-left: 12px; color: #909399; font-size: 13px; }
</style>
