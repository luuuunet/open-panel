<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { categoryLabel } from '@/locales'
import { normalizeStoreCategory, orderedStoreCategories } from '@/utils/storeCategories'
import {
  appDescription,
  displayAppName,
  iconKeyForApp,
  installedVersionEntries,
  isVersionInstalled,
  resolveInstallKey,
  versionChoices,
} from '@/utils/storeApps'
import SoftwareIcon from '@/components/SoftwareIcon.vue'
import SoftwareConfigDialog from '@/components/SoftwareConfigDialog.vue'
import SoftwareInstallLogDialog from '@/components/SoftwareInstallLogDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()

const ALL = '__all__'
const tab = ref(route.query.tab === 'installed' ? 'installed' : 'store')
const storeApps = ref<any[]>([])
const installedApps = ref<any[]>([])
const category = ref(ALL)
const keyword = ref('')
const loading = ref(false)
const storeLoadFailed = ref(false)
const refreshingVersions = ref(false)
const initializingStore = ref(false)

const installDialog = ref(false)
const upgradeDialog = ref(false)
const installLogVisible = ref(false)
const installLogKey = ref('')
const installLogName = ref('')
const installLogVersion = ref('')
const installLogMode = ref<'install' | 'upgrade'>('install')
const configDialog = ref(false)
const settingsDialog = ref(false)
const currentApp = ref<any>(null)
const installVersion = ref('')
const upgradeVersion = ref('')
const settingsForm = ref({ port: 0, auto_start: true, version: '', bind_domain: '' })
const versionsRefreshedThisSession = ref(false)

const categories = computed(() => {
  const raw = storeApps.value.map(a => a.category).filter(Boolean)
  return [ALL, ...orderedStoreCategories(raw, catLabel)]
})

const pageSize = ref(50)
const currentPage = ref(1)

const paginatedStore = computed(() => {
  const list = filteredStore.value
  const start = (currentPage.value - 1) * pageSize.value
  return list.slice(start, start + pageSize.value)
})

watch([category, keyword], () => {
  currentPage.value = 1
})

const filteredStore = computed(() => {
  let list = category.value === ALL
    ? storeApps.value
    : storeApps.value.filter(a =>
        normalizeStoreCategory(a.category) === category.value
        || categoryLabel(a.category, t) === catLabel(category.value),
      )
  const q = keyword.value.trim().toLowerCase()
  if (!q) return list
  const norm = (s: string) => s.toLowerCase().replace(/[\s._-]/g, '')
  const nq = norm(q)
  return list.filter(a =>
    a.name.toLowerCase().includes(q) ||
    a.key.toLowerCase().includes(q) ||
    (a.description || '').toLowerCase().includes(q) ||
    norm(a.name).includes(nq) ||
    norm(a.key).includes(nq)
  )
})

const versionOptions = computed(() => versionChoices(currentApp.value || {}))

const upgradeVersionOptions = computed(() => versionChoices(currentApp.value || {}))

function latestVersion(app: any) {
  const choices = versionChoices(app)
  return choices[0] || app?.version || ''
}

function hasUpgrade(app: any) {
  if (app?.grouped) return false
  if (!app?.installed) return false
  const latest = latestVersion(app)
  return !!latest && app.version !== latest
}

function descFor(app: any) {
  return appDescription(app, locale.value, t)
}

function nameFor(app: any) {
  return displayAppName(app, t)
}

watch(tab, (v) => router.replace({ query: { tab: v } }))

function catLabel(c: string) {
  return c === ALL ? t('software.allCategory') : categoryLabel(c, t)
}

async function loadStore() {
  try {
    const res: any = await api.get('/software/store')
    storeApps.value = res.data || []
    storeLoadFailed.value = false
  } catch (e: any) {
    storeApps.value = []
    storeLoadFailed.value = true
    ElMessage.error(e?.error || e?.message || t('software.storeLoadFailed'))
  }
}

async function loadInstalled() {
  const res: any = await api.get('/software/installed')
  installedApps.value = res.data || []
}

async function refreshStoreVersions() {
  refreshingVersions.value = true
  try {
    const res: any = await api.post('/software/store/refresh-versions')
    const msg = res.data?.message || res.message
    ElMessage.success(msg || t('software.versionsRefreshed'))
    await loadStore()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('software.versionsRefreshFailed'))
  } finally {
    refreshingVersions.value = false
  }
}

async function ensureStoreCatalog() {
  if (storeApps.value.length > 0 || storeLoadFailed.value) return
  initializingStore.value = true
  try {
    for (let attempt = 0; attempt < 3 && storeApps.value.length === 0; attempt++) {
      try {
        await api.post('/software/store/sync')
      } catch {
        /* best effort — built-in catalog is seeded at panel install/startup */
      }
      await loadStore()
      if (storeApps.value.length === 0 && attempt < 2) {
        await new Promise(resolve => setTimeout(resolve, 1000))
      }
    }
  } finally {
    initializingStore.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadStore(), loadInstalled()])
    if (storeApps.value.length === 0 && !storeLoadFailed.value) {
      await ensureStoreCatalog()
    }
    if (!versionsRefreshedThisSession.value && storeApps.value.length > 0) {
      versionsRefreshedThisSession.value = true
      api.post('/software/store/refresh-versions').then(() => loadStore()).catch(() => {})
    }
  } finally {
    loading.value = false
  }
}

function openInstall(app: any) {
  currentApp.value = app
  const choices = versionChoices(app)
  const notInstalled = choices.find(v => !isVersionInstalled(app, v))
  installVersion.value = notInstalled || choices[0] || app.version || ''
  installDialog.value = true
}

const installingKey = ref('')
const actionLoading = ref('')

function openInstallLog(app: { key: string; name: string; version?: string; grouped?: boolean; version_entries?: any[] }, triggerInstall = false, installKey?: string) {
  const key = installKey || resolveInstallKey(app, app.version || installVersion.value)
  installLogKey.value = key
  installLogName.value = nameFor(app)
  installLogVersion.value = app.version || installVersion.value || ''
  installLogVisible.value = true
  if (triggerInstall) {
    installingKey.value = key
  }
}

async function onInstallLogDone() {
  installingKey.value = ''
  await loadAll()
}

async function confirmInstall() {
  const app = currentApp.value
  if (isVersionInstalled(app, installVersion.value)) {
    ElMessage.warning(t('software.versionAlreadyInstalled'))
    return
  }
  const key = resolveInstallKey(app, installVersion.value)
  installDialog.value = false
  installLogMode.value = 'install'
  openInstallLog({ ...app, version: installVersion.value }, true, key)
}

function openUpgrade(app: any, presetVersion?: string) {
  currentApp.value = app
  upgradeVersion.value = presetVersion || app.version || latestVersion(app) || ''
  upgradeDialog.value = true
}

function confirmUpgrade() {
  const key = currentApp.value.key
  const name = currentApp.value.name
  upgradeDialog.value = false
  installLogMode.value = 'upgrade'
  openInstallLog({ key, name, version: upgradeVersion.value }, true)
}

function upgradeToLatest(app: any) {
  const latest = latestVersion(app)
  if (!latest || app.version === latest) return
  installLogMode.value = 'upgrade'
  openInstallLog({ key: app.key, name: app.name, version: latest }, true)
}

async function uninstall(app: any) {
  await ElMessageBox.confirm(t('software.uninstallConfirm', { name: app.name }), t('common.warning'), { type: 'warning' })
  try {
    await api.post(`/software/${app.key}/uninstall`)
    ElMessage.success(t('software.uninstallSuccess'))
    loadAll()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('software.installFailed'))
  }
}

async function doAction(app: any, action: string) {
  const key = `${app.key}:${action}`
  actionLoading.value = key
  try {
    await api.post(`/software/${app.key}/${action}`)
    ElMessage.success(t('common.success'))
    loadAll()
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('common.warning'))
  } finally {
    actionLoading.value = ''
  }
}

function actionBusy(app: any, action: string) {
  return actionLoading.value === `${app.key}:${action}`
}

async function openConfig(app: any) {
  currentApp.value = app
  configDialog.value = true
}

function openSettings(app: any) {
  currentApp.value = app
  settingsForm.value = {
    port: app.port,
    auto_start: app.auto_start,
    version: app.version,
    bind_domain: app.bind_domain || '',
  }
  settingsDialog.value = true
}

function openAccessUrl(app: any) {
  const url = app.access_url
  if (!url) return
  window.open(url.startsWith('http') ? url : `http://${url}`, '_blank', 'noopener,noreferrer')
}

async function saveSettings() {
  await api.patch(`/software/${currentApp.value.key}/settings`, settingsForm.value)
  ElMessage.success(t('common.success'))
  settingsDialog.value = false
  loadAll()
}

function statusType(status: string) {
  if (status === 'running') return 'success'
  if (status === 'simulated') return 'warning'
  if (status === 'installing') return 'warning'
  if (status === 'failed') return 'danger'
  return 'info'
}

function statusLabel(status: string) {
  if (status === 'installing') return t('software.installing')
  if (status === 'failed') return t('software.failed')
  if (status === 'simulated') return t('software.simulated')
  if (status === 'running') return t('common.running')
  if (status === 'stopped') return t('common.stopped')
  return status
}

async function openPhpMyAdmin(_app: any) {
  try {
    let res: any = await api.get('/phpmyadmin/access')
    let info = res.data
    if (!info?.installed) {
      ElMessage.warning(t('software.pmaNotInstalled'))
      return
    }
    if (!info?.url) {
      res = await api.post('/phpmyadmin/setup')
      info = res.data
    }
    if (!info?.url) {
      ElMessage.warning(info?.setup_error || t('software.pmaNoUrl'))
      return
    }
    window.open(info.url, '_blank', 'noopener,noreferrer')
  } catch (e: any) {
    ElMessage.error(e?.error || t('common.failed'))
  }
}

async function patchAutoOps(row: any, field: 'watch_enabled' | 'auto_restart', val: boolean) {
  const payload: Record<string, boolean> = { [field]: val }
  if (field === 'auto_restart' && val) payload.watch_enabled = true
  if (field === 'watch_enabled' && !val) payload.auto_restart = false
  try {
    await api.patch(`/auto-ops/watch/${row.key}`, payload)
    row.watch_enabled = payload.watch_enabled ?? row.watch_enabled
    row.auto_restart = payload.auto_restart ?? row.auto_restart
    ElMessage.success(t('autoOps.bulkUpdated'))
  } catch (e: any) {
    ElMessage.error(e?.error || e?.message || t('autoOps.updateFailed'))
    loadInstalled()
  }
}

function goAutoOps(row: any) {
  router.push({ path: '/auto-ops', query: { key: row.key } })
}

function goMailCenter() {
  router.push({ path: '/mail' })
}

onMounted(loadAll)
</script>

<template>
  <div>
    <div class="page-header">
      <h2>{{ t('software.title') }}</h2>
      <div class="header-actions">
        <el-input
          v-model="keyword"
          :placeholder="t('software.searchPlaceholder')"
          clearable
          style="width: 220px"
          prefix-icon="Search"
        />
        <el-button :loading="refreshingVersions" type="primary" plain @click="refreshStoreVersions">
          {{ t('software.refreshVersions') }}
        </el-button>
        <el-button :loading="loading" @click="loadAll">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-alert v-if="initializingStore" type="info" show-icon :closable="false" :title="t('software.storeInitializing')" style="margin-bottom: 16px" />

    <el-alert :title="t('software.dockerHint')" type="info" show-icon :closable="false" style="margin-bottom: 16px" />

    <el-alert v-if="normalizeStoreCategory(category) === '邮件'" :title="t('software.mailCategoryHint')" type="info" show-icon :closable="false" style="margin-bottom: 16px" />

    <el-alert v-if="installedApps.some(a => a.status === 'simulated')" type="warning" show-icon :closable="false" :title="t('software.simulatedHint')" style="margin-bottom: 16px" />

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('software.storeTab')" name="store">
        <el-radio-group v-model="category" style="margin-bottom: 16px">
          <el-radio-button v-for="c in categories" :key="c" :value="c">{{ catLabel(c) }}</el-radio-button>
        </el-radio-group>
        <el-row v-loading="loading || initializingStore" :gutter="16">
          <el-col v-for="app in paginatedStore" :key="app.grouped ? app.family_key || app.key : app.key" :span="6" style="margin-bottom: 16px">
            <el-card shadow="hover" class="soft-card" :class="{ installed: app.installed }">
              <div class="soft-top">
                <SoftwareIcon :app-key="iconKeyForApp(app)" :icon-url="app.icon_url" :size="52" />
                <div class="soft-info">
                  <div class="soft-head">
                    <span class="soft-name">{{ nameFor(app) }}</span>
                    <el-tag v-if="app.status === 'simulated'" type="warning" size="small">{{ t('software.simulated') }}</el-tag>
                    <el-tag v-else-if="app.installed" type="success" size="small">{{ t('common.installed') }}</el-tag>
                  </div>
                  <div class="soft-meta">{{ categoryLabel(app.category, t) }}</div>
                  <div v-if="app.grouped && installedVersionEntries(app).length" class="soft-current-ver">
                    {{ t('software.installedVersionsLabel') }}:
                    <span v-for="v in installedVersionEntries(app)" :key="v.version" class="ver-tag">{{ v.version }}</span>
                  </div>
                  <div v-else-if="app.installed && app.version" class="soft-current-ver">
                    {{ t('software.currentVersionLabel') }}: {{ app.version }}
                    <span v-if="hasUpgrade(app)" class="soft-latest-hint">
                      · {{ t('software.latestVersionLabel') }}: {{ latestVersion(app) }}
                    </span>
                  </div>
                </div>
              </div>
              <p class="soft-desc">{{ descFor(app) }}</p>
              <div class="soft-ver">{{ t('software.availableVersions') }}: {{ versionChoices(app).join(', ') }}</div>
              <div v-if="app.bind_domain" class="soft-domain">
                <el-link type="primary" :underline="false" @click.stop="openAccessUrl(app)">{{ app.bind_domain }}</el-link>
              </div>
              <div class="soft-actions">
                <el-button v-if="app.status === 'installing'" size="small" type="warning" plain @click="openInstallLog(app)">{{ t('software.viewInstallLog') }}</el-button>
                <el-button v-else-if="!app.installed || (app.grouped && versionChoices(app).some(v => !isVersionInstalled(app, v)))" type="primary" size="small" :loading="installingKey.startsWith(app.grouped ? (app.family_key || '') : app.key)" @click="openInstall(app)">
                  {{ app.grouped && app.installed ? t('software.installAnotherVersion') : t('common.install') }}
                </el-button>
                <template v-else-if="app.installed">
                  <el-button v-if="app.key === 'mail-server'" size="small" type="warning" @click="goMailCenter">{{ t('software.openMailCenter') }}</el-button>
                  <el-button v-if="hasUpgrade(app)" size="small" type="primary" :loading="installingKey === app.key" @click="upgradeToLatest(app)">{{ t('software.upgradeToLatest') }}</el-button>
                  <el-button size="small" plain @click="openUpgrade(app)">{{ t('software.changeVersion') }}</el-button>
                  <el-button size="small" @click="openSettings(app)">{{ t('common.settings') }}</el-button>
                  <el-button size="small" @click="openConfig(app)">{{ t('common.config') }}</el-button>
                  <el-button type="danger" size="small" plain @click="uninstall(app)">{{ t('common.uninstall') }}</el-button>
                </template>
              </div>
            </el-card>
          </el-col>
        </el-row>
        <el-empty v-if="storeLoadFailed && !loading" :description="t('software.storeLoadFailed')" />
        <el-empty v-else-if="!filteredStore.length && !loading" :description="t('software.emptySearch')" />
        <div v-if="filteredStore.length" class="store-footer">
          <span class="store-total">{{ t('software.total', { n: filteredStore.length }) }}</span>
          <el-pagination
            v-model:current-page="currentPage"
            :page-size="pageSize"
            :total="filteredStore.length"
            layout="total, prev, pager, next"
            background
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('software.installedTab')" name="installed">
        <el-table :data="installedApps" stripe v-loading="loading">
          <el-table-column :label="t('common.name')" width="200">
            <template #default="{ row }">
              <div class="table-app-name">
                <SoftwareIcon :app-key="row.key" :icon-url="row.icon_url" :size="36" />
                <span>{{ row.name }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="category" :label="t('common.type')" width="130">
            <template #default="{ row }">{{ categoryLabel(row.category, t) }}</template>
          </el-table-column>
          <el-table-column prop="version" :label="t('common.version')" width="120">
            <template #default="{ row }">
              <span>{{ row.version }}</span>
              <el-tag v-if="hasUpgrade(row)" type="warning" size="small" style="margin-left: 4px">{{ latestVersion(row) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="port" :label="t('common.port')" width="80">
            <template #default="{ row }">{{ row.port || '-' }}</template>
          </el-table-column>
          <el-table-column :label="t('software.bindDomain')" width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <el-link v-if="row.bind_domain" type="primary" :underline="false" @click="openAccessUrl(row)">
                {{ row.bind_domain }}
              </el-link>
              <span v-else-if="row.docker_app" class="muted-text">{{ t('software.bindDomainUnset') }}</span>
              <span v-else>—</span>
            </template>
          </el-table-column>
          <el-table-column prop="install_path" :label="t('software.installPath')" show-overflow-tooltip />
          <el-table-column prop="status" :label="t('common.status')" width="90">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="auto_start" :label="t('software.autoStart')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.auto_start ? 'success' : 'info'" size="small">{{ row.auto_start ? t('common.yes') : t('common.no') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('autoOps.autoRestart')" width="100">
            <template #default="{ row }">
              <el-switch
                v-if="row.status !== 'simulated'"
                :model-value="row.auto_restart"
                @change="(v: boolean) => patchAutoOps(row, 'auto_restart', v)"
              />
              <span v-else>—</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="480" fixed="right">
            <template #default="{ row }">
              <el-button
                v-if="row.status === 'installing'"
                text
                type="warning"
                size="small"
                @click="openInstallLog(row)"
              >
                {{ t('software.viewInstallLog') }}
              </el-button>
              <el-button
                v-if="row.key === 'mail-server'"
                text
                type="warning"
                size="small"
                @click="goMailCenter"
              >
                {{ t('software.openMailCenter') }}
              </el-button>
              <el-button
                v-if="row.key === 'phpmyadmin'"
                text
                type="warning"
                size="small"
                @click="openPhpMyAdmin(row)"
              >
                {{ t('software.openApp') }}
              </el-button>
              <el-button text type="primary" size="small" :loading="actionBusy(row, 'start')" @click="doAction(row, 'start')">{{ t('common.start') }}</el-button>
              <el-button text type="warning" size="small" :loading="actionBusy(row, 'stop')" @click="doAction(row, 'stop')">{{ t('common.stop') }}</el-button>
              <el-button text size="small" :loading="actionBusy(row, 'restart')" @click="doAction(row, 'restart')">{{ t('common.restart') }}</el-button>
              <el-button text size="small" :loading="actionBusy(row, 'reload')" @click="doAction(row, 'reload')">{{ t('common.reload') }}</el-button>
              <el-button text type="primary" size="small" @click="openConfig(row)">{{ t('common.config') }}</el-button>
              <el-button v-if="hasUpgrade(row)" text type="primary" size="small" @click="upgradeToLatest(row)">{{ t('software.upgradeToLatest') }}</el-button>
              <el-button text size="small" @click="openUpgrade(row)">{{ t('software.changeVersion') }}</el-button>
              <el-button text size="small" @click="openSettings(row)">{{ t('common.settings') }}</el-button>
              <el-button v-if="row.status !== 'simulated'" text size="small" @click="goAutoOps(row)">{{ t('autoOps.title') }}</el-button>
              <el-button text type="danger" size="small" @click="uninstall(row)">{{ t('common.uninstall') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!installedApps.length && !loading" :description="t('software.emptyInstalled')" />
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="installDialog" width="440px">
      <template #header>
        <div class="dialog-title">
          <SoftwareIcon v-if="currentApp" :app-key="currentApp.key" :icon-url="currentApp.icon_url" :size="40" />
          <span>{{ t('software.installTitle', { name: nameFor(currentApp) }) }}</span>
        </div>
      </template>
      <el-form label-width="80px">
        <el-form-item :label="t('common.version')">
          <el-select v-model="installVersion" style="width: 100%">
            <el-option
              v-for="v in versionOptions"
              :key="v"
              :label="v + (isVersionInstalled(currentApp, v) ? ' ✓' : '')"
              :value="v"
              :disabled="isVersionInstalled(currentApp, v)"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.description')">
          <span style="color: #909399; font-size: 13px">{{ descFor(currentApp) }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="installDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="!!installingKey" @click="confirmInstall">{{ t('software.confirmInstall') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="upgradeDialog" width="440px">
      <template #header>
        <div class="dialog-title">
          <SoftwareIcon v-if="currentApp" :app-key="currentApp.key" :icon-url="currentApp.icon_url" :size="40" />
          <span>{{ t('software.upgradeTitle', { name: currentApp?.name }) }}</span>
        </div>
      </template>
      <el-form label-width="100px">
        <el-form-item :label="t('software.currentVersionLabel')">
          <span>{{ currentApp?.version }}</span>
        </el-form-item>
        <el-form-item :label="t('common.version')">
          <el-select v-model="upgradeVersion" style="width: 100%">
            <el-option v-for="v in upgradeVersionOptions" :key="v" :label="v" :value="v" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="upgradeDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="!!installingKey" @click="confirmUpgrade">{{ t('software.confirmUpgrade') }}</el-button>
      </template>
    </el-dialog>

    <SoftwareConfigDialog v-model="configDialog" :app="currentApp" />

    <SoftwareInstallLogDialog
      v-model="installLogVisible"
      :app-key="installLogKey"
      :app-name="installLogName"
      :version="installLogVersion"
      :install-mode="installLogMode"
      :trigger-install="!!installingKey && installingKey === installLogKey"
      @done="onInstallLogDone"
    />

    <el-dialog v-model="settingsDialog" :title="t('software.settingsTitle', { name: currentApp?.name })" width="480px">
      <el-form :model="settingsForm" label-width="100px">
        <el-form-item v-if="currentApp?.docker_app" :label="t('software.bindDomain')">
          <el-input v-model="settingsForm.bind_domain" :placeholder="t('software.bindDomainPlaceholder')" clearable />
          <div class="form-hint">{{ t('software.bindDomainHint') }}</div>
        </el-form-item>
        <el-form-item v-if="currentApp?.docker_app && currentApp?.access_url" :label="t('software.accessUrl')">
          <el-link type="primary" :underline="false" @click="openAccessUrl(currentApp)">{{ currentApp.access_url }}</el-link>
        </el-form-item>
        <el-form-item :label="t('common.version')">
          <el-select v-model="settingsForm.version" style="width: 100%">
            <el-option v-for="v in (currentApp?.versions || '').split(',')" :key="v" :label="v.trim()" :value="v.trim()" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.port')">
          <el-input-number v-model="settingsForm.port" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item :label="t('software.autoStart')">
          <el-switch v-model="settingsForm.auto_start" />
        </el-form-item>
        <el-form-item :label="t('software.installPath')">
          <el-input :model-value="currentApp?.install_path" disabled />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.soft-card.installed { border-color: var(--el-color-success); }
.soft-top { display: flex; gap: 12px; margin-bottom: 10px; }
.soft-info { flex: 1; min-width: 0; }
.soft-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; margin-bottom: 4px; }
.soft-name { font-size: 16px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.soft-meta { color: #909399; font-size: 12px; }
.soft-current-ver { color: #606266; font-size: 12px; margin-top: 2px; }
.soft-latest-hint { color: var(--el-color-warning); }
.ver-tag { display: inline-block; margin-right: 4px; padding: 0 6px; border-radius: 4px; background: var(--el-color-success-light-9); color: var(--el-color-success); font-size: 11px; }
.soft-desc { color: #606266; font-size: 13px; min-height: 36px; margin-bottom: 6px; }
.soft-ver { color: #909399; font-size: 12px; margin-bottom: 10px; }
.soft-domain { font-size: 12px; margin-bottom: 8px; }
.muted-text { color: #909399; font-size: 12px; }
.form-hint { color: #909399; font-size: 12px; line-height: 1.5; margin-top: 4px; }
.soft-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.table-app-name { display: flex; align-items: center; gap: 10px; }
.dialog-title { display: flex; align-items: center; gap: 10px; font-size: 16px; font-weight: 600; }
.header-actions { display: flex; align-items: center; gap: 12px; }
.store-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 16px; flex-wrap: wrap; gap: 12px; }
.store-total { color: #909399; font-size: 13px; }
</style>
